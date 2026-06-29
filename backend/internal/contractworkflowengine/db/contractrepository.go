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
	TemplateDID     string         `db:"template_did"`
	TemplateVersion int            `db:"template_version"`
}

type ContractMetadata struct {
	DID                  string       `db:"did"`
	ContractVersion      int          `db:"contract_version"`
	State                string       `db:"state"`
	CreatedBy            string       `db:"created_by"`
	CreatedAt            time.Time    `db:"created_at"`
	UpdatedAt            time.Time    `db:"updated_at"`
	StartDate            *time.Time   `db:"start_date"`
	ExpDate              *time.Time   `db:"exp_date"`
	ExpPolicy            *string      `db:"exp_policy"`
	ExpNoticePeriod      *int         `db:"exp_notice_period"`
	Name                 *string      `db:"name"`
	Responsible          *Responsible `db:"responsible"`
	Description          *string      `db:"description"`
	TemplateDID          string       `db:"template_did"`
	TemplateVersion      int          `db:"template_version"`
	Outdated             *bool        `db:"outdated"`
	LatestTemplateDID    *string      `db:"latest_template_did"`
	TemplateIsDeprecated *bool        `db:"template_is_deprecated"`
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
	State           string         `db:"state"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractVersion int            `db:"contract_version"`
	ContractData    *datatype.JSON `db:"contract_data"`
	StartDate       *time.Time     `db:"start_date"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Responsible     *Responsible   `db:"responsible"`
}

type ContractHistory struct {
	ID              string         `db:"id"`
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
	TemplateDID     string         `db:"template_did"`
	TemplateVersion int            `db:"template_version"`
}

type ContractArchiveEntry struct {
	DID                  string         `db:"did"`
	ContractVersion      int            `db:"contract_version"`
	StoredBy             string         `db:"stored_by"`
	StoredAt             time.Time      `db:"stored_at"`
	ArchiveStatus        string         `db:"archive_status"`
	ContractSnapshot     datatype.JSON  `db:"contract_snapshot"`
	ContentHash          string         `db:"content_hash"`
	SnapshotCID          string         `db:"snapshot_cid"`
	SnapshotCIDCreatedAt time.Time      `db:"snapshot_cid_created_at"`
	SignatureMeta        *datatype.JSON `db:"signature_metadata"`
	CredentialHashes     *datatype.JSON `db:"credential_hashes"`
	TSAReceipt           *datatype.JSON `db:"tsa_receipt"`
	Evidence             *datatype.JSON `db:"evidence"`
	RetentionUntil       *time.Time     `db:"retention_until"`
	DeletedAt            *time.Time     `db:"deleted_at"`
	DeletedBy            *string        `db:"deleted_by"`
	DeletionReason       *string        `db:"deletion_reason"`
}

type SearchValues struct {
	DID             string
	ContractVersion int
	State           string
	Name            string
	Description     string
	ContractData    string
}

type ContractPDFState struct {
	IPFSCID         string `db:"pdf_ipfs_cid"`
	RendererVersion string `db:"pdf_renderer_version"`
	C2PAState       string `db:"pdf_c2pa_state"`
}

type ContractRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data Contract) (*time.Time, error)
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractHistory, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	StoreArchiveEntry(ctx context.Context, tx *sqlx.Tx, data ContractArchiveEntry) error
	ReadArchiveEntries(ctx context.Context, tx *sqlx.Tx) ([]ContractArchiveEntry, error)
	ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues, pagination datatype.Pagination) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
	ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*ContractPDFState, error)
	UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data ContractPDFState) error
}
