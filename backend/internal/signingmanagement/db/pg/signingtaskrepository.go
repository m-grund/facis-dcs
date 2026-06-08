package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/signingmanagement/db"

	"github.com/jmoiron/sqlx"
)

type PostgresSigningTaskRepo struct {
}

func (r *PostgresSigningTaskRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.SigningTaskData) (*time.Time, error) {
	statement := `
        INSERT INTO contract_signing_task (
            did, state, signer, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID,
		data.State, data.Signer, data.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresSigningTaskRepo) ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        UPDATE contract_signing_task SET state = 'OPEN'
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresSigningTaskRepo) ReadAll(ctx context.Context, tx *sqlx.Tx, did string) ([]db.SigningTaskData, error) {
	query := `
        SELECT id, did, state, signer,
               created_by, created_at
        FROM contract_signing_task WHERE did = $1
    `
	var approvalTasks []db.SigningTaskData
	err := tx.SelectContext(ctx, &approvalTasks, query, did)
	if err != nil {
		return nil, err
	}
	return approvalTasks, nil
}

func (r *PostgresSigningTaskRepo) ReadAllBySigner(ctx context.Context, tx *sqlx.Tx, signer string) ([]db.SigningTaskData, error) {
	query := `
        SELECT id, did, state, signer,
               created_by, created_at
        FROM contract_signing_task WHERE signer = $1
    `
	var approvalTasks []db.SigningTaskData
	err := tx.SelectContext(ctx, &approvalTasks, query, signer)
	if err != nil {
		return nil, err
	}
	return approvalTasks, nil
}

func (r *PostgresSigningTaskRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, signer string, state string) error {
	statement := `
        UPDATE contract_signing_task SET state = $3
        WHERE did = $1 AND signer = $2
    `
	result, err := tx.ExecContext(ctx, statement, did, signer, state)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("user has no signing task for this contract")
	}
	return nil
}

func (r *PostgresSigningTaskRepo) AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error) {
	placeholders := make([]string, len(states))
	args := []interface{}{did}

	for i, s := range states {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*) 
        FROM contract_signing_task
        WHERE did = $1 AND state IN (%s)
    `, strings.Join(placeholders, ", "))

	var count int
	err := tx.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresSigningTaskRepo) IsValidSigner(ctx context.Context, tx *sqlx.Tx, did string, signer string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_signing_task
        WHERE did = $1 AND signer = $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, signer)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresSigningTaskRepo) TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, signer string, state string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_signing_task
        WHERE did = $1 AND signer = $2 AND state = $3
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, signer, state)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresSigningTaskRepo) TaskExists(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_signing_task
        WHERE did = $1
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresSigningTaskRepo) Delete(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM contract_signing_task
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}
