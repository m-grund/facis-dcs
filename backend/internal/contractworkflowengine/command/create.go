package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type CreateCmd struct {
	DID         string
	TemplateDID string
	CreatedBy   string
	HolderDID   string
	UserRoles   userrole.UserRoles
	DIDDocument base.DIDDocument
}

type Creator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
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

	contractTemplate, err := h.CTRepo.ReadFrameContractTemplateDataByID(ctx, tx, cmd.TemplateDID)
	if err != nil {
		return fmt.Errorf("could not read frame contract template data: %w", err)
	}

	normalizedContractData, err := validation.NormalizeContractDataForPersistence(contractTemplate.TemplateData, cmd.DID, nil, false)
	if err != nil {
		return fmt.Errorf("contract data validation failed: %w", err)
	}

	origin, err := cmd.DIDDocument.ExtractDomainAndPath()
	if err != nil {
		return fmt.Errorf("could not extract did: %w", err)
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          origin,
		CreatedBy:       cmd.CreatedBy,
		State:           contractstate.Draft.String(),
		ContractData:    normalizedContractData,
		TemplateDID:     cmd.TemplateDID,
		TemplateVersion: contractTemplate.TemplateVersion,
	}
	createdAt, err := h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	evt := contractevents.CreateEvent{
		DID:          cmd.DID,
		TemplateDID:  cmd.TemplateDID,
		CreatedBy:    cmd.CreatedBy,
		ContractData: normalizedContractData,
		OccurredAt:   *createdAt,
		HolderDID:    cmd.HolderDID,
		UserRoles:    cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
