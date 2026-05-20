package contracttemplate

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type GetAllMetadataByFilterQry struct {
	RetrievedBy    string
	DID            string
	DocumentNumber string
	Version        int
	State          *contracttemplatestate.ContractTemplateState
	TemplateType   *contracttemplatetype.ContractTemplateType
	Name           string
	Description    string
	TemplateData   string
}

type GetAllMetadataByFilterResult struct {
	DID                string
	DocumentNumber     *string
	Version            int
	State              contracttemplatestate.ContractTemplateState
	TemplateType       contracttemplatetype.ContractTemplateType
	Name               *string
	Description        *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ResponsiblePersons *db.ResponsiblePersons
	MetaData           datatype.JSON
}

type GetAllMetaDataByFilterHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
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

	var templateType string
	if query.TemplateType != nil {
		templateType = query.TemplateType.String()
	}

	searchValues := db.SearchValues{
		DID:            query.DID,
		DocumentNumber: query.DocumentNumber,
		Version:        query.Version,
		State:          state,
		TemplateType:   templateType,
		Name:           query.Name,
		Description:    query.Description,
		TemplateData:   query.TemplateData,
	}

	contractTemplates, err := h.CTRepo.ReadAllMetaDataByFilter(ctx, tx, searchValues)
	if err != nil {
		return nil, fmt.Errorf("could not read all contract templates: %w", err)
	}

	evt := templateevents.SearchEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	result := make([]GetAllMetadataByFilterResult, len(contractTemplates))
	for i, data := range contractTemplates {

		ctState, err := contracttemplatestate.NewContractTemplateState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		ctType, err := contracttemplatetype.NewContractTemplateType(data.TemplateType)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template type: %w", err)
		}

		result[i] = GetAllMetadataByFilterResult{
			DID:                data.DID,
			DocumentNumber:     data.DocumentNumber,
			Version:            data.Version,
			State:              ctState,
			TemplateType:       ctType,
			Name:               data.Name,
			Description:        data.Description,
			CreatedAt:          data.CreatedAt,
			UpdatedAt:          data.UpdatedAt,
			ResponsiblePersons: data.ResponsiblePersons,
		}
	}

	return result, nil
}
