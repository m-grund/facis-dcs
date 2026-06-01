package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type CreateCmd struct {
	DID          string
	TemplateDID  string
	CreatedBy    string
	Name         *string
	Description  *string
	ContractData *datatype.JSON
	Username     string
}

type Creator struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data := db.Contract{
		DID:          cmd.DID,
		CreatedBy:    cmd.CreatedBy,
		State:        contractstate.Draft.String(),
		Name:         cmd.Name,
		Description:  cmd.Description,
		ContractData: cmd.ContractData,
	}
	createdAt, err := h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	evt := contractevents.CreateEvent{
		DID:          cmd.DID,
		TemplateDID:  cmd.TemplateDID,
		CreatedBy:    cmd.CreatedBy,
		Name:         cmd.Name,
		Description:  cmd.Description,
		ContractData: cmd.ContractData,
		OccurredAt:   *createdAt,
		Username:     cmd.Username,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
