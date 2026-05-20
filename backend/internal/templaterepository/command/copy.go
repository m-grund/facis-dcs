package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type CopyCmd struct {
	NewDID   string
	CopyDID  string
	CopiedBy string
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
	defer tx.Rollback()

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
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
