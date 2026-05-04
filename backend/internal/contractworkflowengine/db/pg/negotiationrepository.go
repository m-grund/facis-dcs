package pg

import (
	"context"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostgresNegotiationRepo struct {
}

func (r PostgresNegotiationRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.NegotiationCreateData, negotiators []string) (*time.Time, error) {
	statement := `
        INSERT INTO contract_negotiations (
            did, contract_version, change_request, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING id, created_at
    `

	var result struct {
		ID        string    `db:"id"`
		CreatedAt time.Time `db:"created_at"`
	}
	err := tx.GetContext(ctx, &result, statement,
		data.DID, data.ContractVersion, data.ChangeRequest, data.CreatedBy)
	if err != nil {
		return nil, err
	}

	for _, negotiator := range negotiators {
		decisionStatement := `
            INSERT INTO contract_negotiation_decisions (
                negotiation_id, negotiator
            ) VALUES ($1, $2)
        `
		_, err = tx.ExecContext(ctx, decisionStatement, result.ID, negotiator)
		if err != nil {
			return nil, err
		}
	}

	return &result.CreatedAt, nil
}

func (r PostgresNegotiationRepo) Accept(ctx context.Context, tx *sqlx.Tx, id string, acceptedBy string) error {
	statement := `
        UPDATE contract_negotiation_decisions cnd
        SET decision = 'ACCEPTED'
        FROM contract_negotiations cn
        WHERE
            cn.id = cnd.negotiation_id AND
            cn.id = $1 AND
            decision IS NULL AND
            negotiator = $2
    `
	result, err := tx.ExecContext(ctx, statement, id, acceptedBy)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("no negotiations accepted")
	}

	return nil
}

func (r PostgresNegotiationRepo) Reject(ctx context.Context, tx *sqlx.Tx, id string, rejectedBy string, rejectionReason *string) error {
	statement := `
        UPDATE contract_negotiation_decisions cnd
        SET
            decision = CASE
                WHEN negotiator = $2 THEN 'REJECTED'::contract_negotiation_decision
        		ELSE 'CLOSED'::contract_negotiation_decision
            END,
            rejection_reason = CASE
                WHEN negotiator = $2 THEN $3
            END
        FROM contract_negotiations cn
        WHERE cn.id = cnd.negotiation_id 
          AND cn.id = $1
          AND cnd.decision IS NULL
    `
	result, err := tx.ExecContext(ctx, statement, id, rejectedBy, rejectionReason)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("no negotiations rejected")
	}

	return nil
}

func (r PostgresNegotiationRepo) ReadAllByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.NegotiationData, error) {
	query := `
        SELECT cn.id, did, contract_version, change_request, negotiator, decision,
               rejection_reason, created_by, created_at
        FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
    `
	var negotiations []db.NegotiationData
	err := tx.SelectContext(ctx, &negotiations, query, did)
	if err != nil {
		return nil, err
	}
	return negotiations, nil
}

func (r PostgresNegotiationRepo) ReadAllAcceptedByContractDIDAndVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) ([]db.NegotiationChangeData, error) {
	query := `
        SELECT cn.id, change_request
		FROM contract_negotiations cn
		JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
		WHERE cn.did = $1
		  AND (cn.contract_version = $2 OR ($2 IS NULL AND cn.contract_version IS NULL))
		GROUP BY cn.id, cn.change_request
		HAVING COUNT(*) = COUNT(CASE WHEN cnd.decision = 'ACCEPTED' THEN 1 END)
    `
	var negotiations []db.NegotiationChangeData
	err := tx.SelectContext(ctx, &negotiations, query, did, contractVersion)
	if err != nil {
		return nil, err
	}
	return negotiations, nil
}

func (r PostgresNegotiationRepo) HasOpenNegotiationDecisions(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) (bool, error) {
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
              AND (contract_version = $2 OR ($2 IS NULL AND contract_version IS NULL))
              AND cnd.decision IS NULL
        )
    `
	var exists bool
	err := tx.GetContext(ctx, &exists, query, did, contractVersion)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r PostgresNegotiationRepo) HasNegotiationForContractVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) (bool, error) {

	query := `
        SELECT EXISTS (
            SELECT 1
            FROM contract_negotiations cn
            JOIN contract_negotiation_decisions cnd ON cnd.negotiation_id = cn.id
            WHERE cn.did = $1
              AND (contract_version = $2 OR ($2 IS NULL AND contract_version IS NULL))
        )
    `
	var exists bool
	err := tx.GetContext(ctx, &exists, query, did, contractVersion)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r PostgresNegotiationRepo) Delete(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM contract_review_task
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}
