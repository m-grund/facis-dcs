// Package db holds the repository interface backing dcstodcs's trust
// allowlist (TrustedPeer) and its retry queue for failed peer broadcasts
// (SyncFail); db/pg holds the Postgres implementation.
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
	ID          uint64    `db:"id"`
	DID         string    `db:"did"`
	RetryCount  int       `db:"retry_count"`
	CreatedAt   time.Time `db:"created_at"`
	LastTriedAt time.Time `db:"last_tried_at"`
}

// SyncSignature is the origin peer's JAdES signature over a synced
// contract's canonical representation (DCS-FR-SM-02), persisted on the
// receiving instance as the contract's cross-instance provenance artifact.
type SyncSignature struct {
	DID             string    `db:"did"`
	ContractVersion int       `db:"contract_version"`
	FromPeerDID     string    `db:"from_peer_did"`
	JadesSignature  string    `db:"jades_signature"`
	ReceivedAt      time.Time `db:"received_at"`
}

type SyncRepository interface {
	IsTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) (bool, error)
	UpsertTrustedPeer(ctx context.Context, tx *sqlx.Tx, peerDID string) error

	GetPendingSyncFails(ctx context.Context, tx *sqlx.Tx) ([]SyncFail, error)
	CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string) error
	DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error

	// UpsertSyncSignature stores the latest verified JAdES signature received
	// for a synced contract; GetSyncSignature returns nil when none exists.
	UpsertSyncSignature(ctx context.Context, tx *sqlx.Tx, sig SyncSignature) error
	GetSyncSignature(ctx context.Context, tx *sqlx.Tx, did string) (*SyncSignature, error)
}
