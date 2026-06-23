package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresContractTemplateRepo struct {
}

func (r *PostgresContractTemplateRepo) ReadFrameContractTemplateDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.FrameContractTemplateData, error) {
	statement := `
        SELECT template_data, version
        FROM contract_templates
        WHERE
            did = $1
            AND template_type = 'FRAME_CONTRACT'
            AND (state = 'APPROVED' OR state = 'PUBLISHED')
        LIMIT 1
	`
	var templateData struct {
		TemplateData datatype.JSON `db:"template_data"`
		Version      int           `db:"version"`
	}
	err := tx.GetContext(ctx, &templateData, statement, did)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("could not find frame contract template with DID %q", did)
	case err != nil:
		return nil, err
	}
	return &db.FrameContractTemplateData{
		TemplateData: &templateData.TemplateData,
		Version:      templateData.Version,
	}, nil
}
