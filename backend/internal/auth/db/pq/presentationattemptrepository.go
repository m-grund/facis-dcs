package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	authdb "digital-contracting-service/internal/auth/db"
)

type PostgresPresentationAttemptRepo struct {
	db *sqlx.DB
}

func NewPostgresPresentationAttemptRepo(db *sqlx.DB) *PostgresPresentationAttemptRepo {
	return &PostgresPresentationAttemptRepo{db: db}
}

func (r *PostgresPresentationAttemptRepo) CreatePending(ctx context.Context, attempt authdb.PresentationAttempt) error {
	const stmt = `
		INSERT INTO oid4vp_presentation_attempts (
			presentation_state, status, nonce, expires_at, hydra_login_challenge, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`
	_, err := r.db.ExecContext(ctx, stmt,
		attempt.PresentationState,
		authdb.PresentationPending,
		attempt.Nonce,
		attempt.ExpiresAt,
		attempt.HydraLoginChallenge,
	)
	return err
}

func (r *PostgresPresentationAttemptRepo) FindByPresentationState(ctx context.Context, presentationState string) (*authdb.PresentationAttempt, error) {
	const query = `
		SELECT presentation_state, status, nonce, expires_at, verified_claims, hydra_login_challenge,
			redirect_uri, error_message, subject_did, organization_id, roles, created_at, updated_at
		FROM oid4vp_presentation_attempts
		WHERE presentation_state = $1
	`
	var attempt authdb.PresentationAttempt
	err := r.db.GetContext(ctx, &attempt, query, presentationState)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &attempt, nil
}

func (r *PostgresPresentationAttemptRepo) SetHydraLoginChallenge(ctx context.Context, presentationState, challenge string) error {
	const stmt = `
		UPDATE oid4vp_presentation_attempts
		SET hydra_login_challenge = $2, updated_at = NOW()
		WHERE presentation_state = $1 AND status = $3
	`
	res, err := r.db.ExecContext(ctx, stmt, presentationState, challenge, authdb.PresentationPending)
	if err != nil {
		return err
	}
	return pendingUpdateResult(res, presentationState)
}

func (r *PostgresPresentationAttemptRepo) MarkComplete(
	ctx context.Context,
	presentationState string,
	claims json.RawMessage,
	subjectDID, organizationID string,
	roles json.RawMessage,
	redirectURI string,
) error {
	const stmt = `
		UPDATE oid4vp_presentation_attempts
		SET status = $2, verified_claims = $3, subject_did = $4, organization_id = $5,
			roles = $6, redirect_uri = $7, updated_at = NOW()
		WHERE presentation_state = $1 AND status = $8
	`
	res, err := r.db.ExecContext(ctx, stmt,
		presentationState,
		authdb.PresentationComplete,
		claims,
		subjectDID,
		organizationID,
		roles,
		redirectURI,
		authdb.PresentationPending,
	)
	if err != nil {
		return err
	}
	return pendingUpdateResult(res, presentationState)
}

func (r *PostgresPresentationAttemptRepo) MarkFailed(ctx context.Context, presentationState string, errorMessage string) error {
	const stmt = `
		UPDATE oid4vp_presentation_attempts
		SET status = $2, error_message = $3, updated_at = NOW()
		WHERE presentation_state = $1 AND status = $4
	`
	res, err := r.db.ExecContext(ctx, stmt, presentationState, authdb.PresentationFailed, errorMessage, authdb.PresentationPending)
	if err != nil {
		return err
	}
	return pendingUpdateResult(res, presentationState)
}

func (r *PostgresPresentationAttemptRepo) RenewPending(ctx context.Context, presentationState string, expiresAt time.Time) error {
	const stmt = `
		UPDATE oid4vp_presentation_attempts
		SET status = $2, expires_at = $3, updated_at = NOW()
		WHERE presentation_state = $1 AND status IN ($4, $5)
	`
	res, err := r.db.ExecContext(ctx, stmt,
		presentationState,
		authdb.PresentationPending,
		expiresAt,
		authdb.PresentationPending,
		authdb.PresentationExpired,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", authdb.ErrPresentationNotPending, presentationState)
	}
	return nil
}

func (r *PostgresPresentationAttemptRepo) MarkExpired(ctx context.Context, presentationState string) error {
	const stmt = `
		UPDATE oid4vp_presentation_attempts
		SET status = $2, updated_at = NOW()
		WHERE presentation_state = $1 AND status = $3
	`
	res, err := r.db.ExecContext(ctx, stmt, presentationState, authdb.PresentationExpired, authdb.PresentationPending)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return authdb.ErrPresentationNotPending
	}
	return nil
}

func pendingUpdateResult(res sql.Result, presentationState string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", authdb.ErrPresentationNotPending, presentationState)
	}
	return nil
}
