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
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// ErrDeploymentCallbackUnauthorized is returned when the deployment callback
// shared secret is missing or incorrect (DCS-IR-SI-05).
var ErrDeploymentCallbackUnauthorized = errors.New("incorrect deployment callback shared secret")

// ErrDeploymentNotFound is returned when a callback references a correlation
// ID that was never dispatched by Deployer.
var ErrDeploymentNotFound = errors.New("deployment correlation id not found")

// DeploymentCallbackSecret returns the shared secret that authenticates the
// Contract Target System's deployment callback. It is read from
// DEPLOYMENT_CALLBACK_SECRET, mirroring signingmanagement/command.
// WebhookSecret's EUDIPLO precedent.
func DeploymentCallbackSecret() string {
	if v := strings.TrimSpace(os.Getenv("DEPLOYMENT_CALLBACK_SECRET")); v != "" {
		return v
	}
	return "bdd-deployment-callback-secret"
}

// DeploymentReceiptPayload is the target's execution-evidence receipt
// carried in an acknowledgement callback.
type DeploymentReceiptPayload struct {
	CorrelationID string `json:"correlation_id"`
	PayloadHash   string `json:"payload_hash"`
	ActivatedAt   string `json:"activated_at"`
}

// DeploymentCallbackCmd carries a POST /contract/deployment/callback
// request: either an ack/status update (Status/Receipt set) or a KPI report
// (KPIMetric set), or both.
type DeploymentCallbackCmd struct {
	Secret        string
	DID           string
	CorrelationID string
	Status        string
	Receipt       *DeploymentReceiptPayload
	KPIMetric     string
	KPIValue      string
}

// DeploymentCallbackHandler validates the shared secret, then applies an
// ack (sealing an RFC-3161-timestamped execution-evidence receipt into the
// archive entry and moving the contract SIGNED -> ACTIVE, DCS-FR-SM-10/
// DCS-FR-SM-12) and/or records a reported KPI value, flagging a violation
// when it crosses the contract's own ODRL SLA constraint (DCS-FR-CWE-09).
type DeploymentCallbackHandler struct {
	DB             *sqlx.DB
	CRepo          db.ContractRepo
	DeploymentRepo db.DeploymentRepo
	ArchiveTSA     ArchiveTimestampIssuer
}

func (h *DeploymentCallbackHandler) Handle(ctx context.Context, cmd DeploymentCallbackCmd) error {
	if strings.TrimSpace(cmd.Secret) == "" || cmd.Secret != DeploymentCallbackSecret() {
		return ErrDeploymentCallbackUnauthorized
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	deployment, err := h.DeploymentRepo.FindDeploymentByCorrelationID(ctx, tx, cmd.CorrelationID)
	if err != nil {
		return fmt.Errorf("could not read deployment %s: %w", cmd.CorrelationID, err)
	}
	if deployment == nil {
		return ErrDeploymentNotFound
	}

	if cmd.Receipt != nil || strings.TrimSpace(cmd.Status) != "" {
		if err := h.applyAcknowledgement(ctx, tx, deployment, cmd); err != nil {
			return err
		}
	}

	if strings.TrimSpace(cmd.KPIMetric) != "" {
		if err := h.applyKPIReport(ctx, tx, deployment, cmd); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (h *DeploymentCallbackHandler) applyAcknowledgement(ctx context.Context, tx *sqlx.Tx, deployment *db.ContractDeployment, cmd DeploymentCallbackCmd) error {
	activatedAt := time.Now().UTC()
	receipt := DeploymentReceiptPayload{
		CorrelationID: cmd.CorrelationID,
		PayloadHash:   deployment.ContentHash,
	}
	if cmd.Receipt != nil {
		if cmd.Receipt.PayloadHash != "" {
			receipt.PayloadHash = cmd.Receipt.PayloadHash
		}
		if cmd.Receipt.ActivatedAt != "" {
			receipt.ActivatedAt = cmd.Receipt.ActivatedAt
		}
	}
	if receipt.ActivatedAt == "" {
		receipt.ActivatedAt = activatedAt.Format(time.RFC3339Nano)
	}

	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal execution-evidence receipt: %w", err)
	}
	receiptSum := sha256.Sum256(receiptBytes)
	receiptHash := "sha256:" + hex.EncodeToString(receiptSum[:])

	tsaToken := ""
	if h.ArchiveTSA != nil && h.ArchiveTSA.Enabled() {
		tsaReceipt, err := h.ArchiveTSA.TimestampBytes(ctx, receiptBytes)
		if err != nil {
			return fmt.Errorf("could not timestamp execution-evidence receipt: %w", err)
		}
		tsaToken = tsaReceipt.Token
	}

	if err := h.DeploymentRepo.AcknowledgeDeployment(ctx, tx, cmd.CorrelationID, receiptHash, tsaToken, activatedAt); err != nil {
		return fmt.Errorf("could not acknowledge deployment %s: %w", cmd.CorrelationID, err)
	}

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, deployment.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", deployment.DID, err)
	}
	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventDeploy); err != nil {
		return err
	}
	if err := h.CRepo.UpdateState(ctx, tx, deployment.DID, contractstate.Active.String()); err != nil {
		return fmt.Errorf("could not activate contract %s: %w", deployment.DID, err)
	}

	return nil
}

func (h *DeploymentCallbackHandler) applyKPIReport(ctx context.Context, tx *sqlx.Tx, deployment *db.ContractDeployment, cmd DeploymentCallbackCmd) error {
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, deployment.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", deployment.DID, err)
	}
	var contractDataBytes []byte
	if contract.ContractData != nil && contract.ContractData.IsNotNullValue() {
		contractDataBytes = []byte(*contract.ContractData)
	}

	violation := EvaluateKPIViolation(contractDataBytes, cmd.KPIMetric, cmd.KPIValue)
	correlationID := cmd.CorrelationID

	if err := h.DeploymentRepo.CreateKPI(ctx, tx, db.ContractKPI{
		DID:           deployment.DID,
		CorrelationID: &correlationID,
		Metric:        cmd.KPIMetric,
		Value:         cmd.KPIValue,
		ObservedAt:    time.Now().UTC(),
		Violation:     violation,
	}); err != nil {
		return fmt.Errorf("could not store KPI report: %w", err)
	}

	return nil
}
