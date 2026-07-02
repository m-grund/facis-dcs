package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"

	"github.com/jmoiron/sqlx"
)

type SubmitCmd struct {
	DID         string                 `json:"did"`
	UpdatedAt   time.Time              `json:"updated_at"`
	SubmittedBy string                 `json:"submitted_by"`
	ActionFlag  *actionflag.ActionFlag `json:"action_flag"`
	Comments    []string               `json:"comments"`
	HolderDID   string                 `json:"holder_did"`
	UserRoles   userrole.UserRoles     `json:"user_roles"`
	CauserDID   string                 `json:"causer_did"`
}

type Submitter struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument base.DIDDocument
}

func (h *Submitter) Handle(ctx context.Context, cmd SubmitCmd) error {

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

		err = remoteaction.Submit.Execute(ctx, h.DB, h.DIDDocument, processData.Origin, processData.DID, cmd)
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

	var nextState contractstate.ContractState
	if processData.State == contractstate.Draft.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractCreator) {
			return errors.New("invalid user permission")
		}

		nextState = contractstate.Negotiation

	} else if processData.State == contractstate.Rejected.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractCreator) {
			return errors.New("invalid user permission")
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

		if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.ContractReviewer) {
			return errors.New("invalid user permission")
		}

		isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
		if err != nil {
			return fmt.Errorf("could not validate negotiator: %w", err)
		}

		if !isValidNegotiator {
			return errors.New("this peer is not a valid negotiator")
		}

		hasOpenNegotiations, err := h.NRepo.HasOpenNegotiationDecisions(ctx, tx, cmd.DID, processData.ContractVersion, cmd.CauserDID)
		if err != nil {
			return fmt.Errorf("could not check open negotiations: %w", err)
		}

		if hasOpenNegotiations {
			return errors.New("not all negotiations are processed")
		}

		err = h.NTRepo.UpdateState(ctx, tx, processData.DID, cmd.CauserDID, negotiationtaskstate.Accepted.String())
		if err != nil {
			return fmt.Errorf("could not update negotiation task: %w", err)
		}

		existOpenTasks, err := h.NTRepo.AnyTasksInState(ctx, tx, processData.DID, negotiationtaskstate.Open.String())
		if err != nil {
			return fmt.Errorf("could not check if review task exists: %w", err)
		}

		if !existOpenTasks {

			hasNegotiations, err := h.NRepo.HasNegotiationForContractVersion(ctx, tx, cmd.DID, processData.ContractVersion)
			if err != nil {
				return fmt.Errorf("could not check if negotiation exists: %w", err)
			}

			if hasNegotiations {

				err = h.CRepo.CreateHistoryEntryForDID(ctx, tx, processData.DID)
				if err != nil {
					return fmt.Errorf("could not create history entry for did %s: %w", cmd.DID, err)
				}

				updatedData, err := negotiationmerging.MergeChangeRequests(ctx, tx, h.CRepo, h.NRepo, cmd.DID, processData.ContractVersion)
				if err != nil {
					return fmt.Errorf("could not merge change requests: %w", err)
				}

				updatedData.ContractVersion = processData.ContractVersion + 1
				err = h.CRepo.Update(ctx, tx, *updatedData)
				if err != nil {
					return fmt.Errorf("could not update contract version: %w", err)
				}

				evt := contractevents.IncreaseContractVersionEvent{
					DID:                cmd.DID,
					OldContractVersion: processData.ContractVersion,
					NewContractVersion: processData.ContractVersion + 1,
					SubmittedBy:        cmd.SubmittedBy,
					OccurredAt:         time.Now().UTC(),
					HolderDID:          cmd.HolderDID,
					UserRoles:          cmd.UserRoles,
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

		if !cmd.UserRoles.HasRoles(userrole.ContractReviewer) {
			return errors.New("invalid user permission")
		}

		isValid, err := h.RTRepo.IsValidReviewer(ctx, tx, processData.DID, cmd.CauserDID)
		if err != nil {
			return err
		}

		if !isValid {
			return errors.New("invalid user")
		}

		if cmd.ActionFlag != nil {
			switch *cmd.ActionFlag {
			case actionflag.Approval:
				err = h.RTRepo.UpdateState(ctx, tx, processData.DID, cmd.CauserDID, contractstate.Approved.String())
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
			case actionflag.Reject:
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

		if !cmd.UserRoles.HasRoles(userrole.ContractApprover) {
			return errors.New("invalid user permission")
		}

		isValid, err := h.ATRepo.IsValidApprover(ctx, tx, processData.DID, cmd.CauserDID)
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
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
