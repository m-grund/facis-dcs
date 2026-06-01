package query

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type GetAllMetadataQry struct {
	RetrievedBy string
	Username    string
}

type MetadataItem struct {
	DID             string
	ContractVersion *int
	Name            *string
	Description     *string
	State           contractstate.ContractState
	CreatedAt       time.Time
	UpdatedAt       time.Time
	MetaData        datatype.JSON
}

type GetAllMetadataResult struct {
	Contracts []MetadataItem
}

type GetAllMetadataHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetAllMetadataHandler) Handle(ctx context.Context, query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Println("could not rollback transaction")
		}
	}(tx)

	contractsMetadata, err := h.CRepo.ReadAllMetaData(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all contracts: %w", err)
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

	didToMetadata := make(map[string]MetadataItem)
	var contractItems []MetadataItem
	for _, data := range contractsMetadata {

		state, err := contractstate.NewContractState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract state: %w", err)
		}

		metadata := MetadataItem{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           state,
			Name:            data.Name,
			Description:     data.Description,
			CreatedAt:       data.CreatedAt,
			UpdatedAt:       data.UpdatedAt,
		}
		contractItems = append(contractItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	return &GetAllMetadataResult{
		Contracts: contractItems,
	}, nil
}
