package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type ApproveCmd struct {
	DID           string
	UpdatedAt     time.Time
	ApprovedBy    string
	DecisionNotes []string
}

type Approver struct {
	DB            *sqlx.DB
	CRepo         db.ContractRepo
	ATRepo        db.ApprovalTaskRepo
	IPFSStorer    ArchiveSnapshotStorer
	ArchiveNotary ArchiveNotary
}

type ArchiveSnapshotStorer interface {
	CreateFile(ctx context.Context, data any) (*ipfs.IPFSResult, error)
}

type ArchiveNotary interface {
	NotarizeArchiveEntry(ctx context.Context, payload ArchiveNotaryPayload) (*ArchiveNotaryReceipt, error)
}

func (h *Approver) Handle(ctx context.Context, cmd ApproveCmd) error {

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

	if processData.State != contractstate.Reviewed.String() || processData.State == contractstate.Terminated.String() {
		return errors.New("invalid contract state")
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

	existOpenTasks, err := h.ATRepo.AnyTasksInState(ctx, tx, processData.DID, approvaltaskstate.Open.String())
	if err != nil {
		return fmt.Errorf("could not check if review task exists: %w", err)
	}

	if !existOpenTasks {
		err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Approved.String())
		if err != nil {
			return fmt.Errorf("could not update current template state: %w", err)
		}

		approvedContract, err := h.CRepo.ReadDataByID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read approved contract for archive storage: %w", err)
		}
		archiveEntry, err := BuildArchiveEntry(approvedContract, cmd.ApprovedBy)
		if err != nil {
			return fmt.Errorf("could not build archive entry: %w", err)
		}
		if h.IPFSStorer == nil {
			return errors.New("archive snapshot IPFS storer is required")
		}
		snapshotResult, err := h.IPFSStorer.CreateFile(ctx, archiveEntry.ContractSnapshot)
		if err != nil {
			return fmt.Errorf("could not store archive snapshot in IPFS: %w", err)
		}
		if snapshotResult == nil || snapshotResult.Identifier.Value == "" {
			return errors.New("archive snapshot IPFS storer returned empty CID")
		}
		archiveEntry.SnapshotCID = snapshotResult.Identifier.Value

		err = h.CRepo.StoreArchiveEntry(ctx, tx, archiveEntry)
		if err != nil {
			return fmt.Errorf("could not store contract in archive: %w", err)
		}

		var notaryReceipt *ArchiveNotaryReceipt
		if h.ArchiveNotary != nil {
			notaryReceipt, err = h.ArchiveNotary.NotarizeArchiveEntry(ctx, ArchiveNotaryPayload{
				EventType:       "ARCHIVE_STORED",
				ArchiveEntryID:  archiveNotaryEntryID(cmd.DID, processData.ContractVersion),
				DID:             cmd.DID,
				ContractVersion: processData.ContractVersion,
				ContentHash:     archiveEntry.ContentHash,
				SnapshotCID:     archiveEntry.SnapshotCID,
				StoredBy:        cmd.ApprovedBy,
				StoredAt:        archiveEntry.StoredAt,
			})
			if err != nil {
				return fmt.Errorf("could not notarize archive entry: %w", err)
			}
		}

		archiveEvt := contractevents.StoreArchivedEvent{
			DID:             cmd.DID,
			ContractVersion: processData.ContractVersion,
			StoredBy:        cmd.ApprovedBy,
			ContentHash:     archiveEntry.ContentHash,
			SnapshotCID:     archiveEntry.SnapshotCID,
			ArchiveStatus:   "STORED",
			NotaryReceipt:   archiveNotaryEventReceipt(notaryReceipt),
			EvidenceSummary: contractevents.ArchiveEvidenceSummary{
				SnapshotHashAlgorithm: archiveSnapshotHashAlgorithm,
				SignatureStatus:       "NOT_PERFORMED",
				CredentialHashStatus:  "PENDING",
			},
			OccurredAt: time.Now().UTC(),
		}
		err = event.Create(ctx, tx, archiveEvt, componenttype.ContractStorageArchive)
		if err != nil {
			return fmt.Errorf("could not create archive store event: %w", err)
		}
	}

	evt := contractevents.ApproveEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ApprovedBy:      cmd.ApprovedBy,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
