// Package db holds the base-level repository interfaces shared across
// domains: the audit-trail CID store (this file) and the transactional
// outbox (see PersistEvent/UpdateOutboxEvent). db/pq holds the Postgres
// implementations.
package db

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type AuditTrailRepository interface {
	ReadLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string) (*string, error)
	ReadLogCIDs(ctx context.Context, tx *sqlx.Tx, component string) ([]*string, error)
	UpdateLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string, logCID *string) error
}
