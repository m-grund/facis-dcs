package pq

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
)

func PostgresPersistEvent(ctx context.Context, tx *sqlx.Tx, component componenttype.ComponentType, eventType string, eventJSON []byte, did string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO outbox_events 
		 (component, event_type, event_data, did, processed)
		 VALUES ($1, $2, $3, $4, FALSE)`,
		component.String(),
		eventType,
		eventJSON,
		did,
	)

	if err != nil {
		return fmt.Errorf("failed to insert event into outbox: %w", err)
	}

	return nil
}

func PostgresUpdateOutboxEvent(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `
        UPDATE outbox_events
        SET processed = TRUE, processed_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `, id)

	if err != nil {
		return fmt.Errorf("could not mark event %d as processed: %w", id, err)
	}

	return nil
}

func PostgresMarkOutboxEventPublished(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `
        UPDATE outbox_events
        SET published = TRUE, published_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `, id)

	if err != nil {
		return fmt.Errorf("could not mark event %d as published: %w", id, err)
	}

	return nil
}
