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

type SearchArchivedContractsQry struct {
	RetrievedBy     string
	DID             string
	ContractVersion int
	State           *contractstate.ContractState
	Name            string
	Description     string
	ContractData    string
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

	contractItems, err := toMetadataItems(archivedContractsMetadata)
	if err != nil {
		return nil, err
	}

	return &GetArchivedContractsResult{
		Contracts: contractItems,
	}, nil
}

func (h *GetArchivedContractsHandler) Search(ctx context.Context, query SearchArchivedContractsQry) (*GetArchivedContractsResult, error) {

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

	archivedContractsMetadata, err := h.CRepo.ReadArchivedContractsByFilter(ctx, tx, searchValues)
	if err != nil {
		return nil, fmt.Errorf("could not search archived contracts: %w", err)
	}

	evt := events.SearchEvent{
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

	contractItems, err := toMetadataItems(archivedContractsMetadata)
	if err != nil {
		return nil, err
	}

	return &GetArchivedContractsResult{
		Contracts: contractItems,
	}, nil
}

func toMetadataItems(contractsMetadata []db.ContractMetadata) ([]MetadataItem, error) {
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
	}

	return contractItems, nil
}
