package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type RemoteUpdateCmd struct {
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

type RemoteUpdater struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *RemoteUpdater) Handle(ctx context.Context, cmd RemoteUpdateCmd) error {

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
	newData := db.RemoteContractUpdateData{
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
		UpdatedAt:       cmd.UpdatedAt,
		CreatedAt:       cmd.CreatedAt,
		ExpPolicy:       expPolicy,
		ContractVersion: cmd.ContractVersion,
	}
	err = h.CRepo.RemoteUpdate(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update contract data: %w", err)
	}

	evt := contractevents.RemoteUpdateEvent{
		DID:             cmd.DID,
		TemplateDID:     cmd.TemplateDID,
		CreatedBy:       cmd.CreatedBy,
		ContractData:    cmd.ContractData,
		OccurredAt:      time.Now().UTC(),
		Responsible:     cmd.Responsible,
		Name:            cmd.Name,
		Description:     cmd.Description,
		StartDate:       cmd.StartDate,
		ExpDate:         cmd.ExpDate,
		ExpPolicy:       cmd.ExpPolicy,
		Origin:          cmd.Origin,
		CreatedAt:       cmd.CreatedAt,
		UpdatedAt:       cmd.UpdatedAt,
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
