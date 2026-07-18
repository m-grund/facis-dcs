package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/signingmanagement/db"
)

// PostgresCeremonyRepo persists signing ceremonies in Postgres.
type PostgresCeremonyRepo struct{}

func (r *PostgresCeremonyRepo) CreateCeremony(ctx context.Context, tx *sqlx.Tx, c db.SignatureCeremony) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO signature_ceremonies
			(id, contract_did, field_name, requested_by, status, wallet_uri, nonce, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.ContractDID, c.FieldName, c.RequestedBy, c.Status, c.WalletURI, c.Nonce, c.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create signature ceremony: %w", err)
	}
	return nil
}

func (r *PostgresCeremonyRepo) GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at,
		       prepared_pdf, prepared_pdf_sha256, request_nonce, request_expires_at, credential_type,
		       published_by, published_holder_did, published_roles, consumed_at
		  FROM signature_ceremonies
		 WHERE id = $1`, id,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get signature ceremony %s: %w", id, err)
	}
	return &c, nil
}

func (r *PostgresCeremonyRepo) StorePreparedRequest(ctx context.Context, tx *sqlx.Tx, req db.PreparedRequest) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET prepared_pdf = $2, prepared_pdf_sha256 = $3, request_nonce = $4,
		       request_expires_at = $5, credential_type = $6, published_by = $7,
		       published_holder_did = $8, published_roles = $9, consumed_at = NULL
		 WHERE id = $1 AND status = $10`,
		req.CeremonyID, req.PreparedPDF, req.PreparedPDFSHA256, req.RequestNonce,
		req.RequestExpiresAt, req.CredentialType, req.PublishedBy,
		req.HolderDID, req.Roles, db.CeremonyVerified,
	)
	if err != nil {
		return fmt.Errorf("store prepared request for ceremony %s: %w", req.CeremonyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store prepared request for ceremony %s rows: %w", req.CeremonyID, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s is not in %q state", req.CeremonyID, db.CeremonyVerified)
	}
	return nil
}

func (r *PostgresCeremonyRepo) MarkCeremonyConsumed(ctx context.Context, tx *sqlx.Tx, id string) error {
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET consumed_at = $2
		 WHERE id = $1 AND consumed_at IS NULL`,
		id, now,
	)
	if err != nil {
		return fmt.Errorf("mark ceremony %s consumed: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ceremony %s consumed rows: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s signing request is already consumed", id)
	}
	return nil
}

func (r *PostgresCeremonyRepo) MarkCeremonyVerified(ctx context.Context, tx *sqlx.Tx, id, signerDID, vpToken string, pidClaims []byte, kbSdHash string) error {
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET status = $2, signer_did = $3, vp_token = $4, pid_claims = $5, kb_sd_hash = $6, verified_at = $7
		 WHERE id = $1 AND status = $8`,
		id, db.CeremonyVerified, signerDID, vpToken, pidClaims, kbSdHash, now, db.CeremonyPending,
	)
	if err != nil {
		return fmt.Errorf("mark ceremony %s verified: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ceremony %s verified rows: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s is not in %q state", id, db.CeremonyPending)
	}
	return nil
}

func (r *PostgresCeremonyRepo) FindVerifiedCeremonyByField(ctx context.Context, tx *sqlx.Tx, contractDID, fieldName string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at
		  FROM signature_ceremonies
		 WHERE contract_did = $1 AND field_name = $2 AND status = $3
		 ORDER BY verified_at DESC NULLS LAST
		 LIMIT 1`,
		contractDID, fieldName, db.CeremonyVerified,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find verified ceremony for %s field %q: %w", contractDID, fieldName, err)
	}
	return &c, nil
}

func (r *PostgresCeremonyRepo) FindVerifiedCeremony(ctx context.Context, tx *sqlx.Tx, contractDID, signerDID string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at
		  FROM signature_ceremonies
		 WHERE contract_did = $1 AND signer_did = $2 AND status = $3
		 ORDER BY verified_at DESC NULLS LAST
		 LIMIT 1`,
		contractDID, signerDID, db.CeremonyVerified,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find verified ceremony for %s/%s: %w", contractDID, signerDID, err)
	}
	return &c, nil
}
