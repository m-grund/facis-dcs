package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	DID             string                             `json:"did"`
	UpdatedAt       time.Time                          `json:"updated_at"`
	UpdatedBy       string                             `json:"updated_by"`
	StartDate       *time.Time                         `json:"start_date"`
	ExpDate         *time.Time                         `json:"exp_date"`
	ExpPolicy       *expirationpolicy.ExpirationPolicy `json:"exp_policy"`
	ExpNoticePeriod *int                               `json:"exp_notice_period"`
	Name            *string                            `json:"name"`
	Description     *string                            `json:"description"`
	ContractData    *datatype.JSON                     `json:"contract_data"`
	HolderDID       string                             `json:"holder_did"`
	UserRoles       userrole.UserRoles                 `json:"user_roles"`
	CauserDID       string                             `json:"causer_did"`
}

type Updater struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {

	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		normalizedContractData, err := validation.NormalizeContractDataForPersistence(cmd.ContractData, cmd.DID, true)
		if err != nil {
			return fmt.Errorf("contract data validation failed: %w", err)
		}
		cmd.ContractData = normalizedContractData
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

	oldData, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	if oldData.Origin != localPeer && cmd.CauserDID != oldData.Origin {
		/*
			Unlike every other state-mutating handler in this package, Update does
			NOT forward to the Origin peer — it is simply rejected on non-Origin
			nodes. Updates must be performed directly on the contract's owner peer.
		*/

		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		return fmt.Errorf("updates are just allowed contract's owner peer")
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(oldData.State), contractstate.EventUpdate); err != nil {
		return err
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
		NewStartDate:       cmd.StartDate,
		OldExpDate:         oldData.ExpDate,
		NewExpDate:         cmd.ExpDate,
		OldExpPolicy:       oldExpPolicy,
		NewExpPolicy:       cmd.ExpPolicy,
		OldExpNoticePeriod: oldData.ExpNoticePeriod,
		NewExpNoticePeriod: cmd.ExpNoticePeriod,
		UpdatedBy:          cmd.UpdatedBy,
		OccurredAt:         time.Now().UTC(),
		HolderDID:          cmd.HolderDID,
		UserRoles:          cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
