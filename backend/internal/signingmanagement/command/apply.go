package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"

	"github.com/jmoiron/sqlx"
)

// ApplyCmd carries the inputs for applying a digital signature.
type ApplyCmd struct {
	DID            string
	SignerDID      string
	CredentialType string
	AppliedBy      string
	DSSClient      dss.Client
}

// Applier handles the ApplyCmd command.
type Applier struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle applies a digital signature to a contract (DCS-FR-SM-16, DCS-IR-SI-10).
// The contract must be in APPROVED state; this is enforced by the repo query.
func (h *Applier) Handle(ctx context.Context, cmd ApplyCmd) error {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	// Validates APPROVED state; errors if not found.
	processData, err := h.CRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}

	// Fetch JSON-LD bytes to form the signing payload.
	var rawJSON []byte
	var contractDataJSON *[]byte
	row := tx.QueryRowContext(ctx, `SELECT contract_data FROM contracts WHERE did = $1`, cmd.DID)
	if err := row.Scan(&contractDataJSON); err == nil && contractDataJSON != nil {
		rawJSON = *contractDataJSON
	}
	// Compute SHA-256 of the JSON-LD as the canonical signing payload.
	sum := sha256.Sum256(rawJSON)
	payload, err := json.Marshal(map[string]string{
		"contract_did": cmd.DID,
		"jsonld_hash":  fmt.Sprintf("%x", sum),
	})
	if err != nil {
		return fmt.Errorf("marshal signing payload: %w", err)
	}

	sigBytes, err := cmd.DSSClient.Sign(ctx, payload, cmd.CredentialType)
	if err != nil {
		return fmt.Errorf("DSS sign: %w", err)
	}

	status := "SIGNED"
	if len(sigBytes) == 0 {
		status = "PENDING"
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO contract_signatures
			(contract_did, signer_did, credential_type, signature_bytes, status, signed_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		cmd.DID, cmd.SignerDID, cmd.CredentialType, sigBytes, status, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert contract_signatures: %w", err)
	}

	evt := signingmanagementevents.ApplyEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		SignerDID:       cmd.SignerDID,
		CredentialType:  cmd.CredentialType,
		AppliedBy:       cmd.AppliedBy,
		OccurredAt:      time.Now().UTC(),
	}
	if err := event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return fmt.Errorf("create apply event: %w", err)
	}

	return tx.Commit()
}
