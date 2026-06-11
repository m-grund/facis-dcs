package qry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/base/datatype"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/middleware"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/validation"
)

type GetStaticContentAuditQry struct {
	DID              *string
	ContractDocument *string
	ContractVersion  *string
	Policy           any
	PolicyVersion    *string
	RetrievedBy      string
	HolderDID        string
	UserRoles        userrole.UserRoles
}

type GetStaticContentAuditResult struct {
	ID               int64
	Component        componenttype.ComponentType
	EventType        eventtype.EventType
	EventData        any
	DID              *string
	CreatedAt        time.Time
	ResLogPredCID    *string
	GlobalLogPredCID *string
}

type StaticContentAuditor struct {
}

func (h *StaticContentAuditor) Handle(ctx context.Context, query GetStaticContentAuditQry) ([]datatype.AuditLogEntry, error) {

	if query.ContractDocument == nil {
		return nil, fmt.Errorf("contract_document is required for static contract audits")
	}

	contractDID := base.DerefString(query.DID)
	if contractDID == "" {
		contractDID = "inline-contract"
	}

	metadata := validation.ContractContentAuditMetadata{
		ContractDID:     contractDID,
		ContractVersion: base.DerefString(query.ContractVersion),
		PolicyVersion:   base.DerefString(query.PolicyVersion),
		AuditedBy:       middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
	}

	findings, err := validation.AuditContractContent(query.ContractDocument, query.Policy, metadata)
	if err != nil {
		return nil, err
	}

	entries := make([]datatype.AuditLogEntry, 0, len(findings))
	for i, finding := range findings {

		data, err := json.Marshal(processauditandcompliance.ContractContentPolicyFindingEventData(finding, metadata))
		if err != nil {
			return nil, err
		}

		entries = append(entries, datatype.AuditLogEntry{
			ID:        int64(-1000 - i),
			Component: componenttype.ContractWorkflowEngine.String(),
			EventType: eventtype.ContractContentPolicyAuditFinding.String(),
			EventData: data,
			DID:       query.DID,
			CreatedAt: time.Now().UTC(),
		})
	}

	return entries, nil
}
