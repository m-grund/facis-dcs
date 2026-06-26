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

type RemoteContractData struct {
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

type RemoteCreateCmd struct {
	Contract RemoteContractData
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
	if cmd.Contract.ExpPolicy != nil {
		policy := string(*cmd.Contract.ExpPolicy)
		expPolicy = &policy
	}

	data := db.Contract{
		DID:             cmd.Contract.DID,
		Origin:          cmd.Contract.Origin,
		CreatedBy:       cmd.Contract.CreatedBy,
		State:           cmd.Contract.State.String(),
		ContractData:    cmd.Contract.ContractData,
		TemplateDID:     cmd.Contract.TemplateDID,
		TemplateVersion: cmd.Contract.TemplateVersion,
		Responsible:     cmd.Contract.Responsible,
		Name:            cmd.Contract.Name,
		Description:     cmd.Contract.Description,
		StartDate:       cmd.Contract.StartDate,
		ExpDate:         cmd.Contract.ExpDate,
		ExpNoticePeriod: cmd.Contract.ExpNoticePeriod,
		ExpPolicy:       expPolicy,
		UpdatedAt:       cmd.Contract.UpdatedAt,
		CreatedAt:       cmd.Contract.CreatedAt,
		ContractVersion: cmd.Contract.ContractVersion,
	}
	createdAt, err := h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	evt := contractevents.RemoteCreateEvent{
		DID:             cmd.Contract.DID,
		TemplateDID:     cmd.Contract.TemplateDID,
		CreatedBy:       cmd.Contract.CreatedBy,
		ContractData:    cmd.Contract.ContractData,
		OccurredAt:      *createdAt,
		Responsible:     cmd.Contract.Responsible,
		Name:            cmd.Contract.Name,
		Description:     cmd.Contract.Description,
		StartDate:       cmd.Contract.StartDate,
		ExpDate:         cmd.Contract.ExpDate,
		ExpPolicy:       cmd.Contract.ExpPolicy,
		Origin:          cmd.Contract.Origin,
		CreatedAt:       *createdAt,
		UpdatedAt:       *createdAt,
		ExpNoticePeriod: cmd.Contract.ExpPolicy,
		TemplateVersion: cmd.Contract.TemplateVersion,
		ContractVersion: cmd.Contract.ContractVersion,
		State:           cmd.Contract.State,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
