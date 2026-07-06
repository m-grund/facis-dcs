package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/validation"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"
)

type ApproveCmd struct {
	DID           string             `json:"did"`
	UpdatedAt     time.Time          `json:"updated_at"`
	ApprovedBy    string             `json:"approved_by"`
	DecisionNotes []string           `json:"decision_notes"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
	CauserDID     string             `json:"causer_did"`
}

type Approver struct {
	DB            *sqlx.DB
	CRepo         db.ContractRepo
	ATRepo        db.ApprovalTaskRepo
	SRepo         db2.SyncRepository
	DIDDocument   identity.DIDDocument
	IPFSStorer    ArchiveSnapshotStorer
	ArchiveNotary ArchiveNotary
	ArchiveTSA    ArchiveTimestampIssuer
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
			Not the Origin peer for this contract: forward unchanged instead of
			mutating locally (single-writer-per-aggregate, see package doc).
		*/

		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		err = remoteaction.Approve.Execute(ctx, h.DB, h.DIDDocument, processData.Origin, processData.DID, cmd)
		if err != nil {
			return err
		}

		return nil
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007). The distinct
	// messages tell a local caller to simply reload vs. a forwarded/remote
	// caller to force a full peer re-sync first.
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if processData.State != contractstate.Reviewed.String() || processData.State == contractstate.Terminated.String() {
		return errors.New("invalid contract state")
	}

	// IsValidApprover checks CauserDID (a peer DID) against the task's assigned
	// approver — task ownership is peer-scoped, not individual-user-scoped;
	// per-user permission was already checked locally via UserRoles upstream.
	valid, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return err
	}

	if !valid {
		return errors.New("invalid user")
	}

	contractForPolicyValidation, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract for policy validation: %w", err)
	}
	if contractForPolicyValidation.ContractData == nil {
		return fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}
	if err := validation.ValidateContractPolicySatisfaction(
		*contractForPolicyValidation.ContractData,
		validation.ContractContentAuditMetadata{
			ContractDID:     cmd.DID,
			ContractVersion: fmt.Sprint(processData.ContractVersion),
			AuditedBy:       cmd.ApprovedBy,
			HolderDID:       cmd.HolderDID,
		},
	); err != nil {
		return err
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.CauserDID, approvaltaskstate.Approved.String())
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

		approvedContract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read approved contract for archive storage: %w", err)
		}
		finalContractData, err := semanticmapper.MaterializeStoredContractJSONLD(
			*approvedContract,
			semanticmapper.DefaultProfile(),
		)
		if err != nil {
			return fmt.Errorf("could not materialize approved contract JSON-LD: %w", err)
		}
		finalContractJSON, err := datatype.NewJSON(finalContractData)
		if err != nil {
			return fmt.Errorf("could not encode approved contract JSON-LD: %w", err)
		}
		approvedContract.ContractData = &finalContractJSON
		if err := h.CRepo.Update(ctx, tx, db.ContractUpdateData{
			DID:          cmd.DID,
			ContractData: approvedContract.ContractData,
		}); err != nil {
			return fmt.Errorf("could not persist approved contract JSON-LD: %w", err)
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

		var notaryReceipt *ArchiveNotaryReceipt
		notaryPayload := ArchiveNotaryPayload{
			EventType:       "ARCHIVE_STORED",
			ArchiveEntryID:  archiveNotaryEntryID(cmd.DID, processData.ContractVersion),
			DID:             cmd.DID,
			ContractVersion: processData.ContractVersion,
			ContentHash:     archiveEntry.ContentHash,
			SnapshotCID:     archiveEntry.SnapshotCID,
			StoredBy:        cmd.ApprovedBy,
			StoredAt:        archiveEntry.StoredAt,
		}
		if h.ArchiveNotary != nil {
			notaryReceipt, err = h.ArchiveNotary.NotarizeArchiveEntry(ctx, notaryPayload)
			if err != nil {
				return fmt.Errorf("could not notarize archive entry: %w", err)
			}
		}

		var tsaReceipt *contractevents.ArchiveTSAReceipt
		if h.ArchiveTSA != nil && h.ArchiveTSA.Enabled() && notaryReceipt != nil {
			evidence, err := BuildArchiveTimestampEvidence(notaryPayload, notaryReceipt)
			if err != nil {
				return fmt.Errorf("could not build archive TSA evidence: %w", err)
			}
			evidenceBytes, err := CanonicalArchiveTimestampEvidence(evidence)
			if err != nil {
				return err
			}
			rawReceipt, err := h.ArchiveTSA.TimestampBytes(ctx, evidenceBytes)
			if err != nil {
				return fmt.Errorf("could not timestamp archive entry: %w", err)
			}
			tsaReceipt = archiveTSAEventReceipt(rawReceipt)
			tsaReceiptJSON, err := datatype.NewJSON(tsaReceipt)
			if err != nil {
				return fmt.Errorf("could not encode archive TSA receipt: %w", err)
			}
			archiveEntry.TSAReceipt = &tsaReceiptJSON
		}

		err = h.CRepo.StoreArchiveEntry(ctx, tx, archiveEntry)
		if err != nil {
			return fmt.Errorf("could not store contract in archive: %w", err)
		}

		archiveEvt := contractevents.StoreArchivedEvent{
			DID:             cmd.DID,
			ContractVersion: processData.ContractVersion,
			StoredBy:        cmd.ApprovedBy,
			ContentHash:     archiveEntry.ContentHash,
			SnapshotCID:     archiveEntry.SnapshotCID,
			ArchiveStatus:   "STORED",
			NotaryReceipt:   archiveNotaryEventReceipt(notaryReceipt),
			TSAReceipt:      tsaReceipt,
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
