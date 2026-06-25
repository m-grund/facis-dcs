package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/datatype/remote"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type RemoteUpdateCmd struct {
	Contract             remote.ContractData
	ReviewTasks          []remote.ReviewTaskData
	ApprovalTasks        []remote.ApprovalTaskData
	NegotiationTasks     []remote.NegotiationTaskData
	Negotiations         []remote.NegotiationData
	NegotiationDecisions []remote.NegotiationDecisionData
}

type RemoteUpdater struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
	NRepo  db.NegotiationRepo
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
	if cmd.Contract.ExpPolicy != nil {
		policy := string(*cmd.Contract.ExpPolicy)
		expPolicy = &policy
	}
	newData := db.RemoteContractUpdateData{
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
		UpdatedAt:       cmd.Contract.UpdatedAt,
		CreatedAt:       cmd.Contract.CreatedAt,
		ExpPolicy:       expPolicy,
		ContractVersion: cmd.Contract.ContractVersion,
	}
	err = h.CRepo.RemoteUpdate(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update contract data: %w", err)
	}

	reviewTasks := remote.ToReviewTaskData(cmd.ReviewTasks)
	for _, task := range reviewTasks {
		err := h.RTRepo.RemoteUpdate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote review task: %w", err)
		}
	}

	approvalTasks := remote.ToApprovalTaskData(cmd.ApprovalTasks)
	for _, task := range approvalTasks {
		err := h.ATRepo.RemoteUpdate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote approval task: %w", err)
		}
	}

	negotiationTasks := remote.ToNegotiationTaskData(cmd.NegotiationTasks)
	for _, task := range negotiationTasks {
		err := h.NTRepo.RemoteUpdate(ctx, tx, task)
		if err != nil {
			return fmt.Errorf("could not create remote negotiation task: %w", err)
		}
	}
	/*
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
	*/
	evt := contractevents.RemoteUpdateEvent{
		DID:             cmd.Contract.DID,
		TemplateDID:     cmd.Contract.TemplateDID,
		CreatedBy:       cmd.Contract.CreatedBy,
		ContractData:    cmd.Contract.ContractData,
		OccurredAt:      time.Now().UTC(),
		Responsible:     cmd.Contract.Responsible,
		Name:            cmd.Contract.Name,
		Description:     cmd.Contract.Description,
		StartDate:       cmd.Contract.StartDate,
		ExpDate:         cmd.Contract.ExpDate,
		ExpPolicy:       cmd.Contract.ExpPolicy,
		Origin:          cmd.Contract.Origin,
		CreatedAt:       cmd.Contract.CreatedAt,
		UpdatedAt:       cmd.Contract.UpdatedAt,
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
