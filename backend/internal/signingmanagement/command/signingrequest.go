package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db"
	event2 "digital-contracting-service/internal/signingmanagement/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type SigningRequestCmd struct {
	DID         string
	RequestedBy string
	UpdatedAt   time.Time
}

type SigningRequester struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *SigningRequester) Handle(ctx context.Context, cmd SigningRequestCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err := h.CRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State == contractstate.Approved.String() {
		return errors.New("current contract state is invalid")
	}

	evt := event2.SigningRequestEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RequestedBy:     cmd.RequestedBy,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
