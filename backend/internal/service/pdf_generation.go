package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/ipfs"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
)

type pdfGenerationSrvc struct {
	DB         *sqlx.DB
	IPFSClient *ipfs.APIClient
	CRepo      *cwerepo.PostgresContractRepo
	TRepo      *tplrepo.PostgresContractTemplateRepo
	PDFCore         *pdfcore.Client
	IssuerDID       string
	VCIssuer        provenance.VCIssuer
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
	pdfCore *pdfcore.Client,
	issuerDID string,
	vcIssuer provenance.VCIssuer,
) pdfgen.Service {
	if vcIssuer == nil {
		panic("VCIssuer is required for DCS-OR-C2PA-004 compliance")
	}
	if pdfCore == nil {
		panic("PDFCore client is required")
	}
	return &pdfGenerationSrvc{
		DB:               db,
		IPFSClient:       ipfsClient,
		CRepo:            cRepo,
		TRepo:            tRepo,
		PDFCore:          pdfCore,
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
// If a PDF is already stored in IPFS with the current C2PA state it is returned
// directly; otherwise a fresh PDF is built via pdf-core and cached.
func (s *pdfGenerationSrvc) ExportContractPdf(ctx context.Context, p *pdfgen.ExportContractPdfPayload) (io.ReadCloser, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
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
		jsonldBytes, err = pdfgeneration.InjectTitle(b, contract.Name)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("inject title into contract JSON-LD: %w", err))
		}
	}

	cidStr, lastC2PAState, _, scanErr := readCachedPDFMetadata(ctx, tx.QueryRowContext, "contracts", p.Did)
	if scanErr != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached contract PDF metadata for %s: %w", p.Did, scanErr))
	}
	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err))
	}
	log.Printf("pdfgeneration: ExportContractPdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		p.Did, cidStr, lastC2PAState, contract.State, currentC2PAState)

	if cidStr != "" && lastC2PAState == currentC2PAState {
		// DB state is authoritative — cached PDF is current.
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached PDF from IPFS %s: %w", cidStr, err))
		}
		log.Printf("pdfgeneration: ExportContractPdf %s state matches — returning cached PDF (%d bytes)", p.Did, len(r.Data))
		return io.NopCloser(bytes.NewReader(r.Data)), nil
	}

	if cidStr != "" {
		// Existing PDF but state has advanced — update via pdf-core.
		log.Printf("pdfgeneration: ExportContractPdf %s state changed %q→%q; appending", p.Did, lastC2PAState, currentC2PAState)
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch PDF from IPFS %s for update: %w", cidStr, err))
		}
		pdfBytes, err := s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, r.Data, "contracts")
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append C2PA assertion for contract %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit contract PDF append tx for %s: %w", p.Did, err))
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}

	// No cached PDF — render from scratch via pdf-core /download.
	pdfBytes, _, err := s.PDFCore.Download(ctx, jsonldBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("pdf-core download for contract %s: %w", p.Did, err))
	}

	pdfBytes, err = s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, pdfBytes, "contracts")
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("append and cache contract PDF for %s: %w", p.Did, err))
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
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
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
		jsonldBytes, err = pdfgeneration.InjectTitle(b, tpl.Name)
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("inject title into template JSON-LD: %w", err))
		}
	}

	cidStr, lastC2PAState, _, scanErr := readCachedPDFMetadata(ctx, tx.QueryRowContext, "contract_templates", p.Did)
	if scanErr != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("read cached template PDF metadata for %s: %w", p.Did, scanErr))
	}
	currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err))
	}
	log.Printf("pdfgeneration: ExportTemplatePdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		p.Did, cidStr, lastC2PAState, tpl.State, currentC2PAState)

	if cidStr != "" && lastC2PAState == currentC2PAState {
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached PDF from IPFS %s: %w", cidStr, err))
		}
		log.Printf("pdfgeneration: ExportTemplatePdf %s state matches — returning cached PDF (%d bytes)", p.Did, len(r.Data))
		return io.NopCloser(bytes.NewReader(r.Data)), nil
	}

	if cidStr != "" {
		log.Printf("pdfgeneration: ExportTemplatePdf %s state changed %q→%q; appending", p.Did, lastC2PAState, currentC2PAState)
		r, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(r.Data) == 0 {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch PDF from IPFS %s for update: %w", cidStr, err))
		}
		pdfBytes, err := s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, r.Data, "contract_templates")
		if err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append C2PA assertion for template %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit template PDF append tx for %s: %w", p.Did, err))
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}

	// No cached PDF — render from scratch via pdf-core /download.
	pdfBytes, _, err := s.PDFCore.Download(ctx, jsonldBytes)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("pdf-core download for template %s: %w", p.Did, err))
	}

	pdfBytes, err = s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, pdfBytes, "contract_templates")
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("append and cache template PDF for %s: %w", p.Did, err))
	}
	if err := tx.Commit(); err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("commit template PDF export tx for %s: %w", p.Did, err))
	}
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}

// VerifyContractPdf verifies content integrity and C2PA provenance for a contract (DCS-OR-C2PA-006).
// Delegates content-match + C2PA-chain validation to pdf-core /verify; performs VC proof and
// status-list checks locally using the remaining c2pa helpers.
func (s *pdfGenerationSrvc) VerifyContractPdf(ctx context.Context, p *pdfgen.VerifyContractPdfPayload) (*pdfgen.PDFVerifyResult, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
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

	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err))
	}

	if cachedCID != "" && cachedC2PAState != currentC2PAState {
		// State has advanced — append before verifying (DCS-OR-C2PA-003).
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
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached contract PDF %s from IPFS for verify append: %w", p.Did, err))
		}
		if _, err := s.appendAndCache(ctx, tx, p.Did, contract.State, jsonldBytes, r.Data, "contracts"); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append contract C2PA assertion before verify for %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit pre-verify append tx for contract %s: %w", p.Did, err))
		}
	}

	// Fetch the latest PDF (CID may have changed after state advance above).
	var latestCID string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contracts WHERE did=$1`, p.Did).Scan(&latestCID); err != nil || latestCID == "" {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("no cached PDF for contract %s; call export first", p.Did))
	}
	r, err := s.IPFSClient.FetchFile(latestCID)
	if err != nil || len(r.Data) == 0 {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch contract PDF %s from IPFS for verify: %w", p.Did, err))
	}
	pdfBytes := []byte(r.Data)

	return s.runVerify(ctx, pdfBytes)
}

// VerifyTemplatePdf verifies content integrity and C2PA provenance for a template (DCS-OR-C2PA-006).
func (s *pdfGenerationSrvc) VerifyTemplatePdf(ctx context.Context, p *pdfgen.VerifyTemplatePdfPayload) (*pdfgen.PDFVerifyResult, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
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

	currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err))
	}

	if cachedCID != "" && cachedC2PAState != currentC2PAState {
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
			return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch cached template PDF %s from IPFS for verify append: %w", p.Did, err))
		}
		if _, err := s.appendAndCache(ctx, tx, p.Did, tpl.State, jsonldBytes, r.Data, "contract_templates"); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("append template C2PA assertion before verify for %s: %w", p.Did, err))
		}
		if err := tx.Commit(); err != nil {
			return nil, pdfgen.MakeInternalError(fmt.Errorf("commit pre-verify append tx for template %s: %w", p.Did, err))
		}
	}

	var latestCID string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(pdf_ipfs_cid,'') FROM contract_templates WHERE did=$1`, p.Did).Scan(&latestCID); err != nil || latestCID == "" {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("no cached PDF for template %s; call export first", p.Did))
	}
	r, err := s.IPFSClient.FetchFile(latestCID)
	if err != nil || len(r.Data) == 0 {
		return nil, pdfgen.MakeInternalError(fmt.Errorf("fetch template PDF %s from IPFS for verify: %w", p.Did, err))
	}
	pdfBytes := []byte(r.Data)

	return s.runVerify(ctx, pdfBytes)
}

// runVerify delegates content-match + C2PA-chain validation to pdf-core /verify,
// then performs status-list checks locally using VC bytes from the response.
func (s *pdfGenerationSrvc) runVerify(ctx context.Context, pdfBytes []byte) (*pdfgen.PDFVerifyResult, error) {
	// pdf-core /verify: re-renders JSON-LD and compares, validates C2PA chain.
	// 200 → intact; 409 → content mismatch or C2PA chain broken; other → error.
	result, verifyErr := s.PDFCore.Verify(ctx, pdfBytes)
	match := verifyErr == nil
	c2paManifestFound := verifyErr == nil || (verifyErr != nil && strings.Contains(verifyErr.Error(), "status 409"))
	c2paSignatureValid := verifyErr == nil

	// Query live revocation state from the XFSC status list (DCS-OR-C2PA-006).
	// VC bytes are returned directly by pdf-core — no PDF byte scanning required.
	statusListURI := ""
	statusListStatus := ""
	if result.VCProofValid && len(result.VCBytes) > 0 {
		statusListURI = provenance.ExtractStatusListURI(result.VCBytes)
		if cred, idx, ok := provenance.ExtractCredentialStatusFields(result.VCBytes); ok {
			httpClient := &http.Client{Timeout: 10 * time.Second}
			if status, err := provenance.QueryStatusListStatus(ctx, httpClient, cred, idx); err == nil {
				statusListStatus = status
			}
		}
	}

	return &pdfgen.PDFVerifyResult{
		Match:              match,
		C2paManifestFound:  c2paManifestFound,
		C2paSignatureValid: c2paSignatureValid,
		VcProofValid:       result.VCProofValid,
		StatusListURI:      ptrToString(statusListURI),
		StatusListStatus:   ptrToString(statusListStatus),
	}, nil
}

// appendAndCache issues a W3C VC, calls pdf-core /update to append C2PA + VC,
// stores the updated PDF in IPFS, and writes the new CID + state to the DB.
func (s *pdfGenerationSrvc) appendAndCache(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, table string,
) ([]byte, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return pdfBytes, fmt.Errorf("map lifecycle state %q: %w", state, err)
	}

	log.Printf("pdfgeneration: appendAndCache %s table=%s state=%s c2paState=%s pdfLen=%d",
		did, table, state, c2paState, len(pdfBytes))

	reason := stateToReason(c2paState)

	// Compute asset hash for the VC credentialSubject (DCS-OR-C2PA-004).
	h := sha256.Sum256(pdfBytes)
	assetHash := hex.EncodeToString(h[:])

	// Issue W3C VC for this lifecycle event (DCS-OR-C2PA-004).
	// Status list publication is atomic with VC issuance (DCS-OR-C2PA-005).
	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, s.IssuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	// pdf-core appends a C2PA incremental update embedding the VC attachment.
	// When vcBytes is provided, pdf-core bypasses the "no-changes" guard —
	// this covers the genesis VC attachment case (same JSON-LD as /download).
	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes)
	if err != nil {
		return pdfBytes, fmt.Errorf("pdf-core update for %s: %w", did, err)
	}

	// Store updated PDF in IPFS.
	ipfsResult, err := s.IPFSClient.CreateFile(ctx, base64.StdEncoding.EncodeToString(updatedPDF))
	if err != nil {
		return updatedPDF, fmt.Errorf("store PDF in IPFS for %s: %w", did, err)
	}
	pdfCID := ipfsResult.Identifier.Value

	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3 WHERE did=$4`, table),
		pdfCID, rendererVersion, c2paState, did,
	); err != nil {
		return nil, fmt.Errorf("update %s pdf_ipfs_cid: %w", table, err)
	}
	log.Printf("pdfgeneration: appendAndCache %s done → CID=%s pdfLen=%d", did, pdfCID, len(updatedPDF))
	return updatedPDF, nil
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

// ptrToString converts a string to a *string pointer, returning nil for empty strings.
func ptrToString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}



