package contractstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// ContractState represents the lifecycle state of a contract
type ContractState string

const (
	Draft       ContractState = "DRAFT"
	Negotiation ContractState = "NEGOTIATION"
	Submitted   ContractState = "SUBMITTED"
	Reviewed    ContractState = "REVIEWED"
	Approved    ContractState = "APPROVED"
	Deleted     ContractState = "DELETED"
	Deprecated  ContractState = "DEPRECATED"
)

var validStates = map[ContractState]bool{
	Draft:       true,
	Negotiation: true,
	Submitted:   true,
	Reviewed:    true,
	Approved:    true,
	Deleted:     true,
	Deprecated:  true,
}

func NewContractState(s string) (ContractState, error) {
	ts := ContractState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid contract state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the ContractState is a valid role
func (s ContractState) IsValid() bool {
	upper := ContractState(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the ContractState
func (s ContractState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ContractState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("contract state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ContractState: %T", value)
	}

	state, err := NewContractState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ContractState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid contract state: %s", s)
	}
	return string(s), nil
}
