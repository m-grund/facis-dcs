package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"time"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/builder"
	"digital-contracting-service/internal/pdfgeneration/c2pa"
	"digital-contracting-service/internal/pdfgeneration/verify"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
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
	auth.JWTAuthenticator
}

// NewPDFGeneration constructs the PDFGeneration service implementation.
func NewPDFGeneration(
	db *sqlx.DB,
	jwtAuth auth.JWTAuthenticator,
	ipfsClient *ipfs.APIClient,
	cRepo *cwerepo.PostgresContractRepo,
	tRepo *tplrepo.PostgresContractTemplateRepo,
	signer c2pa.Signer,
	tsaCfg c2pa.TSAConfig,
	issuerDID string,
) pdfgen.Service {
	return &pdfGenerationSrvc{
		DB:               db,
		IPFSClient:       ipfsClient,
		CRepo:            cRepo,
		TRepo:            tRepo,
		Signer:           signer,
		TSACfg:           tsaCfg,
		IssuerDID:        issuerDID,
		JWTAuthenticator: jwtAuth,
	}
}

// ExportContractPdf exports a contract as a PDF/A-3 document.
// If a PDF is already stored in IPFS (from a prior C2PA append cycle) it is
// returned directly; otherwise a fresh PDF is built from the JSON-LD.
func (s *pdfGenerationSrvc) ExportContractPdf(ctx context.Context, p *pdfgen.ExportContractPdfPayload) (io.ReadCloser, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer tx.Rollback()

	contract, err := s.CRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("contract %s: %w", p.Did, err))
	}

	// Return cached PDF from IPFS if available.
	var cidStr string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contracts WHERE did=$1`, p.Did).Scan(&cidStr)
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err == nil && len(r.Data) > 0 {
			return io.NopCloser(bytes.NewReader(r.Data)), nil
		}
	}

	// Build fresh PDF.
	var jsonldBytes []byte
	if contract.ContractData != nil {
		if b, err := json.Marshal(contract.ContractData); err == nil {
			jsonldBytes = b
		}
	}
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

	// Append initial C2PA assertion and cache to IPFS.
	pdfBytes, err = s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, pdfBytes, "contracts")
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("append and cache contract PDF C2PA manifest: %w", err))
	}

	_ = tx.Commit()
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}

// ExportTemplatePdf exports a contract template as a PDF/A-3 document.
func (s *pdfGenerationSrvc) ExportTemplatePdf(ctx context.Context, p *pdfgen.ExportTemplatePdfPayload) (io.ReadCloser, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer tx.Rollback()

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, p.Did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("template %s: %w", p.Did, err))
	}

	// Return cached PDF from IPFS if available.
	var cidStr string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contract_templates WHERE did=$1`, p.Did).Scan(&cidStr)
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err == nil && len(r.Data) > 0 {
			return io.NopCloser(bytes.NewReader(r.Data)), nil
		}
	}

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		if b, err := json.Marshal(tpl.TemplateData); err == nil {
			jsonldBytes = b
		}
	}
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
	_ = tx.Commit()
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}

// VerifyContractPdf verifies MR/HR hash consistency for a contract.
func (s *pdfGenerationSrvc) VerifyContractPdf(ctx context.Context, p *pdfgen.VerifyContractPdfPayload) (*pdfgen.PDFVerifyResult, error) {
	pdfBytes, err := s.fetchOrBuildContractPDF(ctx, p.Did)
	if err != nil {
		return nil, err
	}

	v := &verify.ContractVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return s.rebuildContractFromJSONLD(ctx, p.Did, jsonld)
		},
	}
	result, err := v.Verify(pdfBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify contract PDF: %w", err))
	}
	return &pdfgen.PDFVerifyResult{
		Match:             result.Match,
		JsonldHash:        result.JSONLDHash,
		BasePdfHash:       result.BasePDFHash,
		StoredBasePdfHash: result.StoredBasePDFHash,
	}, nil
}

// VerifyTemplatePdf verifies MR/HR hash consistency for a template.
func (s *pdfGenerationSrvc) VerifyTemplatePdf(ctx context.Context, p *pdfgen.VerifyTemplatePdfPayload) (*pdfgen.PDFVerifyResult, error) {
	pdfBytes, err := s.fetchOrBuildTemplatePDF(ctx, p.Did)
	if err != nil {
		return nil, err
	}

	v := &verify.TemplateVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return s.rebuildTemplateFromJSONLD(ctx, p.Did, jsonld)
		},
	}
	result, err := v.Verify(pdfBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify template PDF: %w", err))
	}
	return &pdfgen.PDFVerifyResult{
		Match:             result.Match,
		JsonldHash:        result.JSONLDHash,
		BasePdfHash:       result.BasePDFHash,
		StoredBasePdfHash: result.StoredBasePDFHash,
	}, nil
}

// appendAndCache appends an initial C2PA assertion and stores the PDF in IPFS,
// updating the pdf_ipfs_cid column in the given table.
func (s *pdfGenerationSrvc) appendAndCache(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, table string,
) ([]byte, error) {
	if s.Signer == nil {
		return nil, fmt.Errorf("C2PA signer is not configured")
	}
	fileHash := c2pa.FileHashOf(jsonldBytes)
	pdfHash := c2pa.FileHashOf(pdfBytes)
	prevHash := c2pa.PrevManifestHashFrom(pdfBytes)
	assertion := c2pa.NewLifecycleAssertion(
		did, fileHash, pdfHash, builder.RendererVersion,
		state, "", s.IssuerDID, "", prevHash,
		time.Now().UTC(),
	)
	result, err := c2pa.AppendManifest(ctx, s.Signer, s.TSACfg, s.IPFSClient, s.IssuerDID, assertion, pdfBytes)
	if err != nil {
		return pdfBytes, err
	}
	_, _ = tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET pdf_ipfs_cid=$1, pdf_renderer_version=$2 WHERE did=$3`, table),
		result.IPFSCID, builder.RendererVersion, did,
	)
	return result.UpdatedPDF, nil
}

func (s *pdfGenerationSrvc) fetchOrBuildContractPDF(ctx context.Context, did string) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer tx.Rollback()

	var cidStr string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contracts WHERE did=$1`, did).Scan(&cidStr)
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err == nil && len(r.Data) > 0 {
			return r.Data, nil
		}
	}

	// Fall back: build from scratch.
	contract, err := s.CRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("contract %s: %w", did, err))
	}
	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes, _ = json.Marshal(contract.ContractData)
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
	defer tx.Rollback()
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
	defer tx.Rollback()

	var cidStr string
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contract_templates WHERE did=$1`, did).Scan(&cidStr)
	if cidStr != "" {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err == nil && len(r.Data) > 0 {
			return r.Data, nil
		}
	}

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, pdfgen.MakeNotFound(fmt.Errorf("template %s: %w", did, err))
	}
	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		jsonldBytes, _ = json.Marshal(tpl.TemplateData)
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
	defer tx.Rollback()
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
