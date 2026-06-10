package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/pdfgeneration/verify"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	event2 "digital-contracting-service/internal/signingmanagement/event"

	"github.com/jmoiron/sqlx"
)

// SignatureVerifyQry carries the inputs for verifying a contract's signatures.
type SignatureVerifyQry struct {
	DID        string
	VerifiedBy string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

// SignatureVerifyResult holds the signature verification summary.
type SignatureVerifyResult struct {
	Match    bool
	Findings []string
	// SigCount is the number of non-revoked signatures on the contract.
	SigCount    int
	JsonldHash  *string
	BasePdfHash *string
}

// SignatureVerifier handles the SignatureVerifyQry command.
type SignatureVerifier struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle verifies that the contract is APPROVED and returns the count of
// active (non-revoked) signatures. Hash comparison is performed at the
// service layer where PDF bytes are available.
func (h *SignatureVerifier) Handle(ctx context.Context, cmd SignatureVerifyQry) (*SignatureVerifyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Validates APPROVED state via repo filter.
	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("contract %s not available for verification: %w", cmd.DID, err)
	}

	sigCount, err := h.CRepo.CountSignatureForContractDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not count signature for contract %s: %w", cmd.DID, err)
	}

	// Fetch PDF bytes and run MR/HR hash check (DCS-FR-CWE-04).
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, cmd.DID)
	if err != nil || len(pdfBytes) == 0 {
		// No PDF yet — return match=false with sig count only.
		return &SignatureVerifyResult{
			Match:    false,
			SigCount: sigCount,
		}, nil
	}

	contractVerifier := &verify.ContractVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return h.CRepo.RebuildContractPDFFromJSONLD(ctx, tx, cmd.DID, jsonld)
		},
		FetchFn:         h.CRepo.ContractIPFSFetchFn(ctx, tx, cmd.DID),
		FetchManifestFn: h.CRepo.ContractManifestIPFSFetchFn(ctx, tx, cmd.DID),
		CheckStatusFn:   h.CRepo.StatusListCheckFn(ctx, tx),
	}
	hashResult, err := contractVerifier.Verify(pdfBytes)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("verify PDF: %w", err))
	}

	evt := event2.VerifyEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		VerifiedBy:      cmd.VerifiedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	findings := make([]string, 0, 4)
	if hashResult.PDFSignatureCount == 0 {
		findings = append(findings, "No PDF signature found")
	} else if hashResult.PDFSignatureValid {
		findings = append(findings, fmt.Sprintf("PDF signature verification passed (%d signature(s))", hashResult.PDFSignatureCount))
	} else {
		findings = append(findings, fmt.Sprintf("PDF signature verification failed (%d signature(s))", hashResult.PDFSignatureCount))
	}
	if !hashResult.C2PAManifestFound {
		findings = append(findings, "C2PA manifest not found")
	} else if !hashResult.C2PASignatureValid {
		findings = append(findings, "C2PA signature invalid")
	}
	if !hashResult.VCProofValid {
		findings = append(findings, "VC proof invalid or missing")
	}
	if status := strings.TrimSpace(hashResult.StatusListStatus); status != "" {
		findings = append(findings, fmt.Sprintf("Status list state: %s", status))
	}

	return &SignatureVerifyResult{
		Match:       hashResult.Match,
		Findings:    findings,
		SigCount:    sigCount,
		JsonldHash:  &hashResult.JSONLDHash,
		BasePdfHash: &hashResult.BasePDFHash,
	}, nil
}
