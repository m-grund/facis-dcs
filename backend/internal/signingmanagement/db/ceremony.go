package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// Ceremony lifecycle statuses (FR-SM-14). A signature may only be applied once
// a ceremony for the signer+contract has reached CeremonyVerified.
const (
	CeremonyPending  = "pending"
	CeremonyVerified = "verified"
	CeremonyExpired  = "expired"
	CeremonyFailed   = "failed"
)

// SignatureCeremony is a signing ceremony: a request for the signer's wallet to
// present a PID (via EUDIPLO/OID4VP) that must complete before a PAdES
// signature can be applied (FR-SM-14, UC-04-02).
type SignatureCeremony struct {
	ID          string     `db:"id"`
	ContractDID string     `db:"contract_did"`
	FieldName   string     `db:"field_name"`
	RequestedBy string     `db:"requested_by"`
	Status      string     `db:"status"`
	WalletURI   *string    `db:"wallet_uri"`
	Nonce       string     `db:"nonce"`
	SignerDID   *string    `db:"signer_did"`
	VpToken     *string    `db:"vp_token"`
	PidClaims   []byte     `db:"pid_claims"`
	KbSdHash    *string    `db:"kb_sd_hash"`
	CreatedAt   time.Time  `db:"created_at"`
	VerifiedAt  *time.Time `db:"verified_at"`
	ExpiresAt   time.Time  `db:"expires_at"`
}

// CeremonyRepo persists signing ceremonies.
type CeremonyRepo interface {
	CreateCeremony(ctx context.Context, tx *sqlx.Tx, c SignatureCeremony) error
	GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*SignatureCeremony, error)
	MarkCeremonyVerified(ctx context.Context, tx *sqlx.Tx, id, signerDID, vpToken string, pidClaims []byte, kbSdHash string) error
	// FindVerifiedCeremony returns the most recent verified ceremony for the
	// given contract and signer, or (nil, nil) when none exists.
	FindVerifiedCeremony(ctx context.Context, tx *sqlx.Tx, contractDID, signerDID string) (*SignatureCeremony, error)
}
