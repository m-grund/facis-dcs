package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// ContractDeployment is one dispatch of a contract to the configured
// Contract Target System (UC-05-01), keyed by CorrelationID for matching the
// target's later ack/status/KPI callbacks.
type ContractDeployment struct {
	ID              int64      `db:"id"`
	DID             string     `db:"did"`
	ContractVersion int        `db:"contract_version"`
	CorrelationID   string     `db:"correlation_id"`
	ContentHash     string     `db:"content_hash"`
	TargetURL       *string    `db:"target_url"`
	Status          string     `db:"status"`
	RequestedBy     string     `db:"requested_by"`
	RequestedAt     time.Time  `db:"requested_at"`
	AcknowledgedAt  *time.Time `db:"acknowledged_at"`
	ReceiptHash     *string    `db:"receipt_hash"`
	TSAToken        *string    `db:"tsa_token"`
}

// ContractKPI is a single KPI value reported via the deployment callback for
// an ACTIVE contract (DCS-FR-CWE-09, DCS-FR-CWE-31).
type ContractKPI struct {
	ID            int64     `db:"id"`
	DID           string    `db:"did"`
	CorrelationID *string   `db:"correlation_id"`
	Metric        string    `db:"metric"`
	Value         string    `db:"value"`
	ObservedAt    time.Time `db:"observed_at"`
	Violation     bool      `db:"violation"`
}

type DeploymentRepo interface {
	CreateDeployment(ctx context.Context, tx *sqlx.Tx, data ContractDeployment) error
	FindDeploymentByCorrelationID(ctx context.Context, tx *sqlx.Tx, correlationID string) (*ContractDeployment, error)
	AcknowledgeDeployment(ctx context.Context, tx *sqlx.Tx, correlationID string, receiptHash string, tsaToken string, acknowledgedAt time.Time) error
	CreateKPI(ctx context.Context, tx *sqlx.Tx, data ContractKPI) error
	ReadKPIsByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractKPI, error)
}
