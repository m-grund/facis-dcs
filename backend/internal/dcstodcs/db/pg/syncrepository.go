// Package pq is the Postgres implementation of dcstodcs's sync repository
// (trusted-peer allowlist + sync-fail retry queue).
package pq

import (
	"context"

	"digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"
)

type PostgresSyncRepository struct{}

func (r PostgresSyncRepository) IsTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM trusted_peers WHERE peer_did = $1)`
	if err := tx.GetContext(ctx, &exists, query, peerDID); err != nil {
		return false, err
	}
	return exists, nil
}

func (r PostgresSyncRepository) CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO sync_fails (did, retry_count, created_at, last_tried_at)
        VALUES ($1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (did) DO UPDATE SET
            retry_count   = sync_fails.retry_count + 1,
            last_tried_at = CURRENT_TIMESTAMP
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r PostgresSyncRepository) DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM sync_fails WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r PostgresSyncRepository) GetPendingSyncFails(ctx context.Context, tx *sqlx.Tx) ([]db.SyncFail, error) {
	query := `
        SELECT *
        FROM sync_fails
    `
	var syncFails []db.SyncFail
	err := tx.SelectContext(ctx, &syncFails, query)
	if err != nil {
		return nil, err
	}
	return syncFails, nil
}
