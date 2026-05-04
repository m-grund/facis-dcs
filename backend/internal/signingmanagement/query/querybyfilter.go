package query

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
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
	Filter          *string
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
}

type GetAllMetaDataByFilterHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetAllMetaDataByFilterHandler) Handle(ctx context.Context, query GetAllMetadataByFilterQry) ([]GetAllMetadataByFilterResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer tx.Rollback()

	searchValues := db.SearchValues{
		DID:             query.DID,
		ContractVersion: query.ContractVersion,
		Name:            query.Name,
		Description:     query.Description,
		Filter:          query.Filter,
	}

	contracts, err := h.CRepo.ReadAllMetaDataByFilter(tx, searchValues)
	if err != nil {
		return nil, fmt.Errorf("could not read all contract: %w", err)
	}

	evt := signingmanagementevents.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now(),
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
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

		result[i] = GetAllMetadataByFilterResult{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           contractState,
			Name:            data.Name,
			Description:     data.Description,
			CreatedAt:       data.CreatedAt,
			UpdatedAt:       data.UpdatedAt,
		}
	}

	return result, nil
}
