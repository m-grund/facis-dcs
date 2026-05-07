package contract

import (
	"context"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type GetAllMetadataByFilterQry struct {
	RetrievedBy     string
	DID             *string
	ContractVersion *int
	State           *contractstate.ContractState
	Name            *string
	Description     *string
	ContractData    *string
}

type GetAllMetadataByFilterResult struct {
	DID             string
	ContractVersion *int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	MetaData        datatype.JSON
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
}

type GetAllMetaDataByFilterHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetAllMetaDataByFilterHandler) Handle(ctx context.Context, query GetAllMetadataByFilterQry) ([]GetAllMetadataByFilterResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer tx.Rollback()

	var state string
	if query.State != nil {
		state = query.State.String()
	}

	searchValues := db.SearchValues{
		DID:             query.DID,
		ContractVersion: query.ContractVersion,
		State:           state,
		Name:            query.Name,
		Description:     query.Description,
		ContractData:    query.ContractData,
	}

	contracts, err := h.CRepo.ReadAllMetaDataByFilter(ctx, tx, searchValues)
	if err != nil {
		return nil, fmt.Errorf("could not read all contract: %w", err)
	}

	evt := templateevents.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	result := make([]GetAllMetadataByFilterResult, len(contracts))
	for i, data := range contracts {

		contractState, err := contractstate.NewContractState(data.State)
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

		result[i] = GetAllMetadataByFilterResult{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           contractState,
			Name:            data.Name,
			Description:     data.Description,
			CreatedAt:       data.CreatedAt,
			UpdatedAt:       data.UpdatedAt,
			ExpDate:         data.ExpDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: data.ExpNoticePeriod,
		}
	}

	return result, nil
}
