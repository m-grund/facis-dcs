package qry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	processauditandcompliance2 "digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"

	"digital-contracting-service/internal/base/validation"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
)

type GetContractContentTrailQry struct {
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type ContractContentTrailAuditor struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *ContractContentTrailAuditor) Handle(ctx context.Context, query GetContractContentTrailQry) (map[string][]datatype.AuditLogEntry, error) {

	result := map[string][]datatype.AuditLogEntry{}
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	pagination := datatype.Pagination{}
	contracts, err := h.CRepo.ReadAllMetaData(ctx, tx, pagination)
	if err != nil {
		return nil, fmt.Errorf("could not read all metadata: %w", err)
	}

	for contractIndex, metadata := range contracts {
		contract, err := h.CRepo.ReadDataByID(ctx, tx, metadata.DID)
		if err != nil {
			return nil, fmt.Errorf("could not read data: %w", err)
		}

		if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
			continue
		}
		auditMetadata := validation.ContractContentAuditMetadata{
			ContractDID:     contract.DID,
			ContractVersion: fmt.Sprint(contract.ContractVersion),
			AuditedBy:       middleware.GetParticipantID(ctx),
			HolderDID:       middleware.GetHolderDID(ctx),
		}
		findings, err := validation.AuditContractContent(contract.ContractData, nil, auditMetadata)
		if err != nil {
			return nil, fmt.Errorf("could not audit contract: %w", err)
		}

		entries := make([]datatype.AuditLogEntry, 0, len(findings))
		for findingIndex, finding := range findings {

			data, err := json.Marshal(processauditandcompliance2.ContractContentPolicyFindingEventData(finding, auditMetadata))
			if err != nil {
				return nil, err
			}

			entries = append(entries, datatype.AuditLogEntry{
				ID:        int64(-3000000 - (contractIndex * 10000) - findingIndex),
				Component: componenttype.ContractWorkflowEngine.String(),
				EventType: eventtype.ContractContentPolicyAuditFinding.String(),
				EventData: data,
				DID:       &contract.DID,
				CreatedAt: time.Now().UTC(),
			})
		}
		result[contract.DID] = entries
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
