package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/dcstodcssynchronizer"

	db2 "digital-contracting-service/internal/dcstodcssynchronizer/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	DID             string
	UpdatedAt       time.Time
	UpdatedBy       string
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Name            *string
	Description     *string
	ContractData    *datatype.JSON
	HolderDID       string
	UserRoles       userrole.UserRoles
	DIDDocument     base.DIDDocument
}

type Updater struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
	NRepo  db.NegotiationRepo
	SRepo  db2.SyncRepository
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {

	origin, err := cmd.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		normalizedContractData, err := validation.NormalizeContractDataForPersistence(cmd.ContractData, cmd.DID, nil, true)
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

	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return errors.New("contract was updated elsewhere, please reload")
	}

	if oldData.State != contracttemplatestate.Draft.String() {
		return errors.New("invalid contract state")
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

	if oldData.Origin == origin {
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

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

	} else {

		contract, err := h.CRepo.ReadDataByDID(ctx, tx, oldData.DID)
		if err != nil {
			return fmt.Errorf("could not get contract data: %w", err)
		}

		trusted, err := h.SRepo.IsTrustedPeer(ctx, tx, oldData.Origin)
		if err != nil {
			return fmt.Errorf("could not check trusted peer: %w", err)
		}

		if !trusted {
			return fmt.Errorf("contract origin peer is not trusted peer list")
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		var startDate *string
		if cmd.StartDate != nil {
			s := contract.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if cmd.ExpDate != nil {
			s := contract.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if cmd.ExpPolicy != nil {
			s := contract.ExpPolicy
			expPolicy = s
		}

		contractItem := dcstodcs.DCSToDCSContractItem{
			Did:             contract.DID,
			ContractVersion: contract.ContractVersion,
			State:           contract.State,
			Name:            cmd.Name,
			Description:     cmd.Description,
			CreatedBy:       contract.CreatedBy,
			CreatedAt:       contract.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       cmd.UpdatedAt.Format(time.RFC3339),
			ContractData:    cmd.ContractData,
			TemplateDid:     contract.TemplateDID,
			TemplateVersion: contract.TemplateVersion,
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: cmd.ExpNoticePeriod,
			Responsible:     contract.Responsible,
			Origin:          contract.Origin,
		}

		result, err := dcstodcssynchronizer.ReadAllTasksData(ctx, h.DB, h.CRepo, h.RTRepo, h.ATRepo, h.NTRepo, h.NRepo, &contract.DID)
		if err != nil {
			return err
		}

		origin, err := cmd.DIDDocument.GetID()
		if err != nil {
			return err
		}

		hostname, err := base.DIDWebToHostname(oldData.Origin)
		if err != nil {
			return err
		}

		client := dcstodcssynchronizer.NewDCSToDCSHttpClient(hostname)
		_, remoteSyncErr := client.Sync(ctx, &dcstodcs.DCSToDCSContractSyncRequest{
			OriginDid:            origin,
			Contract:             &contractItem,
			ReviewTasks:          result.ReviewTasks,
			ApprovalTasks:        result.ApprovalTasks,
			NegotiationTasks:     result.NegotiationTasks,
			NegotiationItems:     result.Negotiations,
			NegotiationDecisions: result.NegotiationDecisions,
		})

		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		if remoteSyncErr != nil {

			err = h.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, oldData.Origin)
			if err != nil {
				return fmt.Errorf("could not create or update sync fail entry: %w", err)
			}

		} else {

			err = h.SRepo.DeleteSyncFailEntry(ctx, tx, oldData.Origin)
			if err != nil {
				return fmt.Errorf("could not create or update sync fail entry: %w", err)
			}
		}

		evt := contractevents.RemoteUpdateRequestEvent{
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

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		return remoteSyncErr
	}

	return nil
}
