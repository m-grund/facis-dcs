package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	DID             string
	UpdatedAt       time.Time
	UpdatedBy       string
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Name            *string
	Description     *string
	ContractData    *datatype.JSON
	Username        string
	UserRoles       userrole.UserRoles
}

type Updater struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	oldData, err := h.CRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	// This avoids that updates on different DCS are possible
	if oldData.CreatedBy != cmd.UpdatedBy {
		return errors.New("invalid participant")
	}

	if oldData.State != contracttemplatestate.Draft.String() {
		return errors.New("invalid contract state")
	}

	if cmd.ExpDate != nil {
		tomorrow := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if cmd.ExpDate.Before(tomorrow) {
			return fmt.Errorf("expiration date must be at least one day in the future")
		}
	}

	if cmd.StartDate != nil {
		tomorrow := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if cmd.StartDate.Before(tomorrow) {
			return fmt.Errorf("start date must be at least one day in the future")
		}
	}

	if cmd.StartDate != nil && cmd.ExpDate != nil {
		if !cmd.ExpDate.After(*cmd.StartDate) {
			return fmt.Errorf("expiration date must be after start date")
		}
	}

	var oldExpPolicy *expirationpolicy.ExpirationPolicy
	if oldData.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*oldData.ExpPolicy)
		if err != nil {
			return fmt.Errorf("could not parse expiration policy: %w", err)
		}
		oldExpPolicy = &policy
	}

	var expPolicy *string
	if cmd.ExpPolicy != nil {
		s := cmd.ExpPolicy.String()
		expPolicy = &s
	}

	newData := db.ContractUpdateData{
		DID:             cmd.DID,
		Name:            cmd.Name,
		Description:     cmd.Description,
		StartDate:       cmd.StartDate,
		ExpDate:         cmd.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: cmd.ExpNoticePeriod,
		ContractData:    cmd.ContractData,
	}
	err = h.CRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update contract data: %w", err)
	}

	evt := contractevents.UpdateEvent{
		DID:                cmd.DID,
		OldName:            oldData.Name,
		NewName:            cmd.Name,
		OldDescription:     oldData.Description,
		NewDescription:     cmd.Description,
		OldContractData:    oldData.ContractData,
		NewContractData:    cmd.ContractData,
		OldStartDate:       oldData.StartDate,
		NewStartDate:       newData.StartDate,
		OldExpDate:         oldData.ExpDate,
		NewExpDate:         cmd.ExpDate,
		OldExpPolicy:       oldExpPolicy,
		NewExpPolicy:       cmd.ExpPolicy,
		OldExpNoticePeriod: oldData.ExpNoticePeriod,
		NewExpNoticePeriod: cmd.ExpNoticePeriod,
		UpdatedBy:          cmd.UpdatedBy,
		OccurredAt:         time.Now().UTC(),
		Username:           cmd.Username,
		UserRoles:          cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
