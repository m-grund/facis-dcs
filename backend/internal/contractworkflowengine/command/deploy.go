package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

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

	contractDataBytes := []byte(`{}`)
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		contractDataBytes = []byte(*data.ContractData)
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
		"dcs:contractDocument": json.RawMessage(contractDataBytes),
		"odrl:policy": map[string]any{
			"@id":   "urn:uuid:deployment-policy-" + correlationID,
			"@type": "odrl:Set",
			"uid":   cmd.DID,
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

func hashDeploymentPayload(payload map[string]any) (string, error) {
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
