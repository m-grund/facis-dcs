package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	DID            string
	DocumentNumber *string
	Version        *int
	TemplateType   *contracttemplatetype.ContractTemplateType
	UpdatedAt      time.Time
	UpdatedBy      string
	Name           *string
	Description    *string
	TemplateData   *datatype.JSON
}

type Updater struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {
	if cmd.TemplateData != nil && cmd.TemplateData.IsNotNullValue() {
		normalizedTemplateData, err := validation.NormalizeTemplateData(cmd.TemplateData)
		if err != nil {
			return fmt.Errorf("template data validation failed: %w", err)
		}
		cmd.TemplateData = normalizedTemplateData
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	oldData, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if oldData.State != contracttemplatestate.Draft.String() &&
		oldData.State != contracttemplatestate.Rejected.String() &&
		oldData.State != contracttemplatestate.Submitted.String() {
		return errors.New("invalid contract template state")
	}

	isValidUser := false
	if (oldData.State == contracttemplatestate.Draft.String() || oldData.State == contracttemplatestate.Rejected.String()) &&
		oldData.CreatedBy == cmd.UpdatedBy {
		isValidUser = true
	} else if oldData.State == contracttemplatestate.Submitted.String() {
		valid, err := h.RTRepo.IsValidReviewer(ctx, tx, cmd.DID, cmd.UpdatedBy)
		if err != nil {
			return err
		}
		isValidUser = valid
	}

	if !isValidUser {
		return fmt.Errorf("invalid user")
	}

	err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}

	err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}

	var templateType string
	if cmd.TemplateType != nil {
		templateType = cmd.TemplateType.String()
	}

	newData := db.ContractTemplateUpdateData{
		DID:            cmd.DID,
		DocumentNumber: cmd.DocumentNumber,
		Version:        cmd.Version,
		TemplateType:   templateType,
		Name:           cmd.Name,
		Description:    cmd.Description,
		TemplateData:   cmd.TemplateData,
	}
	err = h.CTRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update template data: %w", err)
	}

	evt := templateevents.UpdateEvent{
		DID:               cmd.DID,
		OldDocumentNumber: oldData.DocumentNumber,
		NewDocumentNumber: cmd.DocumentNumber,
		OldVersion:        oldData.Version,
		NewVersion:        cmd.Version,
		OldName:           oldData.Name,
		NewName:           cmd.Name,
		OldDescription:    oldData.Description,
		NewDescription:    cmd.Description,
		OldTemplateData:   oldData.TemplateData,
		NewTemplateData:   cmd.TemplateData,
		UpdatedBy:         cmd.UpdatedBy,
		OccurredAt:        time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
