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
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type VerifyContractPdfQry struct {
	DID string
}

type VerifyContractPdfHandler struct {
	DB         *sqlx.DB
	CRepo      cwedb.ContractRepo
	IPFSClient *ipfs.APIClient
	PDFCore    *pdfcore.Client
	VCIssuer   provenance.VCIssuer
	IssuerDID  string
}

func (h *VerifyContractPdfHandler) Handle(ctx context.Context, qry VerifyContractPdfQry) (*pdfgen.PDFVerifyResult, error) {
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

	pdfState, err := h.CRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read contract PDF verification state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	latestCID := pdfState.IPFSCID

	if pdfState.IPFSCID != "" && pdfState.C2PAState != currentC2PAState {
		log.Printf("pdfgeneration: VerifyContractPdf %s state advanced %q→%q; appending before verify",
			qry.DID, pdfState.C2PAState, currentC2PAState)

		var jsonldBytes []byte
		if contract.ContractData != nil {
			jsonldBytes = []byte(*contract.ContractData)
		}

		r, err := h.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(r.Data) == 0 {
			return nil, fmt.Errorf("fetch cached contract PDF %s from IPFS for verify append: %w", qry.DID, err)
		}

		updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
			return h.CRepo.UpdatePDFState(ctx, tx, did, cwedb.ContractPDFState{
				IPFSCID:         state.IPFSCID,
				RendererVersion: state.RendererVersion,
				C2PAState:       state.C2PAState,
			})
		}

		updatedPDF, err := appendAndCache(ctx, tx, qry.DID, contract.State, jsonldBytes, r.Data,
			h.IPFSClient, h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
		if err != nil {
			return nil, fmt.Errorf("append contract C2PA assertion before verify for %s: %w", qry.DID, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit pre-verify append tx for contract %s: %w", qry.DID, err)
		}

		return runVerify(ctx, updatedPDF, h.PDFCore)
	}

	if latestCID == "" {
		return nil, fmt.Errorf("no cached PDF for contract %s; call export first", qry.DID)
	}

	r, err := h.IPFSClient.FetchFile(latestCID)
	if err != nil || len(r.Data) == 0 {
		return nil, fmt.Errorf("fetch contract PDF %s from IPFS for verify: %w", qry.DID, err)
	}

	return runVerify(ctx, r.Data, h.PDFCore)
}
