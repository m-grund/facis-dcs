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
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	events "digital-contracting-service/internal/contractworkflowengine/event"
)

type GetArchivedContractsResult struct {
	Contracts []MetadataItem
}

type GetArchivedContractsHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

type GetArchivedContractsQry struct {
	RetrievedBy string
}

func (h *GetArchivedContractsHandler) Handle(ctx context.Context, query GetArchivedContractsQry) (*GetArchivedContractsResult, error) {

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

	archivedContractsMetadata, err := h.CRepo.ReadArchivedContracts(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all contracts: %w", err)
	}

	evt := events.RetrieveArchivedEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
	}

	err = event.Create(ctx, tx, evt, componenttype.ContractStorageArchive)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	didToMetadata := make(map[string]MetadataItem)
	var contractItems []MetadataItem
	for _, data := range archivedContractsMetadata {

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
			DID:                data.DID,
			ContractVersion:    data.ContractVersion,
			State:              state,
			Name:               data.Name,
			Description:        data.Description,
			CreatedBy:          data.CreatedBy,
			CreatedAt:          data.CreatedAt,
			UpdatedAt:          data.UpdatedAt,
			StartDate:          data.StartDate,
			ExpDate:            data.ExpDate,
			ExpPolicy:          expPolicy,
			ExpNoticePeriod:    data.ExpNoticePeriod,
			ResponsiblePersons: data.ResponsiblePersons,
		}
		contractItems = append(contractItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	return &GetArchivedContractsResult{
		Contracts: contractItems,
	}, nil
}
