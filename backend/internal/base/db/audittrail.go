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
