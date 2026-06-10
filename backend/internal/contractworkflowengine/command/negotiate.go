package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type NegotiationCmd struct {
	DID           string
	NegotiatedBy  string
	ChangeRequest *datatype.JSON
	UpdatedAt     time.Time
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type Negotiator struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	NRepo  db.NegotiationRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *Negotiator) Handle(ctx context.Context, cmd NegotiationCmd) error {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

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
		return fmt.Errorf("could not process core data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State != contractstate.Negotiation.String() || processData.State == contractstate.Terminated.String() {
		return errors.New("current contract state is invalid")
	}

	isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.NegotiatedBy)
	if err != nil {
		return fmt.Errorf("could not validate negotiator: %w", err)
	}

	if !isValidNegotiator {
		return errors.New("invalid user")
	}

	negotiators, err := h.NTRepo.ReadNegotiatorsForDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read negotiators: %w", err)
	}

	data := db.NegotiationCreateData{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		CreatedBy:       cmd.NegotiatedBy,
	}
	_, err = h.NRepo.Create(ctx, tx, data, negotiators)
	if err != nil {
		return fmt.Errorf("could not create negotiation: %w", err)
	}

	err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not reopen negotiation: %w", err)
	}

	evt := contractevents.NegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		NegotiatedBy:    cmd.NegotiatedBy,
		Negotiators:     negotiators,
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
