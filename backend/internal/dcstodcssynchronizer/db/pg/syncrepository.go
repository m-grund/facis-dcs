package pq

import (
	"context"

	"digital-contracting-service/internal/dcstodcssynchronizer/db"

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

func (r PostgresSyncRepository) CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error {
	statement := `
        INSERT INTO sync_fails (peer_did, created_at)
        VALUES ($1, CURRENT_TIMESTAMP)
        ON CONFLICT (peer_did) DO UPDATE SET
            created_at = CURRENT_TIMESTAMP
    `
	_, err := tx.ExecContext(ctx, statement, peerDID)
	return err
}

func (r PostgresSyncRepository) DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error {
	statement := `
        DELETE FROM sync_fails WHERE peer_did = $1
    `
	_, err := tx.ExecContext(ctx, statement, peerDID)
	return err
}

func (r PostgresSyncRepository) ReadAllSyncFailEntries(ctx context.Context, tx *sqlx.Tx) ([]db.SyncFails, error) {
	query := `
        SELECT *
        FROM sync_fails
    `
	var syncFails []db.SyncFails
	err := tx.SelectContext(ctx, &syncFails, query)
	if err != nil {
		return nil, err
	}
	return syncFails, nil
}
