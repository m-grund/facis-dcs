package db

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"time"

	"github.com/jmoiron/sqlx"
)

type ContractTemplate struct {
	DID            string         `db:"did"`
	DocumentNumber *string        `db:"document_number"`
	Version        *int           `db:"version"`
	State          string         `db:"state"`
	TemplateType   string         `db:"template_type"`
	Name           *string        `db:"name"`
	Description    *string        `db:"description"`
	CreatedBy      string         `db:"created_by"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	TemplateData   *datatype.JSON `db:"template_data"`
}

type ContractTemplateMetadata struct {
	DID            string    `db:"did"`
	DocumentNumber *string   `db:"document_number"`
	Version        *int      `db:"version"`
	State          string    `db:"state"`
	TemplateType   string    `db:"template_type"`
	Name           *string   `db:"name"`
	Description    *string   `db:"description"`
	CreatedBy      string    `db:"created_by"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type ContractTemplateProcessData struct {
	DID            string    `db:"did"`
	DocumentNumber *string   `db:"document_number"`
	Version        *int      `db:"version"`
	State          string    `db:"state"`
	CreatedBy      string    `db:"created_by"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type ContractTemplateUpdateData struct {
	DID            string         `db:"did"`
	DocumentNumber *string        `db:"document_number"`
	Version        *int           `db:"version"`
	State          string         `db:"state"`
	TemplateType   string         `db:"template_type"`
	Name           *string        `db:"name"`
	Description    *string        `db:"description"`
	TemplateData   *datatype.JSON `db:"template_data"`
}

type SearchValues struct {
	DID            *string
	DocumentNumber *string
	Version        *int
	State          string
	TemplateType   string
	Name           *string
	Description    *string
	TemplateData   *string
}

type ContractTemplateRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data ContractTemplate) (*time.Time, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplate, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractTemplateMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractTemplateMetadata, error)
	ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplateProcessData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractTemplateUpdateData) error
}
