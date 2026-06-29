package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type TrustedPeer struct {
	PeerDID string `db:"peer_did"`
}

type SyncFail struct {
	ID        uint64    `db:"id"`
	DID       string    `db:"did"`
	CreatedAt time.Time `db:"created_at"`
}

type SyncRepository interface {
	IsTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) (bool, error)

	ReadAllSyncFailEntries(ctx context.Context, tx *sqlx.Tx) ([]SyncFail, error)
	CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string) error
	DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error
}
