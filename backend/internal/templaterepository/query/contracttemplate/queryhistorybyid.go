package contracttemplate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
)

type GetHistoryByIDQry struct {
	DID         string
	RetrievedBy string
	Username    string
}

type GetHistoryByIDResult struct {
	ID                 string
	DID                string
	DocumentNumber     *string
	Version            int
	State              contracttemplatestate.ContractTemplateState
	TemplateType       contracttemplatetype.ContractTemplateType
	Name               *string
	Description        *string
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ResponsiblePersons *db.ResponsiblePersons
	TemplateData       *datatype.JSON
}

type GetHistoryByIDHandler struct {
	Ctx    context.Context
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *GetHistoryByIDHandler) Handle(ctx context.Context, query GetHistoryByIDQry) ([]GetHistoryByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	entries, err := h.CTRepo.ReadHistoryByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract data: %w", err)
	}

	evt := contractevents.RetrieveByIDEvent{
		DID:         query.DID,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		Username:    query.Username,
	}
	err = event.Create(h.Ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	result := make([]GetHistoryByIDResult, len(entries))
	for idx, entry := range entries {

		state, err := contracttemplatestate.NewContractTemplateState(entry.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		ctType, err := contracttemplatetype.NewContractTemplateType(entry.TemplateType)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template type: %w", err)
		}

		result[idx] = GetHistoryByIDResult{
			ID:                 entry.ID,
			DID:                entry.DID,
			DocumentNumber:     entry.DocumentNumber,
			Version:            entry.Version,
			State:              state,
			Name:               entry.Name,
			Description:        entry.Description,
			CreatedBy:          entry.CreatedBy,
			CreatedAt:          entry.CreatedAt,
			UpdatedAt:          entry.UpdatedAt,
			TemplateData:       entry.TemplateData,
			TemplateType:       ctType,
			ResponsiblePersons: entry.ResponsiblePersons,
		}
	}

	return result, nil
}
