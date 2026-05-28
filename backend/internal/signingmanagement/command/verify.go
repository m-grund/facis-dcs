package command

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/signingmanagement/db"

	"github.com/jmoiron/sqlx"
)

// VerifyCmd carries the inputs for verifying a contract's signatures.
type VerifyCmd struct {
	DID string
}

// VerifyResult holds the signature verification summary.
type VerifyResult struct {
	// ActiveSigCount is the number of non-revoked signatures on the contract.
	ActiveSigCount int
}

// SignatureVerifier handles the VerifyCmd command.
type SignatureVerifier struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle verifies that the contract is APPROVED and returns the count of
// active (non-revoked) signatures. Hash comparison is performed at the
// service layer where PDF bytes are available.
func (h *SignatureVerifier) Handle(ctx context.Context, cmd VerifyCmd) (*VerifyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	// Validates APPROVED state via repo filter.
	if _, err := h.CRepo.ReadProcessData(ctx, tx, cmd.DID); err != nil {
		return nil, fmt.Errorf("contract %s not available for verification: %w", cmd.DID, err)
	}

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contract_signatures WHERE contract_did=$1 AND status != 'REVOKED'`,
		cmd.DID,
	).Scan(&count); err != nil {
		return nil, fmt.Errorf("count signatures: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &VerifyResult{ActiveSigCount: count}, nil
}
