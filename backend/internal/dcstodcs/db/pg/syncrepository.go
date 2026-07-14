// Package pq is the Postgres implementation of dcstodcs's sync repository
// (trusted-peer allowlist + sync-fail retry queue).
package pq

import (
	"context"
	"database/sql"
	"errors"

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

// UpsertTrustedPeer idempotently seeds peerDID into the trusted_peers
// allowlist (peer_did is the primary key, see
// backend/migrations/sql/20260626_synchronization.sql) — used both by
// startup seeding from DCS_TRUSTED_PEERS and, potentially,
// future admin tooling. Mirrors CreateOrUpdateSyncFailEntry's
// ON CONFLICT ... DO NOTHING idempotency pattern above.
func (r PostgresSyncRepository) UpsertTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) error {
	statement := `
        INSERT INTO trusted_peers (peer_did)
        VALUES ($1)
        ON CONFLICT (peer_did) DO NOTHING
    `
	_, err := tx.ExecContext(ctx, statement, peerDID)
	return err
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

func (r PostgresSyncRepository) UpsertSyncSignature(ctx context.Context, tx *sqlx.Tx, sig db.SyncSignature) error {
	statement := `
        INSERT INTO contract_sync_signatures (did, contract_version, from_peer_did, jades_signature, received_at)
        VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
        ON CONFLICT (did) DO UPDATE SET
            contract_version = EXCLUDED.contract_version,
            from_peer_did    = EXCLUDED.from_peer_did,
            jades_signature  = EXCLUDED.jades_signature,
            received_at      = CURRENT_TIMESTAMP
    `
	_, err := tx.ExecContext(ctx, statement, sig.DID, sig.ContractVersion, sig.FromPeerDID, sig.JadesSignature)
	return err
}

func (r PostgresSyncRepository) GetSyncSignature(ctx context.Context, tx *sqlx.Tx, did string) (*db.SyncSignature, error) {
	query := `
        SELECT did, contract_version, from_peer_did, jades_signature, received_at
        FROM contract_sync_signatures
        WHERE did = $1
    `
	var sig db.SyncSignature
	err := tx.GetContext(ctx, &sig, query, did)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sig, nil
}
