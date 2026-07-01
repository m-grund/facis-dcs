package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"

	"digital-contracting-service/internal/base"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type AcceptNegotiationCmd struct {
	ID         string             `json:"id"`
	DID        string             `json:"did"`
	AcceptedBy string             `json:"accepted_by"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
	CauserDID  string             `json:"causer_did"`
}

type NegotiationAcceptor struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument base.DIDDocument
}

func (h *NegotiationAcceptor) Handle(ctx context.Context, cmd AcceptNegotiationCmd) error {

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
		return fmt.Errorf("could not process core data: %w", err)
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

		err = remoteaction.AcceptNegotiation.Execute(ctx, h.DB, cmd.CauserDID, processData.Origin, processData.DID, cmd)
		if err != nil {
			return err
		}

		return nil
	}

	if processData.State != contractstate.Negotiation.String() || processData.State == contractstate.Terminated.String() {
		return errors.New("current contract state is invalid")
	}

	isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not validate negotiator: %w", err)
	}

	if !isValidNegotiator {
		return errors.New("invalid user")
	}

	err = h.NRepo.Accept(ctx, tx, cmd.ID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not accept negotiation: %w", err)
	}

	evt := contractevents.AcceptNegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		UserRoles:       cmd.UserRoles,
		AcceptedBy:      cmd.AcceptedBy,
		HolderDID:       cmd.HolderDID,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
