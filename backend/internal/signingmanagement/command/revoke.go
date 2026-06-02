package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type RevokeCmd struct {
	DID       string
	RevokedBy string
	Username  string
	UserRoles userrole.UserRoles
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
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	evt := signingmanagementevents.RevokeEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RevokedBy:       cmd.RevokedBy,
		OccurredAt:      time.Now(),
		Username:        cmd.Username,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
