package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type TerminateCmd struct {
	DID          string             `json:"did"`
	TerminatedBy string             `json:"terminated_by"`
	Reason       string             `json:"reason"`
	UpdatedAt    time.Time          `json:"updated_at"`
	HolderDID    string             `json:"holder_did"`
	UserRoles    userrole.UserRoles `json:"user_roles"`
	CauserDID    string             `json:"causer_did"`
}

type Terminator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument base.DIDDocument
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

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	if processData.Origin != localPeer && cmd.CauserDID != processData.Origin {
		/*
			Forwards the action to contract owner peer
		*/

		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		err = remoteaction.Terminate.Execute(ctx, h.DB, h.DIDDocument, processData.Origin, processData.DID, cmd)
		if err != nil {
			return err
		}

		return nil
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State == contractstate.Terminated.String() {
		return errors.New("contract is already terminated")
	}

	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Terminated.String())
	if err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
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
