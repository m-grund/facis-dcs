package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	Validate             EventType = "VALIDATE_SIGNATURE"
	RetrieveAll          EventType = "RETRIEVE_ALL_CONTRACTS"
	RetrieveByID         EventType = "RETRIEVE_CONTRACT_BY_ID"
	Revoke               EventType = "REVOKE_SIGNATURE"
	ComplianceValidation EventType = "COMPLIANCE_VALIDATION"
	Audit                EventType = "AUDIT_CONTRACT_TEMPLATE"
)

var validStates = map[EventType]bool{
	Validate:             true,
	RetrieveAll:          true,
	RetrieveByID:         true,
	Revoke:               true,
	ComplianceValidation: true,
	Audit:                true,
}

func NewEventType(s string) (EventType, error) {
	ts := EventType(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid event type: %s", s)
	}
	return ts, nil
}

// IsValid checks if the EventType is a valid role
func (s EventType) IsValid() bool {
	upper := EventType(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the EventType
func (s EventType) String() string {
	return string(s)
}
