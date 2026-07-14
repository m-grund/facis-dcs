package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/bundleexport"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
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
	SignRepo   bundleexport.SignatureLoader
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
	signRepo bundleexport.SignatureLoader,
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
		SignRepo:         signRepo,
		PDFCore:          pdfCore,
		IssuerDID:        issuerDID,
		VCIssuer:         vcIssuer,
		JWTAuthenticator: jwtAuth,
	}
}

func (s *pdfGenerationSrvc) newBundler() *bundleexport.Bundler {
	return &bundleexport.Bundler{
		DB:         s.DB,
		CRepo:      s.CRepo,
		TRepo:      s.TRepo,
		SignRepo:   s.SignRepo,
		IPFSClient: s.IPFSClient,
		PDFCore:    s.PDFCore,
		VCIssuer:   s.VCIssuer,
		IssuerDID:  s.IssuerDID,
	}
}

// bundleZipContentType is the fixed media type of every bundle export body.
const bundleZipContentType = "application/zip"

func (s *pdfGenerationSrvc) ExportContractBundle(ctx context.Context, p *pdfgen.ExportContractBundlePayload) (*pdfgen.ExportContractBundleResult, io.ReadCloser, error) {
	body, err := s.newBundler().ExportContract(ctx, p.Did, bundleexport.ExportContext{
		ExportedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	})
	if err != nil {
		if refused, ok := bundleexport.AsRefused(err); ok {
			return nil, nil, &pdfgen.BundleExportRefusedError{
				Name:     "refused",
				Message:  fmt.Sprintf("contract bundle export for %s refused by structural-integrity pre-flight", p.Did),
				Findings: refused.Findings,
			}
		}
		if isNotFoundErr(err) {
			return nil, nil, pdfgen.MakeNotFound(err)
		}
		return nil, nil, pdfgen.MakeInternalError(fmt.Errorf("export contract bundle %s: %w", p.Did, err))
	}
	ct := bundleZipContentType
	return &pdfgen.ExportContractBundleResult{ContentType: &ct}, body, nil
}

func (s *pdfGenerationSrvc) ExportTemplateBundle(ctx context.Context, p *pdfgen.ExportTemplateBundlePayload) (*pdfgen.ExportTemplateBundleResult, io.ReadCloser, error) {
	body, err := s.newBundler().ExportTemplate(ctx, p.Did)
	if err != nil {
		if refused, ok := bundleexport.AsRefused(err); ok {
			return nil, nil, &pdfgen.BundleExportRefusedError{
				Name:     "refused",
				Message:  fmt.Sprintf("template bundle export for %s refused by structural-integrity pre-flight", p.Did),
				Findings: refused.Findings,
			}
		}
		if isNotFoundErr(err) {
			return nil, nil, pdfgen.MakeNotFound(err)
		}
		return nil, nil, pdfgen.MakeInternalError(fmt.Errorf("export template bundle %s: %w", p.Did, err))
	}
	ct := bundleZipContentType
	return &pdfgen.ExportTemplateBundleResult{ContentType: &ct}, body, nil
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
