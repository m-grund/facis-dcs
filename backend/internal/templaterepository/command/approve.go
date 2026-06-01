package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type ApproveCmd struct {
	DID           string
	UpdatedAt     time.Time
	ApprovedBy    string
	DecisionNotes []string
	Username      string
}

type Approver struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Approver) Handle(ctx context.Context, cmd ApproveCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CTRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Reviewed.String() {
		return errors.New("invalid contract template state")
	}

	valid, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.ApprovedBy)
	if err != nil {
		return err
	}

	if !valid {
		return errors.New("invalid user")
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.ApprovedBy, approvaltaskstate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update current template state: %w", err)
	}

	evt := templateevents.ApproveEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		ApprovedBy:     cmd.ApprovedBy,
		DecisionNotes:  cmd.DecisionNotes,
		OccurredAt:     time.Now().UTC(),
		Username:       cmd.Username,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
