package command

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type CopyCmd struct {
	NewDID   string
	CopyDID  string
	CopiedBy string
	Username string
}

type Copier struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *Copier) Handle(ctx context.Context, cmd CopyCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	version, err := h.CTRepo.CopyFromDID(ctx, tx, cmd.CopyDID, cmd.NewDID)
	if err != nil {
		return fmt.Errorf("could not copy contract template: %w", err)
	}

	evt := templateevents.CopyEvent{
		NewDID:     cmd.NewDID,
		CopyDID:    cmd.CopyDID,
		CopiedBy:   cmd.CopiedBy,
		NewVersion: version,
		OccurredAt: time.Now(),
		Username:   cmd.Username,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
