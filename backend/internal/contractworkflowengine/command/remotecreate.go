package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/contractworkflowengine/datatype/remote"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type RemoteCreateCmd struct {
	Contract             remote.ContractData
	ReviewTasks          []remote.ReviewTaskData
	ApprovalTasks        []remote.ApprovalTaskData
	NegotiationTasks     []remote.NegotiationTaskData
	Negotiations         []remote.NegotiationData
	NegotiationDecisions []remote.NegotiationDecisionData
}

type RemoteCreator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
	NRepo  db.NegotiationRepo
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

	reviewTasks := remote.ToReviewTaskData(cmd.ReviewTasks)
	for _, task := range reviewTasks {
		err := h.RTRepo.RemoteCreate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote review task: %w", err)
		}
	}

	approvalTasks := remote.ToApprovalTaskData(cmd.ApprovalTasks)
	for _, task := range approvalTasks {
		err := h.ATRepo.RemoteCreate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote approval task: %w", err)
		}
	}

	negotiationTasks := remote.ToNegotiationTaskData(cmd.NegotiationTasks)
	for _, task := range negotiationTasks {
		err := h.NTRepo.RemoteCreate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote negotiation task: %w", err)
		}
	}

	negotiations := remote.ToNegotiationData(cmd.Negotiations)
	for _, negotiation := range negotiations {
		err := h.NRepo.RemoteCreateNegotiation(ctx, tx, negotiation)
		if err != nil {
			return fmt.Errorf("could not create remote negotiation data: %w", err)
		}
	}

	negotiationDecisions := remote.ToNegotiationDecisionData(cmd.NegotiationDecisions)
	for _, decision := range negotiationDecisions {
		err := h.NRepo.RemoteCreateNegotiationDecision(ctx, tx, decision)
		if err != nil {
			return fmt.Errorf("could not create remote negotiation decision: %w", err)
		}
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
