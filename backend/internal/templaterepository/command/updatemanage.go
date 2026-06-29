package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type UpdateManageCmd struct {
	DID            string
	DocumentNumber *string
	State          *contracttemplatestate.ContractTemplateState
	TemplateType   *contracttemplatetype.ContractTemplateType
	UpdatedAt      time.Time
	UpdatedBy      string
	Name           *string
	Description    *string
	TemplateData   *datatype.JSON
	IsManager      bool
	HolderDID      string
	UserRoles      userrole.UserRoles
}

type UpdateManager struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *UpdateManager) Handle(ctx context.Context, cmd UpdateManageCmd) error {
	if cmd.TemplateData != nil && cmd.TemplateData.IsNotNullValue() {
		normalizedTemplateData, err := validation.NormalizeTemplateDataForPersistence(cmd.TemplateData, cmd.DID, cmd.Name)
		if err != nil {
			return fmt.Errorf("template data validation failed: %w", err)
		}
		cmd.TemplateData = normalizedTemplateData
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	oldData, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if oldData.State == contracttemplatestate.Published.String() ||
		oldData.State == contracttemplatestate.Deprecated.String() ||
		oldData.State == contracttemplatestate.Registered.String() {
		return errors.New("invalid contract template state")
	}

	if cmd.State != nil {
		reviewTasksExist, err := h.RTRepo.TaskExist(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not check existing review tasks: %w", err)
		}

		approvalTaskExists, err := h.ATRepo.TaskExists(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not check existing approval tasks: %w", err)
		}

		if !reviewTasksExist {
			reviewTask := db.ReviewTaskData{
				DID:       cmd.DID,
				Reviewer:  cmd.UpdatedBy,
				State:     reviewtaskstate.Open.String(),
				CreatedBy: cmd.UpdatedBy,
			}
			_, err := h.RTRepo.Create(ctx, tx, reviewTask)
			if err != nil {
				return fmt.Errorf("could not create review tasks: %w", err)
			}
		}

		if !approvalTaskExists {
			data := db.ApprovalTaskData{
				DID:       cmd.DID,
				CreatedBy: cmd.UpdatedBy,
				Approver:  cmd.UpdatedBy,
				State:     reviewtaskstate.Open.String(),
			}

			_, err = h.ATRepo.Create(ctx, tx, data)
			if err != nil {
				return fmt.Errorf("could not create approval task: %w", err)
			}
		}
	}

	newState := oldData.State
	if cmd.State != nil {
		switch *cmd.State {
		case contracttemplatestate.Draft, contracttemplatestate.Deleted:
			err = h.RTRepo.Delete(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not delete review tasks: %w", err)
			}
			err = h.ATRepo.Delete(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not delete approval tasks: %w", err)
			}
		case contracttemplatestate.Rejected, contracttemplatestate.Submitted:
			err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}
			err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}
		case contracttemplatestate.Reviewed:
			err = h.RTRepo.UpdateStateForAllTasks(ctx, tx, cmd.DID, reviewtaskstate.Approved.String())
			if err != nil {
				return err
			}
			err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}
		case contracttemplatestate.Approved:
			err = h.RTRepo.UpdateStateForAllTasks(ctx, tx, cmd.DID, reviewtaskstate.Approved.String())
			if err != nil {
				return err
			}
			err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.UpdatedBy, approvaltaskstate.Approved.String())
			if err != nil {
				return err
			}
		default:
			return errors.New("contract invalid state")
		}
		newState = cmd.State.String()
	}

	var state string
	if cmd.State != nil {
		state = cmd.State.String()
	}

	var templateType string
	if cmd.TemplateType != nil {
		templateType = cmd.TemplateType.String()
	}

	newData := db.ContractTemplateUpdateData{
		DID:            cmd.DID,
		DocumentNumber: cmd.DocumentNumber,
		State:          state,
		TemplateType:   templateType,
		Name:           cmd.Name,
		Description:    cmd.Description,
		TemplateData:   cmd.TemplateData,
	}
	err = h.CTRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update template data: %w", err)
	}

	evt := templateevents.UpdateManageEvent{
		DID:               cmd.DID,
		OldDocumentNumber: oldData.DocumentNumber,
		NewDocumentNumber: cmd.DocumentNumber,
		OldState:          &oldData.State,
		NewState:          &newState,
		OldName:           oldData.Name,
		NewName:           cmd.Name,
		OldDescription:    oldData.Description,
		NewDescription:    cmd.Description,
		OldTemplateData:   oldData.TemplateData,
		NewTemplateData:   cmd.TemplateData,
		UpdatedBy:         cmd.UpdatedBy,
		OccurredAt:        time.Now().UTC(),
		HolderDID:         cmd.HolderDID,
		UserRoles:         cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
