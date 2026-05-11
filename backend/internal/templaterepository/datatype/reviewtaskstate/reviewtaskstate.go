package reviewtaskstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type ReviewTaskState string

const (
	Open     ReviewTaskState = "OPEN"
	Rejected ReviewTaskState = "REJECTED"
	Verified ReviewTaskState = "VERIFIED"
	Approved ReviewTaskState = "APPROVED"
)

var validState = map[ReviewTaskState]bool{
	Open:     true,
	Rejected: true,
	Verified: true,
	Approved: true,
}

func NewReviewTaskState(s string) (ReviewTaskState, error) {
	ts := ReviewTaskState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid review task state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the ReviewTaskState is a valid role
func (s ReviewTaskState) IsValid() bool {
	upper := ReviewTaskState(strings.ToUpper(string(s)))
	return validState[upper]
}

// String returns the string representation of the ReviewTaskState
func (s ReviewTaskState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ReviewTaskState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("review task state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ReviewTaskState: %T", value)
	}

	state, err := NewReviewTaskState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ReviewTaskState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid review task state: %s", s)
	}
	return string(s), nil
}
