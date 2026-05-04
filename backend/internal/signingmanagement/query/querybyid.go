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

type GetByIDQry struct {
	DID         string
	RetrievedBy string
}

type GetByIDResult struct {
	DID             string
	ContractVersion *int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContractData    *datatype.JSON
}

type GetByIDHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	data, err := h.CRepo.ReadDataByID(tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract data: %w", err)
	}

	evt := signingmanagementevents.RetrieveByIDEvent{
		DID:         query.DID,
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

	state, err := contractstate.NewContractState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract state: %w", err)
	}

	return &GetByIDResult{
		DID:             query.DID,
		ContractVersion: data.ContractVersion,
		State:           state,
		Name:            data.Name,
		Description:     data.Description,
		CreatedBy:       data.CreatedBy,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
		ContractData:    data.ContractData,
	}, nil
}
