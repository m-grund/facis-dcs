package signingtaskstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type SigningTaskState string

const (
	Open   SigningTaskState = "OPEN"
	Signed SigningTaskState = "SIGNED"
)

var validStates = map[SigningTaskState]bool{
	Open:   true,
	Signed: true,
}

func NewSigningTaskState(s string) (SigningTaskState, error) {
	ts := SigningTaskState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid signing task state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the SigningTaskState is a valid role
func (s SigningTaskState) IsValid() bool {
	upper := SigningTaskState(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the SigningTaskState
func (s SigningTaskState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *SigningTaskState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("signing task state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for SigningTaskState: %T", value)
	}

	state, err := NewSigningTaskState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s SigningTaskState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid signing task state: %s", s)
	}
	return string(s), nil
}
