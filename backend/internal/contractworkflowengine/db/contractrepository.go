package db

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"time"

	"github.com/jmoiron/sqlx"
)

type Contract struct {
	DID             string         `db:"did"`
	ContractVersion *int           `db:"contract_version"`
	State           string         `db:"state"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractData    *datatype.JSON `db:"contract_data"`
}

type ContractMetadata struct {
	DID             string     `db:"did"`
	ContractVersion *int       `db:"contract_version"`
	State           string     `db:"state"`
	CreatedBy       string     `db:"created_by"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	ExpDate         *time.Time `db:"exp_date"`
	ExpPolicy       *string    `db:"exp_policy"`
	ExpNoticePeriod *int       `db:"exp_notice_period"`
	Name            *string    `db:"name"`
	Description     *string    `db:"description"`
}

type ContractProcessData struct {
	DID             string     `db:"did"`
	ContractVersion *int       `db:"contract_version"`
	State           string     `db:"state"`
	CreatedBy       string     `db:"created_by"`
	UpdatedAt       time.Time  `db:"updated_at"`
	ExpDate         *time.Time `db:"exp_date"`
	ExpPolicy       *string    `db:"exp_policy"`
	ExpNoticePeriod *int       `db:"exp_notice_period"`
}

type ContractUpdateData struct {
	DID             string         `db:"did"`
	ContractVersion *int           `db:"contract_version"`
	State           string         `db:"state"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractData    *datatype.JSON `db:"contract_data"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
}

type SearchValues struct {
	DID             *string
	ContractVersion *int
	State           string
	Name            *string
	Description     *string
	ContractData    *string
}

type ContractRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data Contract) (*time.Time, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
	ExpireOutdatedContracts(ctx context.Context, tx *sqlx.Tx) (int64, error)
}
