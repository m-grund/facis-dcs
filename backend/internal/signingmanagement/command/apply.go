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
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
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
}

// Applier handles the ApplyCmd command.
type Applier struct {
	DB       *sqlx.DB
	CRepo    db.ContractRepo
	DSClient dss.Client
}

// Handle applies a digital signature to a contract (DCS-FR-SM-16, DCS-IR-SI-10).
// The APPROVED -> SIGNED gate is enforced by contractstate.ValidateTransition
// against the single-source-of-truth transition table (see below), not by any
// hardcoded SQL state literal.
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

	// Reads the contract (restricted to APPROVED/SIGNED by the repo query);
	// errors if not found. The SIGN transition itself is gated below.
	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}

	if data.ContractData == nil {
		return fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}

	// The transition table (contractstate.Transitions) is the single source
	// of truth for the APPROVED -> SIGNED gate — no hardcoded SQL state
	// literal decides this anymore (see command package/ADR-3 note).
	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventSign); err != nil {
		return err
	}

	if err := validation.ValidateContractPolicySatisfaction(
		*data.ContractData,
		validation.ContractContentAuditMetadata{
			ContractDID:     cmd.DID,
			ContractVersion: fmt.Sprint(data.ContractVersion),
			AuditedBy:       cmd.AppliedBy,
			HolderDID:       cmd.HolderDID,
		},
	); err != nil {
		return err
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

	sigBytes, err := h.DSClient.Sign(ctx, payload, cmd.CredentialType)
	if err != nil {
		return fmt.Errorf("DSS sign: %w", err)
	}

	status := "SIGNED"
	if len(sigBytes) == 0 {
		status = "PENDING"
	}

	signature := db.ContractSignature{
		ContractDID:    cmd.DID,
		Status:         status,
		SignatureBytes: sigBytes,
		SignerDID:      cmd.AppliedBy,
		CredentialType: cmd.CredentialType,
	}
	err = h.CRepo.CreateSignature(ctx, tx, signature)
	if err != nil {
		return fmt.Errorf("could not create signature: %w", err)
	}

	if status == "SIGNED" {
		if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Signed.String()); err != nil {
			return fmt.Errorf("could not update contract state: %w", err)
		}
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
