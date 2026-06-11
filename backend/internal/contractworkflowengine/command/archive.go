package command

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

const archiveSnapshotHashAlgorithm = "SHA-256"

// BuildArchiveEntry freezes the approved contract state for archive persistence.
func BuildArchiveEntry(contract *db.Contract, storedBy string) (db.ContractArchiveEntry, error) {
	if contract == nil {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract is required")
	}
	if contract.State != contractstate.Approved.String() {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract %s must be approved before archive storage", contract.DID)
	}

	snapshotJSON, err := buildContractSnapshot(contract)
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	contentHash := HashArchiveSnapshot(snapshotJSON)

	signatureMetadata, err := datatype.NewJSON(map[string]any{
		"status":   "NOT_PERFORMED",
		"reason":   "PDF/signature generation is not part of the archive foundation",
		"provider": "PENDING",
	})
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	credentialHashes, err := datatype.NewJSON(map[string]any{
		"status": "PENDING",
		"reason": "credential hash production is not part of the archive foundation",
	})
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}
	evidence, err := datatype.NewJSON(map[string]any{
		"source":                  "FINAL_CONTRACT_APPROVAL",
		"approved_by":             storedBy,
		"approved_state":          contractstate.Approved.String(),
		"snapshot_hash_algorithm": archiveSnapshotHashAlgorithm,
		"signed_pdf_out_of_scope": true,
		"signing_out_of_scope":    true,
	})
	if err != nil {
		return db.ContractArchiveEntry{}, err
	}

	return db.ContractArchiveEntry{
		DID:              contract.DID,
		ContractVersion:  contract.ContractVersion,
		StoredBy:         storedBy,
		ContractSnapshot: snapshotJSON,
		ContentHash:      contentHash,
		SignatureMeta:    &signatureMetadata,
		CredentialHashes: &credentialHashes,
		Evidence:         &evidence,
	}, nil
}

func buildContractSnapshot(contract *db.Contract) (datatype.JSON, error) {
	contractData := json.RawMessage(`{}`)
	if contract.ContractData != nil && contract.ContractData.IsNotNullValue() {
		contractData = json.RawMessage(*contract.ContractData)
	}

	snapshot := map[string]any{
		"did":                 contract.DID,
		"contract_version":    contract.ContractVersion,
		"state":               contract.State,
		"name":                stringPtrValue(contract.Name),
		"description":         stringPtrValue(contract.Description),
		"created_by":          contract.CreatedBy,
		"created_at":          formatArchiveTime(&contract.CreatedAt),
		"updated_at":          formatArchiveTime(&contract.UpdatedAt),
		"start_date":          formatArchiveTime(contract.StartDate),
		"exp_date":            formatArchiveTime(contract.ExpDate),
		"exp_policy":          stringPtrValue(contract.ExpPolicy),
		"exp_notice_period":   intPtrValue(contract.ExpNoticePeriod),
		"responsible_persons": contract.ResponsiblePersons,
		"contract_data":       contractData,
	}

	return datatype.NewJSON(snapshot)
}

func HashArchiveSnapshot(snapshot datatype.JSON) string {
	sum := sha256.Sum256(snapshot)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func formatArchiveTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func intPtrValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
