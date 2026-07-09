package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	c2paservice "digital-contracting-service/gen/c2_pa_service"
	"digital-contracting-service/internal/base/ipfs"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/manifest"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	pdfquery "digital-contracting-service/internal/pdfgeneration/query"

	"github.com/jmoiron/sqlx"
)

// c2paManifestMediaType is the C2PA manifest store media type (DCS-OR-C2PA-008).
const c2paManifestMediaType = "application/c2pa"

type c2paSrvc struct {
	DB         *sqlx.DB
	IPFSClient *ipfs.APIClient
	CRepo      cwedb.ContractRepo
	PDFCore    *pdfcore.Client
	IssuerDID  string
	VCIssuer   provenance.VCIssuer
}

// NewC2PAService wires the public C2PA manifest endpoint (DCS-OR-C2PA-008,
// Workstream D). It reuses the same PDF export dependencies as PDFGeneration so
// it can fetch the current/cached signed PDF and extract its embedded C2PA
// manifest store.
func NewC2PAService(
	db *sqlx.DB,
	ipfsClient *ipfs.APIClient,
	cRepo cwedb.ContractRepo,
	pdfCore *pdfcore.Client,
	issuerDID string,
	vcIssuer provenance.VCIssuer,
) c2paservice.Service {
	if vcIssuer == nil {
		panic("VCIssuer is required for DCS-OR-C2PA-004 compliance")
	}
	if pdfCore == nil {
		panic("PDFCore client is required")
	}
	return &c2paSrvc{
		DB:         db,
		IPFSClient: ipfsClient,
		CRepo:      cRepo,
		PDFCore:    pdfCore,
		IssuerDID:  issuerDID,
		VCIssuer:   vcIssuer,
	}
}

// GetManifest returns the raw C2PA JUMBF manifest store bytes for a
// signed/exported contract (Content-Type application/c2pa), or — when
// ?history=true — a parsed JSON enumeration of the manifest chain.
func (s *c2paSrvc) GetManifest(ctx context.Context, p *c2paservice.GetManifestPayload) (*c2paservice.GetManifestResult, io.ReadCloser, error) {
	// Fetch the current/cached PDF through the same export path
	// export_contract_pdf uses, then extract its embedded C2PA manifest store.
	exportHandler := pdfquery.ExportContractPdfHandler{
		DB:         s.DB,
		CRepo:      s.CRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
	pdfReader, err := exportHandler.Handle(ctx, pdfquery.ExportContractPdfQry{DID: p.ContractDid})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil, c2paservice.MakeNotFound(err)
		}
		return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("export contract PDF %s for C2PA manifest: %w", p.ContractDid, err))
	}
	defer func() { _ = pdfReader.Close() }()

	pdfBytes, err := io.ReadAll(pdfReader)
	if err != nil {
		return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("read exported PDF for %s: %w", p.ContractDid, err))
	}

	manifestBytes, err := s.PDFCore.ExtractManifest(ctx, pdfBytes)
	if err != nil {
		return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("extract C2PA manifest for %s: %w", p.ContractDid, err))
	}
	if len(manifestBytes) == 0 {
		return nil, nil, c2paservice.MakeNotFound(fmt.Errorf("no C2PA manifest embedded in PDF for contract %s", p.ContractDid))
	}

	if p.History != nil && *p.History {
		chain, err := manifest.ParseChain(manifestBytes)
		if err != nil {
			return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("parse C2PA manifest chain for %s: %w", p.ContractDid, err))
		}
		body, err := json.Marshal(chain)
		if err != nil {
			return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("encode C2PA manifest chain for %s: %w", p.ContractDid, err))
		}
		ct := "application/json"
		return &c2paservice.GetManifestResult{ContentType: &ct}, io.NopCloser(bytes.NewReader(body)), nil
	}

	ct := c2paManifestMediaType
	return &c2paservice.GetManifestResult{ContentType: &ct}, io.NopCloser(bytes.NewReader(manifestBytes)), nil
}
