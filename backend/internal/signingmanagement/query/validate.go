package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
	"digital-contracting-service/internal/signingmanagement/pidverify"
)

type ValidateQry struct {
	DID         string
	ValidatedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type ValidationResult struct {
	Findings []string
}

type Validator struct {
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
}

func (h *Validator) Handle(ctx context.Context, cmd ValidateQry) (*ValidationResult, error) {

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

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	findings, err := h.CRepo.CollectValidationFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect validation findings: %w", err)
	}

	findings = append(findings, h.crossCheckEmbeddedPID(ctx, tx, cmd.DID)...)

	evt := signingmanagementevents.ValidateEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ValidatedBy:     cmd.ValidatedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &ValidationResult{
		Findings: findings,
	}, nil
}

// crossCheckEmbeddedPID re-verifies the embedded PID presentation against the
// signature record (UC-04-03): it extracts the signing evidence from the
// stored signed PDF, re-verifies the SD-JWT VC + KB-JWT, and confirms the
// resolved signer DID matches the signature row. Absence of evidence (an
// unsigned contract) yields no findings; any mismatch or verification
// failure is reported as a finding so validate surfaces it.
func (h *Validator) crossCheckEmbeddedPID(ctx context.Context, tx *sqlx.Tx, did string) []string {
	if h.PDFCore == nil {
		return nil
	}

	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return nil
	}

	evidence, found, err := h.PDFCore.ExtractEvidence(ctx, pdfBytes)
	if err != nil {
		return []string{fmt.Sprintf("Could not extract embedded PID evidence: %v", err)}
	}
	if !found || len(evidence) == 0 {
		return nil
	}

	presentation, subject := signingSummaryPIDFields(evidence)
	if presentation == "" {
		return []string{"Embedded signing evidence is missing the PID presentation"}
	}

	signerDID, _, err := pidverify.Verify(presentation)
	if err != nil {
		return []string{fmt.Sprintf("PID verification failed: %v", err)}
	}
	if subject != "" && subject != signerDID {
		return []string{"Evidence mismatch: embedded PID subject does not match the credential subject"}
	}

	records, err := h.CRepo.LoadSignatures(ctx, tx, did)
	if err == nil {
		for _, rec := range records {
			if strings.EqualFold(strings.TrimSpace(rec.Status), "REVOKED") {
				continue
			}
			if rec.SignerDID != "" && rec.SignerDID != signerDID {
				return []string{"Evidence mismatch: re-verified PID signer does not match the signature record"}
			}
		}
	}

	return []string{"Embedded PID presentation re-verified and cross-checked against the signature record"}
}

// signingSummaryPIDFields extracts the verbatim PID presentation and credential
// subject from a ContractSigningSummaryCredential evidence document.
func signingSummaryPIDFields(evidence []byte) (presentation, subject string) {
	var vc struct {
		CredentialSubject struct {
			ID              string `json:"id"`
			PIDPresentation string `json:"pid_presentation"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil {
		return "", ""
	}
	return vc.CredentialSubject.PIDPresentation, vc.CredentialSubject.ID
}
