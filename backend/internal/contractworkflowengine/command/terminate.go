package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type TerminateCmd struct {
	DID          string
	TerminatedBy string
	Reason       string
	UpdatedAt    time.Time
	HolderDID    string
	UserRoles    userrole.UserRoles
}

type Terminator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NRepo  db.NegotiationRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *Terminator) Handle(ctx context.Context, cmd TerminateCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State == contractstate.Terminated.String() {
		return errors.New("contract is already terminated")
	}

	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Terminated.String())
	if err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	err = h.NTRepo.Delete(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete notification task: %w", err)
	}

	err = h.RTRepo.Delete(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete receive task: %w", err)
	}

	err = h.ATRepo.Delete(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete approval task: %w", err)
	}

	evt := contractevents.TerminateEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		TerminatedBy:    cmd.TerminatedBy,
		Reason:          cmd.Reason,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
