package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	PresentationSucceeded EventType = "OID4VP_PRESENTATION_SUCCEEDED"
	PresentationFailed    EventType = "OID4VP_PRESENTATION_FAILED"
)

var validTypes = map[EventType]bool{
	PresentationSucceeded: true,
	PresentationFailed:    true,
}

func NewEventType(s string) (EventType, error) {
	t := EventType(strings.ToUpper(s))
	if !t.IsValid() {
		return "", fmt.Errorf("invalid auth event type: %s", s)
	}
	return t, nil
}

func (t EventType) IsValid() bool {
	return validTypes[EventType(strings.ToUpper(string(t)))]
}

func (t EventType) String() string {
	return string(t)
}
