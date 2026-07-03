package event

import (
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
)

// AuditEvent is emitted when the contract is audited
type AuditEvent struct {
	DID           string                      `json:"did"`
	HolderDID     string                      `json:"holder_did"`
	AuditedBy     string                      `json:"audited_by"`
	OccurredAt    time.Time                   `json:"occurred_at"`
	ComponentType componenttype.ComponentType `json:"component_type"`
	Scope         componenttype.ComponentType `json:"scope"`
	UserRoles     userrole.UserRoles          `json:"user_roles"`
}

// EventType implements the Event interface.
func (e AuditEvent) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvent) GetDID() string {
	if e.DID == "" {
		return "*"
	}
	return e.DID
}

// ReportGeneratedEvent is emitted when PACM generates an audit report.
type ReportGeneratedEvent struct {
	ReportID    string             `json:"report_id"`
	Scope       string             `json:"scope"`
	Format      string             `json:"format"`
	DID         string             `json:"did,omitempty"`
	GeneratedBy string             `json:"generated_by"`
	GeneratedAt time.Time          `json:"generated_at"`
	ContentHash string             `json:"content_hash"`
	Summary     map[string]int     `json:"summary"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ReportGeneratedEvent) EventType() string {
	return "PAC_REPORT_GENERATED"
}

// GetDID implements the Event interface.
func (e ReportGeneratedEvent) GetDID() string {
	if e.DID == "" {
		return "*"
	}
	return e.DID
}
