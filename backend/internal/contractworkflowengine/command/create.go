package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"

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
	Reviewers   []string
	Approvers   []string
	Negotiators []string
	UserRoles   userrole.UserRoles
	DIDDocument base.DIDDocument
}

type Creator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func createTasks(ctx context.Context, tx *sqlx.Tx, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo, ntRepo db.NegotiationTaskRepo, cmd CreateCmd) error {
	for _, reviewer := range cmd.Reviewers {
		reviewTask := db.ReviewTaskData{
			DID:       cmd.DID,
			Reviewer:  reviewer,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: cmd.CreatedBy,
		}
		_, err := rtRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			return fmt.Errorf("could not create review task: %w", err)
		}
	}

	for _, negotiator := range cmd.Negotiators {
		negotiationTask := db.NegotiationTaskData{
			DID:        cmd.DID,
			Negotiator: negotiator,
			State:      reviewtaskstate.Open.String(),
			CreatedBy:  cmd.CreatedBy,
		}
		_, err := ntRepo.Create(ctx, tx, negotiationTask)
		if err != nil {
			return fmt.Errorf("could not create negotiation task: %w", err)
		}
	}

	for _, approver := range cmd.Approvers {
		data := db.ApprovalTaskData{
			DID:       cmd.DID,
			CreatedBy: cmd.CreatedBy,
			Approver:  approver,
			State:     reviewtaskstate.Open.String(),
		}
		_, err := atRepo.Create(ctx, tx, data)
		if err != nil {
			return fmt.Errorf("could not create approval task: %w", err)
		}
	}

	return nil
}

func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {

	if len(cmd.Reviewers) == 0 {
		return errors.New("no reviewers provided")
	}

	if len(cmd.Negotiators) == 0 {
		return errors.New("no negotiators provided")
	}

	if len(cmd.Approvers) == 0 {
		return errors.New("no approvers provided")
	}

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

	did, err := cmd.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	resp := db.Responsible{
		Creator:     did,
		Reviewers:   cmd.Reviewers,
		Approvers:   cmd.Approvers,
		Negotiators: cmd.Negotiators,
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          did,
		CreatedBy:       cmd.CreatedBy,
		State:           contractstate.Draft.String(),
		ContractData:    normalizedContractData,
		TemplateDID:     cmd.TemplateDID,
		TemplateVersion: contractTemplate.TemplateVersion,
		Responsible:     &resp,
	}
	createdAt, err := h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	err = createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd)
	if err != nil {
		return err
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
