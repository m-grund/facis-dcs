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

type RevokeCmd struct {
	DID       string
	RevokedBy string
}

type Revoker struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *Revoker) Handle(ctx context.Context, cmd RevokeCmd) error {

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

	evt := signingmanagementevents.RevokeEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RevokedBy:       cmd.RevokedBy,
		OccurredAt:      time.Now(),
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
