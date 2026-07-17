package query

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/ipfs"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// A contract PDF is produced only by the event-driven background regenerator
// (pdfgeneration/event). Export serves the current cached PDF, waits while a
// regeneration triggered by the latest change is still in flight, and never
// returns the stale cache — nor renders one on demand.
const (
	pdfExportWaitTimeout  = 25 * time.Second
	pdfExportPollInterval = 400 * time.Millisecond
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
	contract, err := h.readContract(ctx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("contract %s: %w", qry.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}
	currentPayloadHash := payloadHash(jsonldBytes)
	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	deadline := time.Now().Add(pdfExportWaitTimeout)
	for {
		pdfState, err := h.readPDFState(ctx, qry.DID)
		if err != nil {
			return nil, fmt.Errorf("read contract PDF state for %s: %w", qry.DID, err)
		}

		// A PAdES-signed PDF is frozen: serve it as-is regardless of later
		// lifecycle bookkeeping — post-signature mutation breaks PAdES validators.
		if pdfState.IPFSCID != "" && isFrozenC2PAState(pdfState.C2PAState) {
			return h.fetch(qry.DID, pdfState.IPFSCID)
		}
		// Serve only when the cached PDF reflects the current content and state.
		if pdfState.IPFSCID != "" && pdfState.C2PAState == currentC2PAState && pdfState.PayloadHash == currentPayloadHash {
			return h.fetch(qry.DID, pdfState.IPFSCID)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("contract %s PDF is being regenerated after the latest change; retry shortly", qry.DID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pdfExportPollInterval):
		}
	}
}

func (h *ExportContractPdfHandler) fetch(did, cid string) (io.ReadCloser, error) {
	r, err := h.IPFSClient.FetchFile(cid)
	if err != nil || len(r.Data) == 0 {
		return nil, fmt.Errorf("fetch PDF from IPFS %s for contract %s: %w", cid, did, err)
	}
	return io.NopCloser(bytes.NewReader(r.Data)), nil
}

func (h *ExportContractPdfHandler) readContract(ctx context.Context, did string) (*cwedb.Contract, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return h.CRepo.ReadDataByDID(ctx, tx, did)
}

func (h *ExportContractPdfHandler) readPDFState(ctx context.Context, did string) (*cwedb.ContractPDFState, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return h.CRepo.ReadPDFState(ctx, tx, did)
}
