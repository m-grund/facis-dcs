package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type ComplianceCmd struct {
	DID       string
	CheckedBy string
	HolderDID string
	UserRoles userrole.UserRoles
}

type ComplianceValidator struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle evaluates the contract's signatures against the signature
// compliance policy (DCS-FR-SM-21: signature level SES/AES/QES, signature
// status, presence of active signed credentials) and returns the findings;
// the check itself — findings included — is recorded as an audit event.
func (h *ComplianceValidator) Handle(ctx context.Context, cmd ComplianceCmd) ([]string, error) {

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

	findings, err := h.CRepo.CollectComplianceFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect compliance findings: %w", err)
	}

	evt := signingmanagementevents.ComplianceValidationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		CheckedBy:       cmd.CheckedBy,
		Findings:        findings,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return findings, nil
}
