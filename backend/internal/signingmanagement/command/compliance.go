package command

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type ComplianceCmd struct {
	DID         string
	ValidatedBy string
}

type ComplianceValidator struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *ComplianceValidator) Handle(ctx context.Context, cmd ComplianceCmd) error {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err := h.CRepo.ReadProcessData(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	evt := signingmanagementevents.ComplianceValidationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ValidatedBy:     cmd.ValidatedBy,
		OccurredAt:      time.Now(),
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
