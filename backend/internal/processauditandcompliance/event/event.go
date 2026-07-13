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

// ComplianceRisk is one detected policy-adherence violation, embedded in
// ComplianceMonitorEvent so the audit trail records what the sweep flagged.
type ComplianceRisk struct {
	DID        string    `json:"did"`
	RiskType   string    `json:"risk_type"`
	Detail     string    `json:"detail"`
	DetectedAt time.Time `json:"detected_at"`
}

// ComplianceMonitorEvent is emitted for every continuous-monitoring sweep
// (GET /pac/monitor, DCS-IR-PACM-03) — including clean sweeps, so the audit
// trail proves monitoring actually ran, not only that risks were found.
type ComplianceMonitorEvent struct {
	MonitoredBy string             `json:"monitored_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	Risks       []ComplianceRisk   `json:"risks"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ComplianceMonitorEvent) EventType() string {
	return "PAC_COMPLIANCE_MONITOR"
}

// GetDID implements the Event interface.
func (e ComplianceMonitorEvent) GetDID() string {
	return "*"
}

// ComplianceRiskEvent anchors one detected compliance risk against the
// affected contract's PAC audit chain. The sweep-level ComplianceMonitorEvent
// carries no resource DID (GetDID "*") and therefore only enters the global
// chain — per-resource anchoring is what makes a flagged risk visible in a
// PROCESS_AUDIT_AND_COMPLIANCE-scope audit read ("flagged ... for manual
// review", DCS-FR-PACM-03).
type ComplianceRiskEvent struct {
	DID         string             `json:"did"`
	RiskType    string             `json:"risk_type"`
	Detail      string             `json:"detail"`
	MonitoredBy string             `json:"monitored_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ComplianceRiskEvent) EventType() string {
	return "PAC_COMPLIANCE_RISK"
}

// GetDID implements the Event interface.
func (e ComplianceRiskEvent) GetDID() string {
	return e.DID
}
