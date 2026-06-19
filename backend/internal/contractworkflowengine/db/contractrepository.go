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
	TemplateDID     string       `db:"template_did"`
	TemplateVersion int          `db:"template_version"`
	Outdated        *bool        `db:"outdated"`
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
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues, pagination datatype.Pagination) ([]ContractMetadata, error)
	ReadExpiredContacts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
	ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*ContractPDFState, error)
	UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data ContractPDFState) error
}
