package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type ApproveCmd struct {
	DID           string
	UpdatedAt     time.Time
	ApprovedBy    string
	DecisionNotes []string
	HolderDID     string
	UserRoles     userrole.UserRoles
	DIDDocument   base.DIDDocument
}

type Approver struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Approver) Handle(ctx context.Context, cmd ApproveCmd) error {

	localPeer, err := cmd.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
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

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State != contractstate.Reviewed.String() || processData.State == contractstate.Terminated.String() {
		return errors.New("invalid contract state")
	}

	valid, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, localPeer)
	if err != nil {
		return err
	}

	if !valid {
		return errors.New("invalid user")
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, localPeer, approvaltaskstate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	existOpenTasks, err := h.ATRepo.AnyTasksInState(ctx, tx, processData.DID, approvaltaskstate.Open.String())
	if err != nil {
		return fmt.Errorf("could not check if review task exists: %w", err)
	}

	if !existOpenTasks {
		err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Approved.String())
		if err != nil {
			return fmt.Errorf("could not update current template state: %w", err)
		}
	}

	evt := contractevents.ApproveEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ApprovedBy:      cmd.ApprovedBy,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
