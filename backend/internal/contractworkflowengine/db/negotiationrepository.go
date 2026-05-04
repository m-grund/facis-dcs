package db

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"time"

	"github.com/jmoiron/sqlx"
)

type NegotiationCreateData struct {
	DID             string         `db:"did"`
	ContractVersion *int           `db:"contract_version"`
	ChangeRequest   *datatype.JSON `db:"change_request"`
	CreatedBy       string         `db:"created_by"`
}

type NegotiationData struct {
	ID              string         `db:"id"`
	DID             string         `db:"did"`
	ContractVersion *int           `db:"contract_version"`
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

type NegotiationRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data NegotiationCreateData, negotiators []string) (*time.Time, error)
	Accept(ctx context.Context, tx *sqlx.Tx, id string, acceptedBy string) error
	Reject(ctx context.Context, tx *sqlx.Tx, id string, rejectedBy string, rejectionReason *string) error
	ReadAllByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationData, error)
	ReadAllAcceptedByContractDIDAndVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) ([]NegotiationChangeData, error)
	HasOpenNegotiationDecisions(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) (bool, error)
	HasNegotiationForContractVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion *int) (bool, error)
	Delete(ctx context.Context, tx *sqlx.Tx, did string) error
}
