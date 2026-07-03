package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type NegotiationCreateData struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	ChangeRequest   *datatype.JSON `db:"change_request"`
	CreatedBy       string         `db:"created_by"`
}

type NegotiationData struct {
	ID              string         `db:"id"`
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	ChangeRequest   *datatype.JSON `db:"change_request"`
	Negotiator      string         `db:"negotiator"`
	Decision        *string        `db:"decision"`
	RejectionReason *string        `db:"rejection_reason"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
}

type NegotiationChangeData struct {
	ID            string         `db:"id"`
	ChangeRequest *datatype.JSON `db:"change_request"`
}

type NegotiationDecisionData struct {
	ID              string  `db:"id"`
	NegotiationID   string  `db:"negotiation_id"`
	Negotiator      string  `db:"negotiator"`
	Decision        *string `db:"decision"`
	RejectionReason *string `db:"rejection_reason"`
}

type NegotiationRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data NegotiationCreateData, negotiators []string) (*time.Time, error)
	Accept(ctx context.Context, tx *sqlx.Tx, id string, acceptedBy string) error
	Reject(ctx context.Context, tx *sqlx.Tx, id string, rejectedBy string, rejectionReason *string) error
	ReadAllByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationData, error)
	ReadAllAcceptedByContractDIDAndVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) ([]NegotiationChangeData, error)
	HasOpenNegotiationDecisions(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int, negotiator string) (bool, error)
	HasNegotiationForContractVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) (bool, error)
	ReadAllNegotiationDecisionsByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationDecisionData, error)

	RemoteCreateOrUpdateNegotiation(ctx context.Context, tx *sqlx.Tx, data NegotiationData) error
	RemoteCreateOrUpdateNegotiationDecision(ctx context.Context, tx *sqlx.Tx, data NegotiationDecisionData) error
}
