package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type TrustedPeer struct {
	PeerDID string `db:"peer_did"`
}

type SyncFails struct {
	ID        uint64    `db:"id"`
	PeerDID   string    `db:"peer_did"`
	CreatedAt time.Time `db:"created_at"`
}

type SyncRepository interface {
	IsTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) (bool, error)

	ReadAllSyncFailEntries(ctx context.Context, tx *sqlx.Tx) ([]SyncFails, error)

	CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error
	DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error
}
