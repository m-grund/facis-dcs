package remotesync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PeerSyncCmd struct {
	FromPeerDID          string
	LocalPeer            string
	Contract             ContractData
	ReviewTasks          []ReviewTaskData
	ApprovalTasks        []ApprovalTaskData
	NegotiationTasks     []NegotiationTaskData
	Negotiations         []NegotiationData
	NegotiationDecisions []NegotiationDecisionData
	DIDDocument          base.DIDDocument
	ContractOrigin       string
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

	oldData, err := h.CRepo.ReadProcessDataByDIDOrNil(ctx, tx, cmd.Contract.DID)
	if err != nil {
		return fmt.Errorf("could not check if contract exists: %w", err)
	}

	if oldData != nil {
		if cmd.Contract.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {

			evt := contractevents.OutdatedPeerEvent{
				DID:             cmd.Contract.DID,
				OutdatedPeerDID: cmd.FromPeerDID,
				OccurredAt:      time.Now().UTC(),
			}
			err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
			if err != nil {
				return fmt.Errorf("could not create event: %w", err)
			}

			err = tx.Commit()
			if err != nil {
				return fmt.Errorf("could not commit transaction: %w", err)
			}

			return fmt.Errorf("contract data is outdated. start synchronization. please reload")
		}
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

	if oldData != nil {
		err = h.CRepo.RemoteUpdate(ctx, tx, data)
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

	} else {
		err := h.CRepo.RemoteCreate(ctx, tx, data)
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
	}

	negotiations := toNegotiationData(cmd.Negotiations)
	for _, negotiation := range negotiations {
		err := h.NRepo.RemoteCreateOrUpdateNegotiation(ctx, tx, negotiation)
		if err != nil {
			return fmt.Errorf("could not create negotiation data: %w", err)
		}
	}

	negotiationDecisions := toNegotiationDecisionData(cmd.NegotiationDecisions)
	for _, decision := range negotiationDecisions {
		err := h.NRepo.RemoteCreateOrUpdateNegotiationDecision(ctx, tx, decision)
		if err != nil {
			return fmt.Errorf("could not create negotiation decision: %w", err)
		}
	}

	if cmd.FromPeerDID == cmd.LocalPeer || cmd.ContractOrigin == cmd.LocalPeer {
		evt := contractevents.RemoteSyncEvent{
			FromPeerDID:     cmd.FromPeerDID,
			LocalPeerDID:    cmd.LocalPeer,
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
	} else {
		evt := contractevents.RemoteSyncRequestEvent{
			FromPeerDID:     cmd.FromPeerDID,
			LocalPeerDID:    cmd.LocalPeer,
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
	}

	return tx.Commit()
}
