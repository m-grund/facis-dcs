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
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetAllMetadataByFilterQry struct {
	RetrievedBy     string
	DID             string
	ContractVersion int
	State           *contractstate.ContractState
	Name            string
	Description     string
	ContractData    string
	Username        string
}

type GetAllMetadataByFilterResult struct {
	DID                string
	ContractVersion    int
	State              contractstate.ContractState
	Name               *string
	Description        *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MetaData           datatype.JSON
	StartDate          *time.Time
	ExpDate            *time.Time
	ExpPolicy          *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod    *int
	ResponsiblePersons *db.ResponsiblePersons
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
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

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

	evt := templateevents.SearchEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		Username:    query.Username,
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
			DID:                data.DID,
			ContractVersion:    data.ContractVersion,
			State:              contractState,
			Name:               data.Name,
			Description:        data.Description,
			CreatedAt:          data.CreatedAt,
			UpdatedAt:          data.UpdatedAt,
			StartDate:          data.StartDate,
			ExpDate:            data.ExpDate,
			ExpPolicy:          expPolicy,
			ExpNoticePeriod:    data.ExpNoticePeriod,
			ResponsiblePersons: data.ResponsiblePersons,
		}
	}

	return result, nil
}
