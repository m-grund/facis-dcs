package remotesync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type PeerSyncCmd struct {
	Contract             ContractData
	ReviewTasks          []ReviewTaskData
	ApprovalTasks        []ApprovalTaskData
	NegotiationTasks     []NegotiationTaskData
	Negotiations         []NegotiationData
	NegotiationDecisions []NegotiationDecisionData
}

type PeerSynchronizer struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
	NRepo  db.NegotiationRepo
}

func (h *PeerSynchronizer) Handle(ctx context.Context, cmd PeerSyncCmd) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	exists, err := h.CRepo.ExistsByDID(ctx, tx, cmd.Contract.DID)
	if err != nil {
		return fmt.Errorf("could not check if contract exists: %w", err)
	}

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

	if exists {
		err := h.CRepo.RemoteUpdate(ctx, tx, data)
		if err != nil {
			return fmt.Errorf("could not update contract: %w", err)
		}

		reviewTasks := toReviewTaskData(cmd.ReviewTasks)
		for _, task := range reviewTasks {
			err := h.RTRepo.RemoteUpdate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not update review task: %w", err)
			}
		}

		approvalTasks := toApprovalTaskData(cmd.ApprovalTasks)
		for _, task := range approvalTasks {
			err := h.ATRepo.RemoteUpdate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not update approval task: %w", err)
			}
		}

		negotiationTasks := toNegotiationTaskData(cmd.NegotiationTasks)
		for _, task := range negotiationTasks {
			err := h.NTRepo.RemoteUpdate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not update negotiation task: %w", err)
			}
		}
		/*
			negotiations := toNegotiationData(cmd.Negotiations)
			for _, negotiation := range negotiations {
				err := h.NRepo.RemoteCreateNegotiation(ctx, tx, negotiation)
				if err != nil {
					return fmt.Errorf("could not create remote negotiation data: %w", err)
				}
			}

			negotiationDecisions := toNegotiationDecisionData(cmd.NegotiationDecisions)
			for _, decision := range negotiationDecisions {
				err := h.NRepo.RemoteCreateNegotiationDecision(ctx, tx, decision)
				if err != nil {
					return fmt.Errorf("could not create remote negotiation decision: %w", err)
				}
			}
		*/
	} else {
		err := h.CRepo.Create(ctx, tx, data)
		if err != nil {
			return fmt.Errorf("could not create contract: %w", err)
		}

		reviewTasks := toReviewTaskData(cmd.ReviewTasks)
		for _, task := range reviewTasks {
			err := h.RTRepo.RemoteCreate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not create review task: %w", err)
			}
		}

		approvalTasks := toApprovalTaskData(cmd.ApprovalTasks)
		for _, task := range approvalTasks {
			err := h.ATRepo.RemoteCreate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not create approval task: %w", err)
			}
		}

		negotiationTasks := toNegotiationTaskData(cmd.NegotiationTasks)
		for _, task := range negotiationTasks {
			err := h.NTRepo.RemoteCreate(ctx, tx, task)
			if err != nil {
				return fmt.Errorf("could not create negotiation task: %w", err)
			}
		}

		negotiations := toNegotiationData(cmd.Negotiations)
		for _, negotiation := range negotiations {
			err := h.NRepo.RemoteCreateNegotiation(ctx, tx, negotiation)
			if err != nil {
				return fmt.Errorf("could not create negotiation data: %w", err)
			}
		}

		negotiationDecisions := toNegotiationDecisionData(cmd.NegotiationDecisions)
		for _, decision := range negotiationDecisions {
			err := h.NRepo.RemoteCreateNegotiationDecision(ctx, tx, decision)
			if err != nil {
				return fmt.Errorf("could not create negotiation decision: %w", err)
			}
		}
	}

	evt := contractevents.RemoteSyncEvent{
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
