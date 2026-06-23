package db

import (
	"context"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type ContractTemplateRepo interface {
	ReadFrameContractTemplateDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*FrameContractTemplateData, error)
}

type FrameContractTemplateData struct {
	TemplateData *datatype.JSON
	Version      int
}
