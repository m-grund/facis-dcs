package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type SubmitCmd struct {
	DID         string
	UpdatedAt   time.Time
	SubmittedBy string
	Reviewers   []string
	Approver    *string
	Negotiators []string
	ActionFlag  *actionflag.ActionFlag
	Comments    []string
}

type Submitter struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NRepo  db.NegotiationRepo
	NTRepo db.NegotiationTaskRepo
}

func createTasks(ctx context.Context, tx *sqlx.Tx, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo, ntRepo db.NegotiationTaskRepo, cmd SubmitCmd) error {
	for _, reviewer := range cmd.Reviewers {
		reviewTask := db.ReviewTaskData{
			DID:       cmd.DID,
			Reviewer:  reviewer,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: cmd.SubmittedBy,
		}
		_, err := rtRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			return fmt.Errorf("could not create review task: %w", err)
		}
	}

	for _, negotiator := range cmd.Negotiators {
		negotiationTask := db.NegotiationTaskData{
			DID:        cmd.DID,
			Negotiator: negotiator,
			State:      reviewtaskstate.Open.String(),
			CreatedBy:  cmd.SubmittedBy,
		}
		_, err := ntRepo.Create(ctx, tx, negotiationTask)
		if err != nil {
			return fmt.Errorf("could not create negotiation task: %w", err)
		}
	}

	data := db.ApprovalTaskData{
		DID:       cmd.DID,
		CreatedBy: cmd.SubmittedBy,
		Approver:  *cmd.Approver,
		State:     reviewtaskstate.Open.String(),
	}
	_, err := atRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create approval task: %w", err)
	}

	return nil
}

func (h *Submitter) Handle(ctx context.Context, cmd SubmitCmd) error {

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

	var nextState contractstate.ContractState
	if processData.State == contractstate.Draft.String() {

		if cmd.SubmittedBy != processData.CreatedBy {
			return errors.New("invalid user")
		}

		if len(cmd.Reviewers) == 0 {
			return errors.New("no reviewer provided")
		}

		if len(cmd.Negotiators) == 0 {
			return errors.New("no negotiators provided")
		}

		if cmd.Approver == nil || len(*cmd.Approver) == 0 {
			return errors.New("no approver provided")
		}

		err := createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd)
		if err != nil {
			return err
		}

		nextState = contractstate.Negotiation

	} else if processData.State == contractstate.Rejected.String() {

		if processData.CreatedBy != cmd.SubmittedBy {
			return errors.New("invalid user")
		}

		err := h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen review tasks")
		}

		err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen negotiation tasks")
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen approval tasks")
		}

		nextState = contractstate.Negotiation

	} else if processData.State == contractstate.Negotiation.String() {

		isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.SubmittedBy)
		if err != nil {
			return fmt.Errorf("could not validate negotiator: %w", err)
		}

		if isValidNegotiator == false {
			return errors.New("invalid user")
		}

		hasOpenNegotiations, err := h.NRepo.HasOpenNegotiationDecisions(ctx, tx, cmd.DID, processData.ContractVersion)
		if err != nil {
			return fmt.Errorf("could not check open negotiations: %w", err)
		}

		if hasOpenNegotiations {
			return errors.New("not all negotiations are processed")
		}

		err = h.NTRepo.UpdateState(ctx, tx, processData.DID, cmd.SubmittedBy, negotiationtaskstate.Accepted.String())
		if err != nil {
			return fmt.Errorf("could not update negotiation task: %w", err)
		}

		existOpenTasks, err := h.NTRepo.AnyTasksInState(ctx, tx, processData.DID, negotiationtaskstate.Open.String())
		if err != nil {
			return fmt.Errorf("could not check if review task exists: %w", err)
		}

		if existOpenTasks == false {

			hasNegotiations, err := h.NRepo.HasNegotiationForContractVersion(ctx, tx, cmd.DID, processData.ContractVersion)
			if err != nil {
				return fmt.Errorf("could not check if negotiation exists: %w", err)
			}

			if hasNegotiations {
				err = contractworkflowengine.MergeChangeRequests(ctx, tx, h.CRepo, h.NRepo, cmd.DID, processData.ContractVersion)
				if err != nil {
					return fmt.Errorf("could not merge change requests: %w", err)
				}

				newVersion := 1
				if processData.ContractVersion != nil {
					newVersion = *processData.ContractVersion + 1
				}

				err = h.CRepo.Update(ctx, tx, db.ContractUpdateData{
					DID:             cmd.DID,
					ContractVersion: &newVersion,
				})
				if err != nil {
					return fmt.Errorf("could not update contract version: %w", err)
				}

				evt := contractevents.IncreaseContractVersionEvent{
					DID:                cmd.DID,
					OldContractVersion: processData.ContractVersion,
					NewContractVersion: &newVersion,
					SubmittedBy:        cmd.SubmittedBy,
					OccurredAt:         time.Now().UTC(),
				}
				err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
				if err != nil {
					return fmt.Errorf("could not create event: %w", err)
				}

			} else {
				nextState = contractstate.Submitted
			}
		}

	} else if processData.State == contractstate.Submitted.String() {

		isValid, err := h.RTRepo.IsValidReviewer(ctx, tx, processData.DID, cmd.SubmittedBy)
		if err != nil {
			return err
		}

		if !isValid {
			return errors.New("invalid user")
		}

		if cmd.ActionFlag != nil {
			if *cmd.ActionFlag == actionflag.Approval {

				err = h.RTRepo.UpdateState(ctx, tx, processData.DID, cmd.SubmittedBy, contractstate.Approved.String())
				if err != nil {
					return fmt.Errorf("could not update approval task: %w", err)
				}

				existOpenTasks, err := h.RTRepo.AnyTasksInState(ctx, tx, processData.DID, reviewtaskstate.Open.String())
				if err != nil {
					return fmt.Errorf("could not check if review task exists: %w", err)
				}

				if !existOpenTasks {
					nextState = contractstate.Reviewed
				}

			} else if *cmd.ActionFlag == actionflag.Reject {

				err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}

				err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}

				err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}

				nextState = contractstate.Negotiation
			}

		} else {
			return errors.New("action flags is missing")
		}

	} else if processData.State == contractstate.Reviewed.String() {

		isValid, err := h.ATRepo.IsValidApprover(ctx, tx, processData.DID, cmd.SubmittedBy)
		if err != nil {
			return err
		}

		if !isValid {
			return errors.New("invalid user")
		}

		err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		nextState = contractstate.Submitted

	} else {
		return errors.New("current contract state is invalid")
	}

	if len(nextState) > 0 && processData.State != nextState.String() {
		err = h.CRepo.UpdateState(ctx, tx, cmd.DID, nextState.String())
		if err != nil {
			return fmt.Errorf("could not update contract state: %w", err)
		}

		evt := contractevents.SubmitEvent{
			DID:             cmd.DID,
			ContractVersion: processData.ContractVersion,
			SubmittedBy:     cmd.SubmittedBy,
			PreviousState:   processData.State,
			NewState:        nextState.String(),
			ActionFlag:      cmd.ActionFlag,
			Comments:        cmd.Comments,
			OccurredAt:      time.Now().UTC(),
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
		if err != nil {
			return fmt.Errorf("could not create event: %w", err)
		}
	}

	return tx.Commit()
}
