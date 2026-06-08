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
	Username      string                      `json:"username"`
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
	return e.DID
}
