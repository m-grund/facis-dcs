package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type Contract struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractData    *datatype.JSON `db:"contract_data"`
}

type ContractMetadata struct {
	DID             string    `db:"did"`
	ContractVersion int       `db:"contract_version"`
	State           string    `db:"state"`
	CreatedBy       string    `db:"created_by"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
	Name            *string   `db:"name"`
	Description     *string   `db:"description"`
}

type ContractProcessData struct {
	DID             string    `db:"did"`
	ContractVersion int       `db:"contract_version"`
	State           string    `db:"state"`
	CreatedBy       string    `db:"created_by"`
	UpdatedAt       time.Time `db:"updated_at"`
}

type ContractUpdateData struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractData    *datatype.JSON `db:"contract_data"`
}

type SearchValues struct {
	DID             *string
	ContractVersion *int
	Name            *string
	Description     *string
	ContractData    *string
}

type ContractRepo interface {
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
}
