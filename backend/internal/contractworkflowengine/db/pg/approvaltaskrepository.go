package pg

import (
	"context"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostgresApprovalTaskRepo struct {
}

func (r *PostgresApprovalTaskRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.ApprovalTaskData) (*time.Time, error) {
	statement := `
        INSERT INTO contract_approval_task (
            did, state, approver, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID,
		data.State, data.Approver, data.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresApprovalTaskRepo) ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        UPDATE contract_approval_task SET state = 'OPEN'
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresApprovalTaskRepo) ReadAll(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ApprovalTaskData, error) {
	query := `
        SELECT id, did, state, approver,
               created_by, created_at
        FROM contract_approval_task WHERE did = $1
    `
	var approvalTasks []db.ApprovalTaskData
	err := tx.SelectContext(ctx, &approvalTasks, query, did)
	if err != nil {
		return nil, err
	}
	return approvalTasks, nil
}

func (r *PostgresApprovalTaskRepo) ReadAllByApprover(ctx context.Context, tx *sqlx.Tx, approver string) ([]db.ApprovalTaskData, error) {
	query := `
        SELECT id, did, state, approver,
               created_by, created_at
        FROM contract_approval_task WHERE approver = $1
    `
	var approvalTasks []db.ApprovalTaskData
	err := tx.SelectContext(ctx, &approvalTasks, query, approver)
	if err != nil {
		return nil, err
	}
	return approvalTasks, nil
}

func (r *PostgresApprovalTaskRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) error {
	statement := `
        UPDATE contract_approval_task SET state = $3
        WHERE did = $1 AND approver = $2
    `
	result, err := tx.ExecContext(ctx, statement, did, approver, state)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("user has no review task for this contract")
	}
	return nil
}

func (r *PostgresApprovalTaskRepo) AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error) {
	placeholders := make([]string, len(states))
	args := []interface{}{did}

	for i, s := range states {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*) 
        FROM contract_approval_task
        WHERE did = $1 AND state IN (%s)
    `, strings.Join(placeholders, ", "))

	var count int
	err := tx.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresApprovalTaskRepo) IsValidApprover(ctx context.Context, tx *sqlx.Tx, did string, approver string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_approval_task
        WHERE did = $1 AND approver = $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, approver)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresApprovalTaskRepo) TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_approval_task
        WHERE did = $1 AND approver = $2 AND state = $3
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, approver, state)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresApprovalTaskRepo) TaskExists(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_approval_task
        WHERE did = $1
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresApprovalTaskRepo) Delete(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM contract_approval_task
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}
