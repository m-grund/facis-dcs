package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

type VerifyTemplatePdfQry struct {
	DID string
}

type VerifyTemplatePdfHandler struct {
	DB         *sqlx.DB
	TRepo      tpldb.ContractTemplateRepo
	IPFSClient *ipfs.APIClient
	PDFCore    *pdfcore.Client
	VCIssuer   provenance.VCIssuer
	IssuerDID  string
}

func (h *VerifyTemplatePdfHandler) Handle(ctx context.Context, qry VerifyTemplatePdfQry) (*pdfgen.PDFVerifyResult, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := h.TRepo.ReadDataByID(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", qry.DID, err)
	}

	pdfState, err := h.TRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read template PDF verification state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
	}

	latestCID := pdfState.IPFSCID

	if pdfState.IPFSCID != "" && pdfState.C2PAState != currentC2PAState {
		log.Printf("pdfgeneration: VerifyTemplatePdf %s state advanced %q→%q; appending before verify",
			qry.DID, pdfState.C2PAState, currentC2PAState)

		var jsonldBytes []byte
		if tpl.TemplateData != nil {
			jsonldBytes = []byte(*tpl.TemplateData)
		}

		r, err := h.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(r.Data) == 0 {
			return nil, fmt.Errorf("fetch cached template PDF %s from IPFS for verify append: %w", qry.DID, err)
		}

		updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
			return h.TRepo.UpdatePDFState(ctx, tx, did, tpldb.ContractTemplatePDFState{
				IPFSCID:         state.IPFSCID,
				RendererVersion: state.RendererVersion,
				C2PAState:       state.C2PAState,
			})
		}

		updatedPDF, err := appendAndCache(ctx, tx, qry.DID, tpl.State, jsonldBytes, r.Data,
			h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
		if err != nil {
			return nil, fmt.Errorf("append template C2PA assertion before verify for %s: %w", qry.DID, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit pre-verify append tx for template %s: %w", qry.DID, err)
		}

		return runVerify(ctx, updatedPDF, h.PDFCore, currentC2PAState)
	}

	if latestCID == "" {
		return nil, fmt.Errorf("no cached PDF for template %s; call export first", qry.DID)
	}

	r, err := h.IPFSClient.FetchFile(latestCID)
	if err != nil || len(r.Data) == 0 {
		return nil, fmt.Errorf("fetch template PDF %s from IPFS for verify: %w", qry.DID, err)
	}

	return runVerify(ctx, r.Data, h.PDFCore, currentC2PAState)
}
