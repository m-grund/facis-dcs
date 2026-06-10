package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	event2 "digital-contracting-service/internal/signingmanagement/event"

	"github.com/jmoiron/sqlx"
)

// ApplyCmd carries the inputs for applying a digital signature.
type ApplyCmd struct {
	DID            string
	CredentialType string
	AppliedBy      string
	HolderDID      string
	UserRoles      userrole.UserRoles
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
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Validates APPROVED state; errors if not found.
	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}

	if data.ContractData == nil {
		return fmt.Errorf("could not read data from contract %s: %w", cmd.DID, err)
	}

	// Compute SHA-256 of the JSON-LD as the canonical signing payload.
	sum := sha256.Sum256(*data.ContractData)
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
		cmd.DID, cmd.HolderDID, cmd.CredentialType, sigBytes, status, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert contract_signatures: %w", err)
	}

	evt := event2.ApplyEvent{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		CredentialType:  cmd.CredentialType,
		AppliedBy:       cmd.AppliedBy,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
