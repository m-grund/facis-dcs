package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// ErrSigningIncomplete rejects deployment of a multi-signer contract whose
// declared signature fields are not all signed yet (DCS-FR-SM-07/-17).
var ErrSigningIncomplete = errors.New("signing workflow incomplete")

// DeployCmd carries the inputs for deploying a SIGNED contract to the
// configured Contract Target System (UC-05-01).
type DeployCmd struct {
	DID         string
	UpdatedAt   time.Time
	RequestedBy string
}

// DeployResult is what both the manual /contract/deploy endpoint and the
// automatic post-signing subscriber receive back from Deployer.Handle.
type DeployResult struct {
	DID             string
	ContractVersion int
	ContentHash     string
	Timestamp       time.Time
	CorrelationID   string
	Payload         map[string]any
}

// Deployer handles DeployCmd: it gates on the contract being SIGNED (the
// same EventDeploy edge the ack-driven SIGNED -> ACTIVE transition uses,
// declared once in contractstate.Transitions), builds the machine-readable
// deployment payload (JSON-LD contract document, DID, version, content hash,
// timestamp, and an enclosing odrl:Set describing the deployment's own
// authorization), records the dispatch, and best-effort forwards it to the
// configured Contract Target System.
type Deployer struct {
	DB             *sqlx.DB
	CRepo          db.ContractRepo
	DeploymentRepo db.DeploymentRepo
	Target         ContractTargetClient
}

func (h *Deployer) Handle(ctx context.Context, cmd DeployCmd) (*DeployResult, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventDeploy); err != nil {
		return nil, err
	}

	// Multi-signer gate (DCS-FR-SM-07/-17, DCS-NFR-BR-03): a contract that
	// declares signature fields may only deploy once EVERY declared field is
	// signed. The auto-deploy subscriber fires after each signature, so a
	// partially signed contract hits this gate until the last signatory
	// signs.
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		required := validation.RequiredSignatureFields([]byte(*data.ContractData))
		if len(required) > 0 {
			signedFields, err := h.CRepo.ReadSignedSignatureFieldNames(ctx, tx, cmd.DID)
			if err != nil {
				return nil, fmt.Errorf("could not read signed signature fields: %w", err)
			}
			signed := make(map[string]bool, len(signedFields))
			for _, f := range signedFields {
				signed[f] = true
			}
			var missing []string
			for _, f := range required {
				if !signed[f] {
					missing = append(missing, f)
				}
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("%w: unsigned signature fields: %s", ErrSigningIncomplete, strings.Join(missing, ", "))
			}
		}
	}

	contractDataBytes := []byte(`{}`)
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		contractDataBytes = []byte(*data.ContractData)
	}
	// Decode the contract document instead of embedding the raw jsonb bytes:
	// the content hash must be reproducible by the RECEIVING target system
	// from the parsed JSON (canonical form = recursively key-sorted, compact,
	// no HTML escaping — see hashDeploymentPayload), and Postgres jsonb's
	// length-then-bytewise key order would otherwise leak into the hash.
	var contractDocument map[string]any
	if err := json.Unmarshal(contractDataBytes, &contractDocument); err != nil {
		return nil, fmt.Errorf("could not decode contract document for %s: %w", cmd.DID, err)
	}

	correlationID := uuid.NewString()
	now := time.Now().UTC()

	payload := map[string]any{
		"@context": map[string]string{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@type":                "dcs:ContractDeployment",
		"dcs:contractDid":      cmd.DID,
		"dcs:contractVersion":  data.ContractVersion,
		"dcs:timestamp":        now.Format(time.RFC3339Nano),
		"dcs:correlationId":    correlationID,
		"dcs:contractDocument": contractDocument,
		"odrl:policy": map[string]any{
			"@id":   "urn:uuid:deployment-policy-" + correlationID,
			"@type": "odrl:Set",
		},
	}

	contentHash, err := hashDeploymentPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("could not hash deployment payload: %w", err)
	}
	payload["dcs:contentHash"] = contentHash

	targetURL := ContractTargetURL()
	var targetURLPtr *string
	if targetURL != "" {
		targetURLPtr = &targetURL
	}

	if err := h.DeploymentRepo.CreateDeployment(ctx, tx, db.ContractDeployment{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		CorrelationID:   correlationID,
		ContentHash:     contentHash,
		TargetURL:       targetURLPtr,
		Status:          "DISPATCHED",
		RequestedBy:     cmd.RequestedBy,
		RequestedAt:     now,
	}); err != nil {
		return nil, fmt.Errorf("could not store deployment record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	// Best-effort forward to the configured target: the target's own
	// callback (POST /contract/deployment/callback) is the authoritative
	// signal of a successful deployment, not this outbound call.
	if h.Target != nil && targetURL != "" {
		if err := h.Target.Deploy(ctx, payload); err != nil {
			log.Printf("contractworkflowengine: could not dispatch deployment %s for contract %s to target: %v", correlationID, cmd.DID, err)
		}
	}

	return &DeployResult{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		ContentHash:     contentHash,
		Timestamp:       now,
		CorrelationID:   correlationID,
		Payload:         payload,
	}, nil
}

// hashDeploymentPayload computes the payload's canonical content hash:
// recursively key-sorted (Go marshals maps sorted), compact, WITHOUT HTML
// escaping — so a receiving target system can reproduce it from the parsed
// JSON with a plain deep-sort + stringify (the shipped ORCE
// contract-target-flow and the BDD harness both do exactly that).
func hashDeploymentPayload(payload map[string]any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "", err
	}
	canonical := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
