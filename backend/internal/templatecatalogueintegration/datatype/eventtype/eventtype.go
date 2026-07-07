// Package eventtype enumerates template catalogue integration event type strings.
package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	RetrieveAll  EventType = "RETRIEVE_ALL_TEMPLATE_CATALOGUE"
	RetrieveByID EventType = "RETRIEVE_TEMPLATE_CATALOGUE_BY_ID"
	Search       EventType = "SEARCH_TEMPLATE_CATALOGUE"
)

var validTypes = map[EventType]bool{
	RetrieveAll:  true,
	RetrieveByID: true,
	Search:       true,
}

func NewEventType(s string) (EventType, error) {
	ts := EventType(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid event type: %s", s)
	}
	return ts, nil
}

func (s EventType) IsValid() bool {
	return validTypes[EventType(strings.ToUpper(string(s)))]
}

func (s EventType) String() string {
	return string(s)
}
