package pg

import (
	"context"
	"database/sql"
	"digital-contracting-service/internal/signingmanagement/db"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
)

type PostgresContractRepo struct {
	Ctx context.Context
}

func (r *PostgresContractRepo) ReadDataByID(tx *sqlx.Tx, did string) (*db.Contract, error) {
	query := `
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version
        FROM contracts
        WHERE did = $1
         AND state = 'APPROVED'
    `
	var ct db.Contract
	err := tx.GetContext(r.Ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ReadAllMetaData(tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version
        FROM contracts
        WHERE state = 'APPROVED'
    `
	var cts []db.ContractMetadata
	err := tx.SelectContext(r.Ctx, &cts, query)
	if err != nil {
		return []db.ContractMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadAllMetaDataByFilter(tx *sqlx.Tx, values db.SearchValues) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version
        FROM contracts
        WHERE state = 'APPROVED'
    `
	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	var cts []db.ContractMetadata
	err = tx.SelectContext(r.Ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadProcessData(tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, state, updated_at, created_by, contract_version
        FROM contracts
        WHERE did = $1
         AND  state = 'APPROVED'
    `
	var processData db.ContractProcessData
	err := tx.GetContext(r.Ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractRepo) UpdateState(tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contracts SET state = $2
        WHERE did = $1
         AND  state = 'APPROVED'
    `
	_, err := tx.ExecContext(r.Ctx, statement, did, state)
	return err
}

func (r *PostgresContractRepo) Update(tx *sqlx.Tx, data db.ContractUpdateData) error {
	query, params, err := createQuery(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(r.Ctx, *query, params...)
	return err
}

func createSearchConditions(values db.SearchValues) (*string, []interface{}, error) {
	conditions := ""
	var params []interface{}
	paramIndex := 1

	if values.DID != nil {
		conditions += ` did = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, *values.DID)
		paramIndex++
	}
	if values.ContractVersion != nil {
		conditions += ` contract_version = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, *values.ContractVersion)
		paramIndex++
	}
	if values.Name != nil {
		conditions += ` name ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+*values.Name+"%")
		paramIndex++
	}
	if values.Description != nil {
		conditions += ` description ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+*values.Description+"%")
		paramIndex++
	}
	if values.Filter != nil {
		conditions += ` search_vector @@ plainto_tsquery('english', $` + strconv.Itoa(paramIndex) + `) AND`
		params = append(params, *values.Filter)
		paramIndex++
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

	if data.Name != nil {
		addParam("name", data.Name)
	}
	if data.Description != nil {
		addParam("description", data.Description)
	}
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		addParam("contract_data", data.ContractData)
	}
	if data.ContractVersion != nil {
		addParam("contract_version", data.ContractVersion)
	}
	if len(columns) == 0 {
		return nil, nil, errors.New("no fields to update")
	}

	fullQuery := queryBase + strings.Join(columns, ", ")
	nextIdx := len(params) + 1
	fullQuery += fmt.Sprintf(" WHERE did = $%d AND state = 'APPROVED';",
		nextIdx)
	params = append(params, data.DID)

	return &fullQuery, params, nil
}
