package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
)

type Event interface {
	// EventType returns the name of the event (used as NATS subject).
	EventType() string

	// GetDID returns the entity DID for event reference and correlation.
	GetDID() string
}

func Create(ctx context.Context, tx *sqlx.Tx, evt Event, component componenttype.ComponentType) error {
	if evt == nil {
		return errors.New("event cannot be nil")
	}

	eventType := evt.EventType()
	if eventType == "" {
		return errors.New("event type cannot be empty")
	}

	did := evt.GetDID()
	if did == "" {
		return errors.New("did cannot be empty")
	}

	eventJSON, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = db.PersistEvent(ctx, tx, component, eventType, eventJSON, did)
	if err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	return nil
}
