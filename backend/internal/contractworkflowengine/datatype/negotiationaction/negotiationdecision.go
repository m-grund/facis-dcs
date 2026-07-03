// Package negotiationdescision (folder name negotiationaction, package name
// negotiationdescision — pre-existing naming mismatch/typo, kept for import
// compatibility) is a single negotiator's decision on one change request,
// distinct from NegotiationTaskState in the sibling negotiationtaskstate
// package: this tracks the outcome of one specific change request, while
// NegotiationTaskState tracks whether that negotiator has responded at all
// in the current round.
package negotiationdescision

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type NegotiationDecision string

const (
	Accepted NegotiationDecision = "ACCEPTED"
	Rejected NegotiationDecision = "REJECTED"
	Closed   NegotiationDecision = "CLOSED"
)

var validState = map[NegotiationDecision]bool{
	Accepted: true,
	Rejected: true,
	Closed:   true,
}

func NewNegotiationDecision(s string) (NegotiationDecision, error) {
	ts := NegotiationDecision(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid negotiation decision: %s", s)
	}
	return ts, nil
}

// IsValid checks if the NegotiationDecision is a valid role
func (s NegotiationDecision) IsValid() bool {
	upper := NegotiationDecision(strings.ToUpper(string(s)))
	return validState[upper]
}

// String returns the string representation of the NegotiationDecision
func (s NegotiationDecision) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *NegotiationDecision) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("negotiation decision cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for NegotiationDecision: %T", value)
	}

	state, err := NewNegotiationDecision(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s NegotiationDecision) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid negotiation decision: %s", s)
	}
	return string(s), nil
}
