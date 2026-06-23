package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresContractRepo struct {
}

func (r *PostgresContractRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.Contract) (*time.Time, error) {
	statement := `
        INSERT INTO contracts (
            did, origin, created_by, state, name,
            description, contract_data, template_did, template_version
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.Origin, data.CreatedBy, data.State, data.Name,
		data.Description, data.ContractData, data.TemplateDID, data.TemplateVersion)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresContractRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO contract_history 
            (did, origin, state, name, description, created_by, created_at, updated_at, 
             contract_version, contract_data, start_date, exp_date, exp_policy, 
             exp_notice_period, responsible, template_did, template_version)
        SELECT 
            did, origin, state, name, description, created_by, created_at, updated_at, 
            contract_version, contract_data, start_date, exp_date, exp_policy, 
            exp_notice_period, responsible, template_did, template_version
        FROM contracts_effective 
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractRepo) ReadLastHistoryEntryByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractHistory, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contract_history
        WHERE did = $1
        ORDER BY contract_version DESC NULLS LAST
    	LIMIT 1
    `
	var ct db.ContractHistory
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractHistory, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contract_history
        WHERE did = $1
    `
	var ct []db.ContractHistory
	err := tx.SelectContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.ContractHistory{}, fmt.Errorf("contract with DID %s not found", did)
		}
		return []db.ContractHistory{}, err
	}
	return ct, nil
}

func (r *PostgresContractRepo) ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.Contract, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contracts_effective
        WHERE did = $1
    `
	var ct db.Contract
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
		SELECT
			cem.did, cem.origin, cem.state, cem.name, cem.description, cem.created_by, cem.created_at, cem.updated_at,
			cem.contract_version, cem.start_date, cem.exp_date, cem.exp_policy, cem.exp_notice_period, cem.responsible,
			cem.template_did, cem.template_version,
			cem.state IN ('DRAFT', 'REJECTED', 'SUBMITTED', 'NEGOTIATION', 'REVIEWED', 'APPROVED')
			AND COALESCE(latest.version > cem.template_version, FALSE) AS outdated,
			latest.did AS latest_template_did,
			COALESCE(tpl.state = 'DEPRECATED', FALSE) AS template_is_deprecated
		FROM contracts_effective_metadata cem
		LEFT JOIN contract_templates tpl
			ON tpl.did = cem.template_did
		LEFT JOIN LATERAL (
			SELECT ct.did, ct.version
			FROM contract_templates ct
			WHERE ct.base_template = tpl.base_template
			  AND ct.state IN ('REGISTERED', 'PUBLISHED')
			ORDER BY ct.version DESC
			LIMIT 1
		) latest ON true
	`
	var params []any
	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		query += ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, origin, state, name, description, created_by, created_at, updated_at, contract_version, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contracts_effective_metadata
    `

	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		n := len(params) + 1
		query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, origin,  state, updated_at, created_by, contract_version, start_date, exp_date, exp_policy, exp_notice_period
        FROM contracts_effective_process_data WHERE did = $1
    `
	var processData db.ContractProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractRepo) ReadExpiredContacts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `
    SELECT did, origin, state, name, description, created_by, created_at, updated_at, contract_version, start_date,
           exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
    FROM contracts
    WHERE exp_date IS NOT NULL
    AND exp_date < NOW()
    AND state NOT IN ('DRAFT', 'TERMINATED', 'REJECTED', 'EXPIRED')
`
	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return nil, err
	}

	return cts, nil
}

func (r *PostgresContractRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contracts SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

func (r *PostgresContractRepo) ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractPDFState, error) {
	var state db.ContractPDFState
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_renderer_version,''), COALESCE(pdf_c2pa_state,'') FROM contracts WHERE did=$1`, did,
	).Scan(&state.IPFSCID, &state.RendererVersion, &state.C2PAState)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *PostgresContractRepo) UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data db.ContractPDFState) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE contracts SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3 WHERE did=$4`,
		data.IPFSCID, data.RendererVersion, data.C2PAState, did,
	)
	return err
}

func (r *PostgresContractRepo) Update(ctx context.Context, tx *sqlx.Tx, data db.ContractUpdateData) error {
	query, params, err := createQuery(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, *query, params...)
	return err
}

func createSearchConditions(values db.SearchValues) (*string, []interface{}, error) {
	conditions := ""
	var params []interface{}
	paramIndex := 1

	if len(values.DID) > 0 {
		conditions += ` did = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.DID)
		paramIndex++
	}
	if values.ContractVersion > 0 {
		conditions += ` contract_version = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.ContractVersion)
		paramIndex++
	}
	if len(values.State) > 0 {
		conditions += ` state = $` + strconv.Itoa(paramIndex) + ` AND`
		state := strings.ToUpper(values.State)
		params = append(params, state)
		paramIndex++
	}
	if len(values.Name) > 0 {
		conditions += ` name ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+values.Name+"%")
		paramIndex++
	}
	if len(values.Description) > 0 {
		conditions += ` description ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+values.Description+"%")
		paramIndex++
	}
	if len(values.ContractData) > 0 {
		conditions += ` search_vector @@ plainto_tsquery('english', $` + strconv.Itoa(paramIndex) + `) AND`
		params = append(params, values.ContractData)
	}
	l := len(" AND")
	if len(conditions) > l {
		conditions = conditions[:len(conditions)-l]
	}

	return &conditions, params, nil
}

func createQuery(data db.ContractUpdateData) (*string, []interface{}, error) {
	queryBase := `UPDATE contracts SET `
	var columns []string
	var params []interface{}

	addParam := func(columnName string, value interface{}) {
		columns = append(columns, fmt.Sprintf("%s = $%d", columnName, len(params)+1))
		params = append(params, value)
	}

	if data.ContractVersion > 0 {
		addParam("contract_version", data.ContractVersion)
	}
	if len(data.State) > 0 {
		addParam("state", data.State)
	}
	if data.Name != nil {
		addParam("name", data.Name)
	}
	if data.Description != nil {
		addParam("description", data.Description)
	}
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		addParam("contract_data", data.ContractData)
	}
	if data.StartDate != nil {
		addParam("start_date", data.StartDate)
	}
	if data.ExpDate != nil {
		addParam("exp_date", data.ExpDate)
	}
	if data.ExpPolicy != nil {
		addParam("exp_policy", data.ExpPolicy)
	}
	if data.ExpNoticePeriod != nil {
		addParam("exp_notice_period", data.ExpNoticePeriod)
	}
	if data.Responsible != nil {
		addParam("responsible", data.Responsible)
	}
	if len(columns) == 0 {
		return nil, nil, errors.New("no fields to update")
	}

	fullQuery := queryBase + strings.Join(columns, ", ")
	nextIdx := len(params) + 1
	fullQuery += fmt.Sprintf(" WHERE did = $%d;",
		nextIdx)
	params = append(params, data.DID)

	return &fullQuery, params, nil
}
