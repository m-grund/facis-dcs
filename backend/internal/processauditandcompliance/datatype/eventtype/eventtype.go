package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	TemplatePolicyAuditFinding             EventType = "TEMPLATE_POLICY_AUDIT_FINDING"
	TemplateApprovalProvenanceAuditFinding EventType = "TEMPLATE_APPROVAL_PROVENANCE_AUDIT_FINDING"
	ContractContentPolicyAuditFinding      EventType = "CONTRACT_CONTENT_POLICY_AUDIT_FINDING"
)

var validType = map[EventType]bool{
	TemplatePolicyAuditFinding:             true,
	TemplateApprovalProvenanceAuditFinding: true,
	ContractContentPolicyAuditFinding:      true,
}

func NewEventType(s string) (EventType, error) {
	flag := EventType(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid action flag: %s", s)
	}
	return flag, nil
}

// IsValid checks if the EventType is a valid role
func (f EventType) IsValid() bool {
	upper := EventType(strings.ToUpper(string(f)))
	return validType[upper]
}

// String returns the string representation of the EventType
func (f EventType) String() string {
	return string(f)
}
