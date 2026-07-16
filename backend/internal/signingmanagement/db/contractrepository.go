package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

// ErrSignatureNotFound reports a revocation (or lookup) that named a signer
// with no matching signature row on the contract — surfaced instead of
// letting a zero-row UPDATE pass as success.
var ErrSignatureNotFound = errors.New("signature not found")

type Responsible struct {
	Creator     string   `json:"creator"`
	Approvers   []string `json:"approvers"`
	Reviewers   []string `json:"reviewers"`
	Negotiators []string `json:"negotiators"`
}

func (r Responsible) Value() (driver.Value, error) {
	return json.Marshal(r)
}

func (r *Responsible) Scan(src any) error {
	if src == nil {
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported type: %T", src)
	}
	return json.Unmarshal(b, r)
}

type Contract struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	StartDate       *time.Time     `db:"start_date"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	Responsible     *Responsible   `db:"responsible"`
	ContractData    *datatype.JSON `db:"contract_data"`
}

type ContractMetadata struct {
	DID             string       `db:"did"`
	ContractVersion int          `db:"contract_version"`
	State           string       `db:"state"`
	CreatedBy       string       `db:"created_by"`
	CreatedAt       time.Time    `db:"created_at"`
	UpdatedAt       time.Time    `db:"updated_at"`
	StartDate       *time.Time   `db:"start_date"`
	ExpDate         *time.Time   `db:"exp_date"`
	ExpPolicy       *string      `db:"exp_policy"`
	ExpNoticePeriod *int         `db:"exp_notice_period"`
	Name            *string      `db:"name"`
	Responsible     *Responsible `db:"responsible"`
	Description     *string      `db:"description"`
}

type ContractProcessData struct {
	DID             string     `db:"did"`
	ContractVersion int        `db:"contract_version"`
	State           string     `db:"state"`
	CreatedBy       string     `db:"created_by"`
	UpdatedAt       time.Time  `db:"updated_at"`
	StartDate       *time.Time `db:"start_date"`
	ExpDate         *time.Time `db:"exp_date"`
	ExpPolicy       *string    `db:"exp_policy"`
	ExpNoticePeriod *int       `db:"exp_notice_period"`
}

type ContractUpdateData struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractData    *datatype.JSON `db:"contract_data"`
}

type ContractSignature struct {
	ContractDID    string     `db:"contract_did"`
	SignerDID      string     `db:"signer_did"`
	CredentialType string     `db:"credential_type"`
	Status         string     `db:"status"`
	SignedAt       *time.Time `db:"signed_at"`
	RevokedAt      *time.Time `db:"revoked_at"`
	CertRevokedAt  *time.Time `db:"cert_revoked_at"`
	IpfsCID        *string    `db:"ipfs_cid"`
	SignatureBytes []byte     `db:"signature_bytes"`
	KeyVersion     int        `db:"key_version"`
	CeremonyID     *string    `db:"ceremony_id"`
	PDFHash        *string    `db:"pdf_hash"`
	ContentHash    *string    `db:"content_hash"`
	FieldName      *string    `db:"field_name"`
}

type ContractSignatureEnvelope struct {
	ContractDID    string  `db:"contract_did"`
	SignerDID      string  `db:"signer_did"`
	CredentialType string  `db:"credential_type"`
	Status         string  `db:"status"`
	SignedAt       *string `db:"signed_at"`
	RevokedAt      *string `db:"revoked_at"`
	IpfsCID        *string `db:"ipfs_cid"`
	KeyVersion     int     `db:"key_version"`
}

type ContractSigningTask struct {
	ContractDID     string    `db:"contract_did"`
	ContractVersion int       `db:"contract_version"`
	State           string    `db:"state"`
	SignerDID       string    `db:"signer_did"`
	CreatedAt       time.Time `db:"created_at"`
}

type SignatureRecord struct {
	SignerDID      string     `db:"signer_did"`
	CredentialType string     `db:"credential_type"`
	Status         string     `db:"status"`
	SignedAt       *time.Time `db:"signed_at"`
	RevokedAt      *time.Time `db:"revoked_at"`
	CertRevokedAt  *time.Time `db:"cert_revoked_at"`
	// FieldName is the declared signature field this signature covers
	// (DCS-FR-SM-07/-17); nil for signatures predating multi-signer support.
	FieldName *string `db:"field_name"`
}

type ContractRepo interface {
	ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	UpdateContractData(ctx context.Context, tx *sqlx.Tx, did string, contractData datatype.JSON) error

	CreateSignature(ctx context.Context, tx *sqlx.Tx, signature ContractSignature) error
	// SetSignedPDF points the contract at the PAdES-signed PDF artefact in IPFS
	// and records its C2PA lifecycle state and payload hash so the export/verify
	// endpoints recognize this artefact as already up to date and serve it
	// frozen — never re-embedding a C2PA assertion into an already-signed PDF
	// (any post-signature attachment mutation is flagged as an illegal
	// modification by standards-compliant PAdES validators).
	SetSignedPDF(ctx context.Context, tx *sqlx.Tx, did, ipfsCID, rendererVersion, c2paState, payloadHash string) error
	RevokeSignature(ctx context.Context, tx *sqlx.Tx, did string, signerDID string) error
	ActiveKeyVersion(ctx context.Context, tx *sqlx.Tx, label string) (int, error)
	ReadLatestEnvelopeByContractDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractSignatureEnvelope, error)
	ReadAllSigningTasks(ctx context.Context, tx *sqlx.Tx) ([]ContractSigningTask, error)
	CountSignatureForContractDID(ctx context.Context, tx *sqlx.Tx, did string) (int, error)
	FetchContractPDFBytes(ctx context.Context, tx *sqlx.Tx, did string) ([]byte, error)
	CollectValidationFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	LoadSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]SignatureRecord, error)
	CollectComplianceFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
}
