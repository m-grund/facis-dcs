package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/ipfs"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/builder"
	"digital-contracting-service/internal/pdfgeneration/c2pa"
	"digital-contracting-service/internal/pdfgeneration/verify"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
)

type pdfGenerationSrvc struct {
	DB         *sqlx.DB
	IPFSClient *ipfs.APIClient
	CRepo      *cwerepo.PostgresContractRepo
	TRepo      *tplrepo.PostgresContractTemplateRepo
	Signer     c2pa.Signer
	TSACfg     c2pa.TSAConfig
	IssuerDID  string
	VCIssuer   c2pa.VCIssuer
	auth.JWTAuthenticator
}

// NewPDFGeneration constructs the PDFGeneration service implementation.
// Fails hard if required dependencies are nil (per SRS DCS-OR-C2PA-004 and 005).
func NewPDFGeneration(
	db *sqlx.DB,
	jwtAuth auth.JWTAuthenticator,
	ipfsClient *ipfs.APIClient,
	cRepo *cwerepo.PostgresContractRepo,
	tRepo *tplrepo.PostgresContractTemplateRepo,
	signer c2pa.Signer,
	tsaCfg c2pa.TSAConfig,
	issuerDID string,
	vcIssuer c2pa.VCIssuer,
) pdfgen.Service {
	if vcIssuer == nil {
		panic("VCIssuer is required for DCS-OR-C2PA-004 compliance")
	}
	// Note: VCIssuer now includes StatusListPublisher atomically (DCS-OR-C2PA-005).
	return &pdfGenerationSrvc{
		DB:               db,
		IPFSClient:       ipfsClient,
		CRepo:            cRepo,
		TRepo:            tRepo,
		Signer:           signer,
		TSACfg:           tsaCfg,
		IssuerDID:        issuerDID,
		VCIssuer:         vcIssuer,
		JWTAuthenticator: jwtAuth,
	}
}

func readCachedPDFMetadata(ctx context.Context, queryRow func(context.Context, string, ...any) *sql.Row, table, did string) (cidStr, c2paState, rendererVersion string, err error) {
	query := fmt.Sprintf(`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_c2pa_state,''), COALESCE(pdf_renderer_version,'') FROM %s WHERE did=$1`, table)
	if err := queryRow(ctx, query, did).Scan(&cidStr, &c2paState, &rendererVersion); err != nil {
		return "", "", "", err
	}
	return cidStr, c2paState, rendererVersion, nil
}

// ExportContractPdf exports a contract as a PDF/A-3 document.
// If a PDF is already stored in IPFS (from a prior C2PA append cycle) it is
// returned directly; otherwise a fresh PDF is built from the JSON-LD.
func (s *pdfGenerationSrvc) ExportContractPdf(ctx context.Context, p *pdfgen.ExportContractPdfPayload) (io.ReadCloser, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contract, err := s.CRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("contract %s: %w", p.Did, err))
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		b, err := json.Marshal(contract.ContractData)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal contract JSON-LD: %w", err))
		}
		jsonldBytes = b
	}

	cidStr, lastC2PAState, lastRendererVersion, scanErr := readCachedPDFMetadata(ctx, tx.QueryRowContext, "contracts", p.Did)
	if scanErr != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached contract PDF metadata for %s: %w", p.Did, scanErr))
	}
	currentC2PAState, err := c2pa.MapCWEStateToC2PAStrict(contract.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err))
	}
	log.Printf("pdfgeneration: ExportContractPdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		p.Did, cidStr, lastC2PAState, contract.State, currentC2PAState)
	if cidStr != "" && lastRendererVersion == builder.RendererVersion {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached PDF from IPFS %s: %w", cidStr, err))
		}
		log.Printf("pdfgeneration: ExportContractPdf %s fetched %d bytes from IPFS", p.Did, len(r.Data))

		// Extract JUMBF manifest from PDF, then check lifecycle status
		_, manifestBytes, _ := c2pa.ExtractAndVerifyManifest(r.Data)
		actualManifestState := c2pa.ExtractLifecycleStatus(manifestBytes)
		log.Printf("pdfgeneration: ExportContractPdf %s actualManifestState=%q currentC2PAState=%q",
			p.Did, actualManifestState, currentC2PAState)

		if actualManifestState == currentC2PAState {
			log.Printf("pdfgeneration: ExportContractPdf %s manifest matches current state (%s) — returning cached PDF", p.Did, actualManifestState)
			return io.NopCloser(bytes.NewReader(r.Data)), nil
		}

		log.Printf("pdfgeneration: ExportContractPdf %s manifest %q ≠ current %q — appending new assertion",
			p.Did, actualManifestState, currentC2PAState)
		// Manifest state doesn't match current state — append new assertion to PDF
		pdfBytes, err := s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, r.Data, "contracts")
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append C2PA assertion for contract %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit contract PDF append tx for %s: %w", p.Did, err))
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}
	if cidStr != "" && lastRendererVersion != builder.RendererVersion {
		log.Printf("pdfgeneration: ExportContractPdf %s cached renderer %q != current %q; rebuilding", p.Did, lastRendererVersion, builder.RendererVersion)
	}

	// No cached PDF — build from scratch.
	name := ""
	if contract.Name != nil {
		name = *contract.Name
	}
	desc := ""
	if contract.Description != nil {
		desc = *contract.Description
	}
	pdfBytes, err := builder.BuildContract(builder.ContractInput{
		DID:          contract.DID,
		State:        contract.State,
		Version:      contract.ContractVersion,
		Name:         name,
		Description:  desc,
		CreatedBy:    contract.CreatedBy,
		CreatedAt:    contract.CreatedAt,
		UpdatedAt:    contract.UpdatedAt,
		ContractData: jsonldBytes,
	})
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("build contract PDF: %w", err))
	}

	pdfBytes, err = s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, pdfBytes, "contracts")
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("append and cache contract PDF C2PA manifest: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("commit contract PDF export tx for %s: %w", p.Did, err))
	}
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}

// ExportTemplatePdf exports a contract template as a PDF/A-3 document.
func (s *pdfGenerationSrvc) ExportTemplatePdf(ctx context.Context, p *pdfgen.ExportTemplatePdfPayload) (io.ReadCloser, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("template %s: %w", p.Did, err))
	}

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		b, err := json.Marshal(tpl.TemplateData)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal template JSON-LD: %w", err))
		}
		jsonldBytes = b
	}

	cidStr, lastC2PAState, lastRendererVersion, scanErr := readCachedPDFMetadata(ctx, tx.QueryRowContext, "contract_templates", p.Did)
	if scanErr != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached template PDF metadata for %s: %w", p.Did, scanErr))
	}
	currentC2PAState, err := c2pa.MapCWEStateToC2PAStrict(tpl.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err))
	}
	log.Printf("pdfgeneration: ExportTemplatePdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		p.Did, cidStr, lastC2PAState, tpl.State, currentC2PAState)
	if cidStr != "" && lastRendererVersion == builder.RendererVersion {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached PDF from IPFS %s: %w", cidStr, err))
		}
		first := r.Data
		if len(first) > 8 {
			first = first[:8]
		}
		log.Printf("pdfgeneration: ExportTemplatePdf %s fetched %d bytes from IPFS, first8=%x", p.Did, len(r.Data), first)

		// Extract JUMBF manifest from PDF, then check lifecycle status
		_, manifestBytes, _ := c2pa.ExtractAndVerifyManifest(r.Data)
		actualManifestState := c2pa.ExtractLifecycleStatus(manifestBytes)
		log.Printf("pdfgeneration: ExportTemplatePdf %s actualManifestState=%q currentC2PAState=%q",
			p.Did, actualManifestState, currentC2PAState)

		if actualManifestState == currentC2PAState {
			log.Printf("pdfgeneration: ExportTemplatePdf %s manifest matches current state (%s) — returning cached PDF", p.Did, actualManifestState)
			return io.NopCloser(bytes.NewReader(r.Data)), nil
		}

		log.Printf("pdfgeneration: ExportTemplatePdf %s manifest %q ≠ current %q — appending new assertion",
			p.Did, actualManifestState, currentC2PAState)
		// Manifest state doesn't match current state — append new assertion to PDF
		pdfBytes, err := s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, r.Data, "contract_templates")
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append C2PA assertion for template %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit template PDF append tx for %s: %w", p.Did, err))
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}
	if cidStr != "" && lastRendererVersion != builder.RendererVersion {
		log.Printf("pdfgeneration: ExportTemplatePdf %s cached renderer %q != current %q; rebuilding", p.Did, lastRendererVersion, builder.RendererVersion)
	}

	// No cached PDF — build from scratch.
	name := ""
	if tpl.Name != nil {
		name = *tpl.Name
	}
	desc := ""
	if tpl.Description != nil {
		desc = *tpl.Description
	}
	docNumber := ""
	if tpl.DocumentNumber != nil {
		docNumber = *tpl.DocumentNumber
	}
	pdfBytes, err := builder.BuildTemplate(builder.TemplateInput{
		DID:            tpl.DID,
		State:          tpl.State,
		Version:        tpl.Version,
		Name:           name,
		Description:    desc,
		TemplateType:   tpl.TemplateType,
		DocumentNumber: docNumber,
		CreatedBy:      tpl.CreatedBy,
		CreatedAt:      tpl.CreatedAt,
		UpdatedAt:      tpl.UpdatedAt,
		TemplateData:   jsonldBytes,
	})
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("build template PDF: %w", err))
	}

	pdfBytes, err = s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, pdfBytes, "contract_templates")
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("append and cache template PDF C2PA manifest: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("commit template PDF export tx for %s: %w", p.Did, err))
	}
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}

// VerifyContractPdf verifies MR/HR hash consistency and C2PA provenance for a contract (DCS-OR-C2PA-006).
// Per DCS-OR-C2PA-003, if the contract state has advanced since the cached PDF was generated,
// this method ensures a new manifest is appended before verification.
func (s *pdfGenerationSrvc) VerifyContractPdf(ctx context.Context, p *pdfgen.VerifyContractPdfPayload) (*pdfgen.PDFVerifyResult, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contract, err := s.CRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("contract %s: %w", p.Did, err))
	}

	var cachedCID, cachedC2PAState string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_c2pa_state,'') FROM contracts WHERE did=$1`,
		p.Did).Scan(&cachedCID, &cachedC2PAState); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read contract PDF verification metadata for %s: %w", p.Did, err))
	}

	currentC2PAState, err := c2pa.MapCWEStateToC2PAStrict(contract.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err))
	}
	if cachedCID != "" && cachedC2PAState != currentC2PAState {
		// State has advanced since the cached PDF was created (DCS-OR-C2PA-003).
		// Append a new manifest to ensure the PDF reflects the current lifecycle state
		// before verification returns.
		log.Printf("pdfgeneration: VerifyContractPdf %s state advanced %q→%q; appending before verify",
			p.Did, cachedC2PAState, currentC2PAState)

		var jsonldBytes []byte
		if contract.ContractData != nil {
			b, err := json.Marshal(contract.ContractData)
			if err != nil {
				return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal contract JSON-LD for verify append %s: %w", p.Did, err))
			}
			jsonldBytes = b
		}

		r, err := s.IPFSClient.FetchFile(cachedCID)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached contract PDF %s from IPFS %s for verify append: %w", p.Did, cachedCID, err))
		}
		if _, err := s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, r.Data, "contracts"); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append contract C2PA assertion before verify for %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit pre-verify append tx for contract %s: %w", p.Did, err))
		}
	}

	pdfBytes, err := s.fetchOrBuildContractPDF(ctx, p.Did)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	v := &verify.ContractVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return s.rebuildContractFromJSONLD(ctx, p.Did, jsonld)
		},
		// FetchManifestFn retrieves the dedicated remote C2PA manifest object
		// (JUMBF bytes) when embedded provenance is stripped (DCS-OR-C2PA-008).
		FetchManifestFn: s.contractManifestIPFSFetchFn(ctx, p.Did),
		// FetchFn fetches the canonical PDF (with C2PA manifests) from IPFS when the
		// input PDF has been stripped of incremental updates (DCS-OR-C2PA-008).
		FetchFn: s.contractIPFSFetchFn(ctx, p.Did),
		// CheckStatusFn queries live revocation state from the XFSC status list (DCS-OR-C2PA-006).
		CheckStatusFn: func(statusListCredential string, index uint32) (string, error) {
			return c2pa.QueryStatusListStatus(ctx, httpClient, statusListCredential, index)
		},
	}
	result, err := v.Verify(pdfBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify contract PDF: %w", err))
	}
	return &pdfgen.PDFVerifyResult{
		Match:              result.Match,
		JsonldHash:         result.JSONLDHash,
		BasePdfHash:        result.BasePDFHash,
		StoredBasePdfHash:  result.StoredBasePDFHash,
		C2paManifestFound:  result.C2PAManifestFound,
		C2paSignatureValid: result.C2PASignatureValid,
		VcProofValid:       result.VCProofValid,
		StatusListURI:      ptrToString(result.StatusListURI),
		LifecycleStatus:    ptrToString(result.LifecycleStatus),
		StatusListStatus:   ptrToString(result.StatusListStatus),
	}, nil
}

// VerifyTemplatePdf verifies MR/HR hash consistency and C2PA provenance for a template (DCS-OR-C2PA-006).
// Per DCS-OR-C2PA-003, if the template state has advanced since the cached PDF was generated,
// this method ensures a new manifest is appended before verification.
func (s *pdfGenerationSrvc) VerifyTemplatePdf(ctx context.Context, p *pdfgen.VerifyTemplatePdfPayload) (*pdfgen.PDFVerifyResult, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("template %s: %w", p.Did, err))
	}

	var cachedCID, cachedC2PAState string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_c2pa_state,'') FROM contract_templates WHERE did=$1`,
		p.Did).Scan(&cachedCID, &cachedC2PAState); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read template PDF verification metadata for %s: %w", p.Did, err))
	}

	currentC2PAState, err := c2pa.MapCWEStateToC2PAStrict(tpl.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err))
	}
	if cachedCID != "" && cachedC2PAState != currentC2PAState {
		// State has advanced since the cached PDF was created (DCS-OR-C2PA-003).
		// Append a new manifest to ensure the PDF reflects the current lifecycle state
		// before verification returns.
		log.Printf("pdfgeneration: VerifyTemplatePdf %s state advanced %q→%q; appending before verify",
			p.Did, cachedC2PAState, currentC2PAState)

		var jsonldBytes []byte
		if tpl.TemplateData != nil {
			b, err := json.Marshal(tpl.TemplateData)
			if err != nil {
				return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal template JSON-LD for verify append %s: %w", p.Did, err))
			}
			jsonldBytes = b
		}

		r, err := s.IPFSClient.FetchFile(cachedCID)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached template PDF %s from IPFS %s for verify append: %w", p.Did, cachedCID, err))
		}
		if _, err := s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, r.Data, "contract_templates"); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append template C2PA assertion before verify for %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit pre-verify append tx for template %s: %w", p.Did, err))
		}
	}

	pdfBytes, err := s.fetchOrBuildTemplatePDF(ctx, p.Did)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	v := &verify.TemplateVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return s.rebuildTemplateFromJSONLD(ctx, p.Did, jsonld)
		},
		// FetchManifestFn retrieves the dedicated remote C2PA manifest object
		// for templates (DCS-OR-C2PA-008).
		FetchManifestFn: s.templateManifestIPFSFetchFn(ctx, p.Did),
		// FetchFn fetches the canonical PDF from IPFS when stripped (DCS-OR-C2PA-008).
		FetchFn: s.templateIPFSFetchFn(ctx, p.Did),
		// CheckStatusFn queries live revocation state from the XFSC status list (DCS-OR-C2PA-006).
		CheckStatusFn: func(statusListCredential string, index uint32) (string, error) {
			return c2pa.QueryStatusListStatus(ctx, httpClient, statusListCredential, index)
		},
	}
	result, err := v.Verify(pdfBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify template PDF: %w", err))
	}
	return &pdfgen.PDFVerifyResult{
		Match:              result.Match,
		JsonldHash:         result.JSONLDHash,
		BasePdfHash:        result.BasePDFHash,
		StoredBasePdfHash:  result.StoredBasePDFHash,
		C2paManifestFound:  result.C2PAManifestFound,
		C2paSignatureValid: result.C2PASignatureValid,
		VcProofValid:       result.VCProofValid,
		StatusListURI:      ptrToString(result.StatusListURI),
		LifecycleStatus:    ptrToString(result.LifecycleStatus),
		StatusListStatus:   ptrToString(result.StatusListStatus),
	}, nil
}

// contractIPFSFetchFn returns a FetchFn that retrieves the canonical contract PDF
// from IPFS using the stored pdf_ipfs_cid (DCS-OR-C2PA-008).
func (s *pdfGenerationSrvc) contractIPFSFetchFn(ctx context.Context, did string) func() ([]byte, error) {
	if s.IPFSClient == nil {
		return nil
	}
	return func() ([]byte, error) {
		cidStr, _, rendererVersion, err := readCachedPDFMetadata(ctx, s.DB.QueryRowContext, "contracts", did)
		if err != nil {
			return nil, fmt.Errorf("read contract pdf_ipfs_cid for %s: %w", did, err)
		}
		if cidStr == "" || rendererVersion != builder.RendererVersion {
			if cidStr != "" && rendererVersion != builder.RendererVersion {
				log.Printf("pdfgeneration: fetchOrBuildContractPDF %s cached renderer %q != current %q; rebuilding", did, rendererVersion, builder.RendererVersion)
			}
			return nil, nil
		}
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}
}

// templateIPFSFetchFn is the template counterpart of contractIPFSFetchFn (DCS-OR-C2PA-008).
func (s *pdfGenerationSrvc) templateIPFSFetchFn(ctx context.Context, did string) func() ([]byte, error) {
	if s.IPFSClient == nil {
		return nil
	}
	return func() ([]byte, error) {
		cidStr, _, rendererVersion, err := readCachedPDFMetadata(ctx, s.DB.QueryRowContext, "contract_templates", did)
		if err != nil {
			return nil, fmt.Errorf("read template pdf_ipfs_cid for %s: %w", did, err)
		}
		if cidStr == "" || rendererVersion != builder.RendererVersion {
			if cidStr != "" && rendererVersion != builder.RendererVersion {
				log.Printf("pdfgeneration: fetchOrBuildTemplatePDF %s cached renderer %q != current %q; rebuilding", did, rendererVersion, builder.RendererVersion)
			}
			return nil, nil
		}
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}
}

// contractManifestIPFSFetchFn returns a FetchManifestFn that retrieves the
// standalone remote C2PA manifest bytes for the given contract DID.
func (s *pdfGenerationSrvc) contractManifestIPFSFetchFn(ctx context.Context, did string) func() ([]byte, error) {
	if s.IPFSClient == nil {
		return nil
	}
	return func() ([]byte, error) {
		var cidStr string
		if err := s.DB.QueryRowContext(ctx,
			`SELECT COALESCE(pdf_manifest_ipfs_cid,'') FROM contracts WHERE did=$1`, did,
		).Scan(&cidStr); err != nil {
			return nil, fmt.Errorf("read contract pdf_manifest_ipfs_cid for %s: %w", did, err)
		}
		if cidStr == "" {
			return nil, nil
		}
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}
}

// templateManifestIPFSFetchFn returns a FetchManifestFn that retrieves the
// standalone remote C2PA manifest bytes for the given template DID.
func (s *pdfGenerationSrvc) templateManifestIPFSFetchFn(ctx context.Context, did string) func() ([]byte, error) {
	if s.IPFSClient == nil {
		return nil
	}
	return func() ([]byte, error) {
		var cidStr string
		if err := s.DB.QueryRowContext(ctx,
			`SELECT COALESCE(pdf_manifest_ipfs_cid,'') FROM contract_templates WHERE did=$1`, did,
		).Scan(&cidStr); err != nil {
			return nil, fmt.Errorf("read template pdf_manifest_ipfs_cid for %s: %w", did, err)
		}
		if cidStr == "" {
			return nil, nil
		}
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}
}

// appendAndCache appends an initial C2PA assertion and stores the PDF in IPFS,
// updating the pdf_ipfs_cid column in the given table.
// It also creates a W3C Verifiable Credential for the lifecycle event (DCS-OR-C2PA-004).
func (s *pdfGenerationSrvc) appendAndCache(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, table string,
) ([]byte, error) {
	if s.Signer == nil {
		return nil, fmt.Errorf("C2PA signer is not configured")
	}
	assetHash := c2pa.FileHashOf(pdfBytes)
	prevHash := c2pa.PrevManifestHashFrom(pdfBytes)

	// If the PDF was freshly built (no embedded manifest) check whether a prior
	// content-changing edit preserved a chain link in prev_manifest_hash
	// (DCS-OR-C2PA-001 Gap E).
	if prevHash == "" {
		var stored string
		if err := tx.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COALESCE(prev_manifest_hash,'') FROM %s WHERE did=$1`, table),
			did,
		).Scan(&stored); err != nil {
			return nil, fmt.Errorf("read prev_manifest_hash for %s from %s: %w", did, table, err)
		}
		prevHash = stored
	}

	// Map the raw CWE/DB state to the SRS-defined C2PA vocabulary (DCS-OR-C2PA-003).
	c2paState, err := c2pa.MapCWEStateToC2PAStrict(state)
	if err != nil {
		return pdfBytes, fmt.Errorf("map lifecycle state %q: %w", state, err)
	}
	var fromState string
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COALESCE(pdf_c2pa_state,'') FROM %s WHERE did=$1`, table),
		did,
	).Scan(&fromState); err != nil {
		return nil, fmt.Errorf("read previous C2PA state for %s from %s: %w", did, table, err)
	}
	log.Printf("pdfgeneration: appendAndCache %s table=%s state=%s c2paState=%s prevHash=%q pdfLen=%d",
		did, table, state, c2paState, prevHash, len(pdfBytes))

	// Generate a reason based on the state transition (DCS-OR-C2PA-003).
	reason := stateToReason(c2paState)

	// Issue a W3C VC for this lifecycle event (DCS-OR-C2PA-004).
	// Status list publication is atomic with VC issuance (DCS-OR-C2PA-005).
	vcID, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, s.IssuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	assertion := c2pa.NewLifecycleAssertion(
		did, assetHash, assetHash, builder.RendererVersion,
		c2paState, reason, s.IssuerDID, vcID, prevHash,
		time.Now().UTC(),
	)
	result, err := c2pa.AppendManifest(ctx, s.Signer, s.TSACfg, s.IPFSClient, s.IssuerDID, assertion, pdfBytes, vcBytes)
	if err != nil {
		return pdfBytes, err
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3, pdf_manifest_hash=$4, pdf_manifest_ipfs_cid=$5, prev_manifest_hash=NULL WHERE did=$6`, table),
		result.IPFSCID, builder.RendererVersion, c2paState, result.ManifestHash, result.ManifestIPFSCID, did,
	); err != nil {
		return nil, fmt.Errorf("update %s pdf_ipfs_cid: %w", table, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO c2pa_audit_log (entity_type, entity_did, from_state, to_state, actor_did, reason, vc_id, manifest_hash, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		auditEntityTypeForTable(table), did, nullableString(fromState), c2paState, s.IssuerDID,
		reason, nullableString(vcID), result.ManifestHash, time.Now().UTC(),
	); err != nil {
		return nil, fmt.Errorf("insert c2pa_audit_log for %s: %w", did, err)
	}
	log.Printf("pdfgeneration: appendAndCache %s done → CID=%s pdfLen=%d", did, result.IPFSCID, len(result.UpdatedPDF))
	return result.UpdatedPDF, nil
}

func (s *pdfGenerationSrvc) fetchOrBuildContractPDF(ctx context.Context, did string) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var cidStr string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contracts WHERE did=$1`, did).Scan(&cidStr); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached contract PDF CID for %s: %w", did, err))
	}
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached contract PDF for %s from IPFS %s: %w", did, cidStr, err))
		}
		if len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("cached contract PDF for %s from IPFS %s is empty", did, cidStr))
		}
		return r.Data, nil
	}

	// Fall back: build from scratch.
	contract, err := s.CRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("contract %s: %w", did, err))
	}
	var jsonldBytes []byte
	if contract.ContractData != nil {
		b, err := json.Marshal(contract.ContractData)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal contract JSON-LD for rebuild %s: %w", did, err))
		}
		jsonldBytes = b
	}
	name := ""
	if contract.Name != nil {
		name = *contract.Name
	}
	return builder.BuildContract(builder.ContractInput{
		DID: did, State: contract.State, Version: contract.ContractVersion,
		Name: name, CreatedBy: contract.CreatedBy,
		CreatedAt: contract.CreatedAt, UpdatedAt: contract.UpdatedAt,
		ContractData: jsonldBytes,
	})
}

func (s *pdfGenerationSrvc) rebuildContractFromJSONLD(ctx context.Context, did string, jsonld []byte) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)
	contract, err := s.CRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, err
	}
	name := ""
	if contract.Name != nil {
		name = *contract.Name
	}
	return builder.BuildContract(builder.ContractInput{
		DID: did, State: contract.State, Version: contract.ContractVersion,
		Name: name, CreatedBy: contract.CreatedBy,
		CreatedAt: contract.CreatedAt, UpdatedAt: contract.UpdatedAt,
		ContractData: jsonld,
	})
}

func (s *pdfGenerationSrvc) fetchOrBuildTemplatePDF(ctx context.Context, did string) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var cidStr string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contract_templates WHERE did=$1`, did).Scan(&cidStr); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached template PDF CID for %s: %w", did, err))
	}
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached template PDF for %s from IPFS %s: %w", did, cidStr, err))
		}
		if len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("cached template PDF for %s from IPFS %s is empty", did, cidStr))
		}
		return r.Data, nil
	}

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("template %s: %w", did, err))
	}
	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		b, err := json.Marshal(tpl.TemplateData)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("marshal template JSON-LD for rebuild %s: %w", did, err))
		}
		jsonldBytes = b
	}
	name := ""
	if tpl.Name != nil {
		name = *tpl.Name
	}
	docNumber := ""
	if tpl.DocumentNumber != nil {
		docNumber = *tpl.DocumentNumber
	}
	return builder.BuildTemplate(builder.TemplateInput{
		DID: did, State: tpl.State, Version: tpl.Version,
		Name: name, TemplateType: tpl.TemplateType, DocumentNumber: docNumber,
		CreatedBy: tpl.CreatedBy, CreatedAt: tpl.CreatedAt, UpdatedAt: tpl.UpdatedAt,
		TemplateData: jsonldBytes,
	})
}

func (s *pdfGenerationSrvc) rebuildTemplateFromJSONLD(ctx context.Context, did string, jsonld []byte) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)
	tpl, err := s.TRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, err
	}
	name := ""
	if tpl.Name != nil {
		name = *tpl.Name
	}
	docNumber := ""
	if tpl.DocumentNumber != nil {
		docNumber = *tpl.DocumentNumber
	}
	return builder.BuildTemplate(builder.TemplateInput{
		DID: did, State: tpl.State, Version: tpl.Version,
		Name: name, TemplateType: tpl.TemplateType, DocumentNumber: docNumber,
		CreatedBy: tpl.CreatedBy, CreatedAt: tpl.CreatedAt, UpdatedAt: tpl.UpdatedAt,
		TemplateData: jsonld,
	})
}

// stateToReason generates a human-readable reason for a state transition (DCS-OR-C2PA-003).
func stateToReason(state string) string {
	switch state {
	case "draft":
		return "Contract created as draft"
	case "active":
		return "Contract activated for execution"
	case "amended":
		return "Contract amended with new terms"
	case "suspended":
		return "Contract suspended pending review"
	case "terminated":
		return "Contract terminated by parties"
	case "expired":
		return "Contract reached expiration date"
	case "replaced":
		return "Contract replaced with newer version"
	default:
		return "Contract state changed to: " + state
	}
}

// ptrToString converts a string to a *string pointer, handling empty strings.
func ptrToString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func auditEntityTypeForTable(table string) string {
	if table == "contract_templates" {
		return "template"
	}
	return "contract"
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
