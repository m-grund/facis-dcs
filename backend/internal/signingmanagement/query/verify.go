package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
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
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
}

// Handle verifies that the contract is APPROVED and returns the count of
// active (non-revoked) signatures.
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

	// pdf-core /verify: re-renders JSON-LD and compares, validates C2PA chain.
	// 200 → intact; 409 → content mismatch; other → C2PA invalid.
	verifyResult, verifyErr := h.PDFCore.Verify(ctx, pdfBytes)
	match := verifyErr == nil
	c2paManifestFound := verifyErr == nil || (verifyErr != nil && strings.Contains(verifyErr.Error(), "status 409"))
	c2paSignatureValid := verifyErr == nil

	// Query live revocation state from the XFSC status list (DCS-OR-C2PA-006).
	// VC bytes are returned directly by pdf-core — no PDF byte scanning required.
	statusListStatus := ""
	if verifyResult.VCProofValid && len(verifyResult.VCBytes) > 0 {
		if cred, idx, ok := provenance.ExtractCredentialStatusFields(verifyResult.VCBytes); ok {
			httpClient := &http.Client{Timeout: 10 * time.Second}
			if status, statusErr := provenance.QueryStatusListStatus(ctx, httpClient, cred, idx); statusErr == nil {
				statusListStatus = status
			}
		}
	}

	evt := event2.VerifyEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		VerifiedBy:      cmd.VerifiedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	if err = event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	findings := make([]string, 0, 4)
	if !c2paManifestFound {
		findings = append(findings, "C2PA manifest not found")
	} else if !c2paSignatureValid {
		findings = append(findings, "C2PA signature invalid")
	}
	if !verifyResult.VCProofValid {
		findings = append(findings, "VC proof invalid or missing")
	}
	if status := strings.TrimSpace(statusListStatus); status != "" {
		findings = append(findings, fmt.Sprintf("Status list state: %s", status))
	}

	return &SignatureVerifyResult{
		Match:    match,
		Findings: findings,
		SigCount: sigCount,
	}, nil
}
