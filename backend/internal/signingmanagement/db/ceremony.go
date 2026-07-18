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
	// The published OID4VP Document-Retrieval signing request (ADR-12): the
	// to-be-signed PDF and its digest the wallet fetches and signs, the request
	// object's nonce/expiry, and the publishing signer's participant context the
	// JWT-less callback replays into finalize. All nil until publish.
	PreparedPDF        []byte     `db:"prepared_pdf"`
	PreparedPDFSHA256  *string    `db:"prepared_pdf_sha256"`
	RequestNonce       *string    `db:"request_nonce"`
	RequestExpiresAt   *time.Time `db:"request_expires_at"`
	CredentialType     *string    `db:"credential_type"`
	PublishedBy        *string    `db:"published_by"`
	PublishedHolderDID *string    `db:"published_holder_did"`
	PublishedRoles     []byte     `db:"published_roles"`
	ConsumedAt         *time.Time `db:"consumed_at"`
}

// PreparedRequest carries the published OID4VP signing request state persisted on
// a ceremony at publish (ADR-12).
type PreparedRequest struct {
	CeremonyID        string
	PreparedPDF       []byte
	PreparedPDFSHA256 string
	RequestNonce      string
	RequestExpiresAt  time.Time
	CredentialType    string
	PublishedBy       string
	HolderDID         string
	Roles             []byte
}

// CeremonyRepo persists signing ceremonies.
type CeremonyRepo interface {
	CreateCeremony(ctx context.Context, tx *sqlx.Tx, c SignatureCeremony) error
	GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*SignatureCeremony, error)
	MarkCeremonyVerified(ctx context.Context, tx *sqlx.Tx, id, signerDID, vpToken string, pidClaims []byte, kbSdHash string) error
	// StorePreparedRequest persists the published signing request (the
	// to-be-signed PDF + digest + request object nonce/expiry + the publishing
	// signer's context) on a verified ceremony (ADR-12 publish).
	StorePreparedRequest(ctx context.Context, tx *sqlx.Tx, req PreparedRequest) error
	// MarkCeremonyConsumed records that the signed document has been accepted at
	// the callback, so a published request is single-use.
	MarkCeremonyConsumed(ctx context.Context, tx *sqlx.Tx, id string) error
	// FindVerifiedCeremony returns the most recent verified ceremony for the
	// given contract and signer, or (nil, nil) when none exists.
	FindVerifiedCeremony(ctx context.Context, tx *sqlx.Tx, contractDID, signerDID string) (*SignatureCeremony, error)
	// FindVerifiedCeremonyByField returns the most recent verified ceremony
	// for the given contract and signature FIELD, or (nil, nil) when none
	// exists — the all-ceremonies-before-first-signature gate of the
	// multi-signer flow (DCS-FR-SM-07/-17) checks every declared field.
	FindVerifiedCeremonyByField(ctx context.Context, tx *sqlx.Tx, contractDID, fieldName string) (*SignatureCeremony, error)
}
