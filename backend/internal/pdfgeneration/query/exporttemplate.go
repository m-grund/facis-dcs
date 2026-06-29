package query

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

type ExportTemplatePdfQry struct {
	DID string
}

type ExportTemplatePdfHandler struct {
	DB         *sqlx.DB
	TRepo      tpldb.ContractTemplateRepo
	IPFSClient *ipfs.APIClient
	PDFCore    *pdfcore.Client
	VCIssuer   provenance.VCIssuer
	IssuerDID  string
}

func (h *ExportTemplatePdfHandler) Handle(ctx context.Context, qry ExportTemplatePdfQry) (io.ReadCloser, error) {
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

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		jsonldBytes, err = pdfgeneration.MarshalJSONLD([]byte(*tpl.TemplateData))
		if err != nil {
			return nil, fmt.Errorf("marshal template JSON-LD: %w", err)
		}
	}

	pdfState, err := h.TRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read cached template PDF state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
	}

	log.Printf("pdfgeneration: ExportTemplatePdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		qry.DID, pdfState.IPFSCID, pdfState.C2PAState, tpl.State, currentC2PAState)

	updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
		return h.TRepo.UpdatePDFState(ctx, tx, did, tpldb.ContractTemplatePDFState{
			IPFSCID:         state.IPFSCID,
			RendererVersion: state.RendererVersion,
			C2PAState:       state.C2PAState,
		})
	}

	if pdfState.IPFSCID != "" && pdfState.C2PAState == currentC2PAState {
		r, err := h.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(r.Data) == 0 {
			return nil, fmt.Errorf("fetch cached PDF from IPFS %s: %w", pdfState.IPFSCID, err)
		}
		log.Printf("pdfgeneration: ExportTemplatePdf %s state matches — returning cached PDF (%d bytes)", qry.DID, len(r.Data))
		return io.NopCloser(bytes.NewReader(r.Data)), nil
	}

	if pdfState.IPFSCID != "" {
		log.Printf("pdfgeneration: ExportTemplatePdf %s state changed %q→%q; appending", qry.DID, pdfState.C2PAState, currentC2PAState)
		r, err := h.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(r.Data) == 0 {
			return nil, fmt.Errorf("fetch PDF from IPFS %s for update: %w", pdfState.IPFSCID, err)
		}
		pdfBytes, err := appendAndCache(ctx, tx, qry.DID, tpl.State, jsonldBytes, r.Data,
			h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
		if err != nil {
			return nil, fmt.Errorf("append C2PA assertion for template %s: %w", qry.DID, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit template PDF append tx for %s: %w", qry.DID, err)
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}

	pdfBytes, _, err := h.PDFCore.Download(ctx, jsonldBytes)
	if err != nil {
		return nil, fmt.Errorf("pdf-core download for template %s: %w", qry.DID, err)
	}

	pdfBytes, err = appendAndCache(ctx, tx, qry.DID, tpl.State, jsonldBytes, pdfBytes,
		h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
	if err != nil {
		return nil, fmt.Errorf("append and cache template PDF for %s: %w", qry.DID, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit template PDF export tx for %s: %w", qry.DID, err)
	}
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}
