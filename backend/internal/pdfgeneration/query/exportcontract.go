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
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type ExportContractPdfQry struct {
	DID string
}

type ExportContractPdfHandler struct {
	DB         *sqlx.DB
	CRepo      cwedb.ContractRepo
	IPFSClient *ipfs.APIClient
	PDFCore    *pdfcore.Client
	VCIssuer   provenance.VCIssuer
	IssuerDID  string
}

func (h *ExportContractPdfHandler) Handle(ctx context.Context, qry ExportContractPdfQry) (io.ReadCloser, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("contract %s: %w", qry.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}

	pdfState, err := h.CRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read cached contract PDF state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	log.Printf("pdfgeneration: ExportContractPdf %s cidStr=%q lastC2PAState=%q currentState=%s c2paState=%q",
		qry.DID, pdfState.IPFSCID, pdfState.C2PAState, contract.State, currentC2PAState)

	updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
		return h.CRepo.UpdatePDFState(ctx, tx, did, cwedb.ContractPDFState{
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
		log.Printf("pdfgeneration: ExportContractPdf %s state matches — returning cached PDF (%d bytes)", qry.DID, len(r.Data))
		return io.NopCloser(bytes.NewReader(r.Data)), nil
	}

	if pdfState.IPFSCID != "" {
		log.Printf("pdfgeneration: ExportContractPdf %s state changed %q→%q; appending", qry.DID, pdfState.C2PAState, currentC2PAState)
		r, err := h.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(r.Data) == 0 {
			return nil, fmt.Errorf("fetch PDF from IPFS %s for update: %w", pdfState.IPFSCID, err)
		}
		pdfBytes, err := appendAndCache(ctx, tx, qry.DID, contract.State, jsonldBytes, r.Data,
			h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
		if err != nil {
			return nil, fmt.Errorf("append C2PA assertion for contract %s: %w", qry.DID, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit contract PDF append tx for %s: %w", qry.DID, err)
		}
		return io.NopCloser(bytes.NewReader(pdfBytes)), nil
	}

	pdfBytes, _, err := h.PDFCore.Download(ctx, jsonldBytes)
	if err != nil {
		return nil, fmt.Errorf("pdf-core download for contract %s: %w", qry.DID, err)
	}

	pdfBytes, err = appendAndCache(ctx, tx, qry.DID, contract.State, jsonldBytes, pdfBytes,
		h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
	if err != nil {
		return nil, fmt.Errorf("append and cache contract PDF for %s: %w", qry.DID, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit contract PDF export tx for %s: %w", qry.DID, err)
	}
	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}
