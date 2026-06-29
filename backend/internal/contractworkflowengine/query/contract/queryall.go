package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	events "digital-contracting-service/internal/contractworkflowengine/event"
)

type GetAllMetadataQry struct {
	RetrievedBy string
	HolderDID   string
	Pagination  datatype.Pagination
	UserRoles   userrole.UserRoles
}

type MetadataItem struct {
	DID                  string
	ContractVersion      int
	Name                 *string
	Description          *string
	State                contractstate.ContractState
	CreatedAt            time.Time
	UpdatedAt            time.Time
	MetaData             datatype.JSON
	CreatedBy            string
	StartDate            *time.Time
	ExpDate              *time.Time
	ExpPolicy            *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod      *int
	Responsible          *db.Responsible
	Outdated             *bool
	LatestTemplateDID    *string
	TemplateDID          string
	TemplateVersion      int
	TemplateIsDeprecated *bool
}

type ReviewTaskItem struct {
	DID             string
	ContractVersion int
	State           reviewtaskstate.ReviewTaskState
	Reviewer        string
	CreatedAt       time.Time
}

type ApprovalTaskItem struct {
	DID             string
	ContractVersion int
	State           approvaltaskstate.ApprovalTaskState
	Approver        string
	CreatedAt       time.Time
}

type NegotiatorTaskItem struct {
	DID             string
	ContractVersion int
	State           negotiationtaskstate.NegotiationTaskState
	Negotiator      string
	CreatedAt       time.Time
}

type GetAllMetadataResult struct {
	Contracts       []MetadataItem
	ReviewerTasks   []ReviewTaskItem
	ApprovalTasks   []ApprovalTaskItem
	NegotiatorTasks []NegotiatorTaskItem
}

type GetAllMetadataHandler struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *GetAllMetadataHandler) Handle(ctx context.Context, query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var contractsMetadata []db.ContractMetadata
	if query.Pagination.Limit >= 0 {
		contractsMetadata, err = h.CRepo.ReadAllMetaData(ctx, tx, query.Pagination)
		if err != nil {
			return nil, fmt.Errorf("could not read all contracts: %w", err)
		}
	}

	negotiationTasks, err := h.NTRepo.ReadAllByNegotiator(ctx, tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiation tasks: %w", err)
	}

	reviewerTasks, err := h.RTRepo.ReadAllByReviewer(ctx, tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	approvalTasks, err := h.ATRepo.ReadAllByApprover(ctx, tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	evt := events.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	didToMetadata := make(map[string]MetadataItem)
	var contractItems []MetadataItem
	for _, data := range contractsMetadata {

		state, err := contractstate.NewContractState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract state: %w", err)
		}

		var expPolicy *expirationpolicy.ExpirationPolicy
		if data.ExpPolicy != nil {
			policy, err := expirationpolicy.NewExpirationPolicy(*data.ExpPolicy)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			expPolicy = &policy
		}

		metadata := MetadataItem{
			DID:                  data.DID,
			ContractVersion:      data.ContractVersion,
			State:                state,
			Name:                 data.Name,
			Description:          data.Description,
			CreatedBy:            data.CreatedBy,
			CreatedAt:            data.CreatedAt,
			UpdatedAt:            data.UpdatedAt,
			TemplateDID:          data.TemplateDID,
			TemplateVersion:      data.TemplateVersion,
			StartDate:            data.StartDate,
			ExpDate:              data.ExpDate,
			ExpPolicy:            expPolicy,
			ExpNoticePeriod:      data.ExpNoticePeriod,
			Responsible:          data.Responsible,
			LatestTemplateDID:    data.LatestTemplateDID,
			TemplateIsDeprecated: data.TemplateIsDeprecated,
		}
		contractItems = append(contractItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	var reviewTaskItems []ReviewTaskItem
	for _, data := range reviewerTasks {

		state, err := reviewtaskstate.NewReviewTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create review task state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var contractVersion int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		reviewTaskItems = append(reviewTaskItems, ReviewTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: contractVersion,
			Reviewer:        data.Reviewer,
			CreatedAt:       data.CreatedAt,
		})
	}

	var negotiationTaskItems []NegotiatorTaskItem
	for _, data := range negotiationTasks {

		state, err := negotiationtaskstate.NewNegotiationTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create negotiation task state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var contractVersion int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		negotiationTaskItems = append(negotiationTaskItems, NegotiatorTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: contractVersion,
			Negotiator:      data.Negotiator,
			CreatedAt:       data.CreatedAt,
		})
	}

	var approvalTasksItems []ApprovalTaskItem
	for _, data := range approvalTasks {

		state, err := approvaltaskstate.NewApprovalTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create approval task state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var contractVersion int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		approvalTasksItems = append(approvalTasksItems, ApprovalTaskItem{
			DID:             data.DID,
			ContractVersion: contractVersion,
			State:           state,
			Approver:        data.Approver,
			CreatedAt:       data.CreatedAt,
		})
	}

	return &GetAllMetadataResult{
		Contracts:       contractItems,
		ReviewerTasks:   reviewTaskItems,
		ApprovalTasks:   approvalTasksItems,
		NegotiatorTasks: negotiationTaskItems,
	}, nil
}
