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

type ResponsiblePersons struct {
	Creator     string   `json:"creator"`
	Approvers   []string `json:"approvers"`
	Reviewers   []string `json:"reviewers"`
	Negotiators []string `json:"negotiators"`
}

func (r ResponsiblePersons) Value() (driver.Value, error) {
	return json.Marshal(r)
}

func (r *ResponsiblePersons) Scan(src any) error {
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
	DID                string              `db:"did"`
	ContractVersion    int                 `db:"contract_version"`
	State              string              `db:"state"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at"`
	StartDate          *time.Time          `db:"start_date"`
	ExpDate            *time.Time          `db:"exp_date"`
	ExpPolicy          *string             `db:"exp_policy"`
	ExpNoticePeriod    *int                `db:"exp_notice_period"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	ContractData       *datatype.JSON      `db:"contract_data"`
}

type ContractMetadata struct {
	DID                string              `db:"did"`
	ContractVersion    int                 `db:"contract_version"`
	State              string              `db:"state"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at"`
	StartDate          *time.Time          `db:"start_date"`
	ExpDate            *time.Time          `db:"exp_date"`
	ExpPolicy          *string             `db:"exp_policy"`
	ExpNoticePeriod    *int                `db:"exp_notice_period"`
	Name               *string             `db:"name"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	Description        *string             `db:"description"`
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
	DID                string              `db:"did"`
	State              string              `db:"state"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	ContractVersion    int                 `db:"contract_version"`
	ContractData       *datatype.JSON      `db:"contract_data"`
	StartDate          *time.Time          `db:"start_date"`
	ExpDate            *time.Time          `db:"exp_date"`
	ExpPolicy          *string             `db:"exp_policy"`
	ExpNoticePeriod    *int                `db:"exp_notice_period"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
}

type ContractHistory struct {
	ID                 string              `db:"id"`
	DID                string              `db:"did"`
	ContractVersion    int                 `db:"contract_version"`
	State              string              `db:"state"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at"`
	StartDate          *time.Time          `db:"start_date"`
	ExpDate            *time.Time          `db:"exp_date"`
	ExpPolicy          *string             `db:"exp_policy"`
	ExpNoticePeriod    *int                `db:"exp_notice_period"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	ContractData       *datatype.JSON      `db:"contract_data"`
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

type ContractRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data Contract) (*time.Time, error)
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractHistory, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	StoreArchiveEntry(ctx context.Context, tx *sqlx.Tx, data ContractArchiveEntry) error
	ReadArchiveEntries(ctx context.Context, tx *sqlx.Tx) ([]ContractArchiveEntry, error)
	ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
}
