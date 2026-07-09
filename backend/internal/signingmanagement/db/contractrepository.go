package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

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
}

type ContractRepo interface {
	ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error

	CreateSignature(ctx context.Context, tx *sqlx.Tx, signature ContractSignature) error
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
