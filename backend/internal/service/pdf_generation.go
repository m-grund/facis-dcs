package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/ipfs"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	pdfquery "digital-contracting-service/internal/pdfgeneration/query"
	tpldb "digital-contracting-service/internal/templaterepository/db"

	"github.com/jmoiron/sqlx"
)

type pdfGenerationSrvc struct {
	DB         *sqlx.DB
	IPFSClient *ipfs.APIClient
	CRepo      cwedb.ContractRepo
	TRepo      tpldb.ContractTemplateRepo
	PDFCore    *pdfcore.Client
	IssuerDID  string
	VCIssuer   provenance.VCIssuer
	auth.JWTAuthenticator
}

func NewPDFGeneration(
	db *sqlx.DB,
	jwtAuth auth.JWTAuthenticator,
	ipfsClient *ipfs.APIClient,
	cRepo cwedb.ContractRepo,
	tRepo tpldb.ContractTemplateRepo,
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

func (s *pdfGenerationSrvc) ExportContractPdf(ctx context.Context, p *pdfgen.ExportContractPdfPayload) (io.ReadCloser, error) {
	handler := pdfquery.ExportContractPdfHandler{
		DB:         s.DB,
		CRepo:      s.CRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
	result, err := handler.Handle(ctx, pdfquery.ExportContractPdfQry{DID: p.Did})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, pdfgen.MakeNotFound(err)
		}
		return nil, pdfgen.MakeInternalError(fmt.Errorf("export contract PDF %s: %w", p.Did, err))
	}
	return result, nil
}

func (s *pdfGenerationSrvc) ExportTemplatePdf(ctx context.Context, p *pdfgen.ExportTemplatePdfPayload) (io.ReadCloser, error) {
	handler := pdfquery.ExportTemplatePdfHandler{
		DB:         s.DB,
		TRepo:      s.TRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
	result, err := handler.Handle(ctx, pdfquery.ExportTemplatePdfQry{DID: p.Did})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, pdfgen.MakeNotFound(err)
		}
		return nil, pdfgen.MakeInternalError(fmt.Errorf("export template PDF %s: %w", p.Did, err))
	}
	return result, nil
}

func (s *pdfGenerationSrvc) VerifyContractPdf(ctx context.Context, p *pdfgen.VerifyContractPdfPayload) (*pdfgen.PDFVerifyResult, error) {
	handler := pdfquery.VerifyContractPdfHandler{
		DB:         s.DB,
		CRepo:      s.CRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
	result, err := handler.Handle(ctx, pdfquery.VerifyContractPdfQry{DID: p.Did})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, pdfgen.MakeNotFound(err)
		}
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify contract PDF %s: %w", p.Did, err))
	}
	return result, nil
}

func (s *pdfGenerationSrvc) VerifyTemplatePdf(ctx context.Context, p *pdfgen.VerifyTemplatePdfPayload) (*pdfgen.PDFVerifyResult, error) {
	handler := pdfquery.VerifyTemplatePdfHandler{
		DB:         s.DB,
		TRepo:      s.TRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
	result, err := handler.Handle(ctx, pdfquery.VerifyTemplatePdfQry{DID: p.Did})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, pdfgen.MakeNotFound(err)
		}
		return nil, pdfgen.MakeInternalError(fmt.Errorf("verify template PDF %s: %w", p.Did, err))
	}
	return result, nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no rows")
}
