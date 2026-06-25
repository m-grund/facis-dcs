package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type RemoteCreateCmd struct {
	DID             string
	Origin          string
	ContractVersion int
	State           contractstate.ContractState
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Name            *string
	Description     *string
	Responsible     *db.Responsible
	ContractData    *datatype.JSON
	TemplateDID     string
	TemplateVersion int
}

type RemoteCreator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *RemoteCreator) Handle(ctx context.Context, cmd RemoteCreateCmd) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var expPolicy *string
	if cmd.ExpPolicy != nil {
		policy := string(*cmd.ExpPolicy)
		expPolicy = &policy
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          cmd.Origin,
		CreatedBy:       cmd.CreatedBy,
		State:           cmd.State.String(),
		ContractData:    cmd.ContractData,
		TemplateDID:     cmd.TemplateDID,
		TemplateVersion: cmd.TemplateVersion,
		Responsible:     cmd.Responsible,
		Name:            cmd.Name,
		Description:     cmd.Description,
		StartDate:       cmd.StartDate,
		ExpDate:         cmd.ExpDate,
		ExpNoticePeriod: cmd.ExpNoticePeriod,
		ExpPolicy:       expPolicy,
		UpdatedAt:       cmd.UpdatedAt,
		CreatedAt:       cmd.CreatedAt,
		ContractVersion: cmd.ContractVersion,
	}
	createdAt, err := h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	evt := contractevents.RemoteCreateEvent{
		DID:             cmd.DID,
		TemplateDID:     cmd.TemplateDID,
		CreatedBy:       cmd.CreatedBy,
		ContractData:    cmd.ContractData,
		OccurredAt:      *createdAt,
		Responsible:     cmd.Responsible,
		Name:            cmd.Name,
		Description:     cmd.Description,
		StartDate:       cmd.StartDate,
		ExpDate:         cmd.ExpDate,
		ExpPolicy:       cmd.ExpPolicy,
		Origin:          cmd.Origin,
		CreatedAt:       *createdAt,
		UpdatedAt:       *createdAt,
		ExpNoticePeriod: cmd.ExpPolicy,
		TemplateVersion: cmd.TemplateVersion,
		ContractVersion: cmd.ContractVersion,
		State:           cmd.State,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
