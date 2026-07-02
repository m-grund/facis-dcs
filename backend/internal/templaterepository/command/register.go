package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type RegisterCmd struct {
	DID          string
	Version      int
	RegisteredBy string
	HolderDID    string
	UserRoles    userrole.UserRoles
}

type Registrar struct {
	DB          *sqlx.DB
	CTRepo      db.ContractTemplateRepo
	FCClient    *fcclient.FederatedCatalogueClient
	DIDDocument identity.DIDDocument
}

func (h *Registrar) Handle(ctx context.Context, cmd RegisterCmd) (*string, error) {

	if cmd.DID == "" {
		return nil, errors.New("did is empty")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	existing, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not check if contract template already exists: %s", cmd.DID)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	if existing != nil {

		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Registered.String())
		if err != nil {
			return nil, fmt.Errorf("could not update registered state: %w", err)
		}

		newState := contracttemplatestate.Registered.String()
		evt := templateevents.RegisterEvent{
			DID:           cmd.DID,
			RegisteredBy:  cmd.RegisteredBy,
			UpdatedAt:     time.Now().UTC(),
			Name:          existing.Name,
			Description:   existing.Description,
			TemplateData:  existing.TemplateData,
			SourceDID:     existing.DID,
			SourceVersion: existing.Version,
			OccurredAt:    time.Now().UTC(),
			HolderDID:     cmd.HolderDID,
			UserRoles:     cmd.UserRoles,
			PreviousState: &existing.State,
			NewState:      &newState,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return &cmd.DID, nil

	} else {

		if cmd.Version < 1 {
			return nil, errors.New("version must be greater than 0")
		}

		if h.FCClient == nil {
			return nil, fcclient.ErrFederatedCatalogueNotConfigured
		}

		newDID, err := base.GenerateID()
		if err != nil {
			return nil, fmt.Errorf("could not get new DID for contract template: %w", err)
		}

		queryHandler := templatequery.GetByIDHandler{
			Ctx:      ctx,
			FCClient: h.FCClient,
		}
		fcTemplate, err := queryHandler.Handle(templatequery.GetByIDQry{
			DID:     cmd.DID,
			Version: cmd.Version,
		})
		if err != nil {
			return nil, fmt.Errorf("could not retrieve template from Federated Catalogue: %w", err)
		}
		if fcTemplate == nil {
			return nil, fcclient.ErrTemplateNotFoundInFederatedCatalogue
		}

		templateData, err := templateDataFromAny(fcTemplate.TemplateData)
		if err != nil {
			return nil, err
		}

		templateTypeValue := ""
		if fcTemplate.TemplateType != nil {
			templateTypeValue = *fcTemplate.TemplateType
		}
		templateType, err := contracttemplatetype.NewContractTemplateType(templateTypeValue)
		if err != nil {
			return nil, fmt.Errorf("invalid template type from Federated Catalogue: %w", err)
		}

		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		_, err = h.CTRepo.Create(ctx, tx, db.ContractTemplate{
			DID:            *newDID,
			DocumentNumber: fcTemplate.DocumentNumber,
			State:          contracttemplatestate.Draft.String(),
			TemplateType:   templateType.String(),
			Name:           fcTemplate.Name,
			Description:    fcTemplate.Description,
			CreatedBy:      cmd.RegisteredBy,
			TemplateData:   templateData,
		})
		if err != nil {
			return nil, fmt.Errorf("could not create registered contract template: %w", err)
		}

		evt := templateevents.RegisterEvent{
			DID:           *newDID,
			RegisteredBy:  cmd.RegisteredBy,
			UpdatedAt:     time.Now().UTC(),
			Name:          fcTemplate.Name,
			Description:   fcTemplate.Description,
			TemplateData:  templateData,
			SourceDID:     cmd.DID,
			SourceVersion: cmd.Version,
			OccurredAt:    time.Now().UTC(),
			HolderDID:     cmd.HolderDID,
			UserRoles:     cmd.UserRoles,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return newDID, nil
	}
}

func templateDataFromAny(raw any) (*datatype.JSON, error) {
	if raw == nil {
		return nil, errors.New("template data is missing from Federated Catalogue")
	}

	templateDataMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid template data format from Federated Catalogue")
	}

	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal template data failed: %w", err)
	}

	return &templateData, nil
}
