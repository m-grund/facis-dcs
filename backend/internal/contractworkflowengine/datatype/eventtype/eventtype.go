package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	Create                  EventType = "CREATE_CONTRACT"
	Submit                  EventType = "SUBMIT_CONTRACT"
	Negotiation             EventType = "NEGOTIATE_CONTRACT"
	AcceptRespond           EventType = "ACCEPT_RESPOND_CONTRACT"
	RejectRespond           EventType = "REJECT_RESPOND_CONTRACT"
	IncreaseContractVersion EventType = "INCREASE_CONTRACT_VERSION"
	Approve                 EventType = "APPROVE_CONTRACT"
	Reject                  EventType = "REJECT_CONTRACT"
	Verify                  EventType = "VERIFY_CONTRACT"
	Update                  EventType = "UPDATE_CONTRACT"
	RetrieveAll             EventType = "RETRIEVE_ALL_CONTRACTS"
	RetrieveByID            EventType = "RETRIEVE_CONTRACT_BY_ID"
	Search                  EventType = "SEARCH_CONTRACT"
	Review                  EventType = "REVIEW_CONTRACT"
	Audit                   EventType = "AUDIT_CONTRACT"
	Terminate               EventType = "TERMINATE_CONTRACT"
	RecordEvidence          EventType = "RECORD_EVIDENCE"
)

var validStates = map[EventType]bool{
	Create:                  true,
	Submit:                  true,
	Negotiation:             true,
	AcceptRespond:           true,
	RejectRespond:           true,
	IncreaseContractVersion: true,
	Approve:                 true,
	Reject:                  true,
	Verify:                  true,
	Update:                  true,
	RetrieveAll:             true,
	RetrieveByID:            true,
	Search:                  true,
	Review:                  true,
	Audit:                   true,
	Terminate:               true,
	RecordEvidence:          true,
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
