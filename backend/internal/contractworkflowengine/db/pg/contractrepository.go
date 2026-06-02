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
            did, created_by, state, name,
            description, contract_data
        ) VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.CreatedBy, data.State, data.Name,
		data.Description, data.ContractData)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresContractRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO contract_history 
            (did, state, name, description, created_by, created_at, updated_at, 
             contract_version, contract_data, start_date, exp_date, exp_policy, 
             exp_notice_period, responsible_persons)
        SELECT 
            did, state, name, description, created_by, created_at, updated_at, 
            contract_version, contract_data, start_date, exp_date, exp_policy, 
            exp_notice_period, responsible_persons
        FROM contracts_effective 
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractRepo) ReadLastHistoryEntryByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractHistory, error) {
	query := `
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
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
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
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
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
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
	var cts []db.ContractMetadata

	query := `
    SELECT did, state, name, description, created_by, created_at, updated_at,
           contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
    FROM contracts_effective_metadata
    ORDER BY created_at DESC
`

	if pagination.PageSize > 0 {
		offset := (pagination.StartIndex - 1) * pagination.PageSize
		query += ` LIMIT :page_size OFFSET :offset`

		err := tx.SelectContext(ctx, &cts, query,
			sql.Named("page_size", pagination.PageSize),
			sql.Named("offset", offset),
		)
		if err != nil {
			return []db.ContractMetadata{}, err
		}
	} else {
		err := tx.SelectContext(ctx, &cts, query)
		if err != nil {
			return []db.ContractMetadata{}, err
		}
	}

	return cts, nil
}

func (r *PostgresContractRepo) ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
        FROM contracts_effective_metadata
    `
	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	var cts []db.ContractMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, state, updated_at, created_by, contract_version, start_date, exp_date, exp_policy, exp_notice_period
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
    SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible_persons
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
	if data.ResponsiblePersons != nil {
		addParam("responsible_persons", data.ResponsiblePersons)
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
