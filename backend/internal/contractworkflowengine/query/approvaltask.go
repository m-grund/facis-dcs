// Package query implements the read-side CQRS use cases for the contract
// workflow engine that operate across tasks/DIDs directly (contract- and
// template-scoped queries live in the contract/ and contracttemplate/
// subpackages). Like the command package, most handlers also emit an
// audit-trail event for the read via base/event.Create.
package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	aopprovaltaskstate "digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type GetAllApprovalTasksForDIDQry struct {
	DID         string
	RetrievedBy string
}

type GetAllApprovalTasksForDIDResult struct {
	ID          string
	DID         string
	State       aopprovaltaskstate.ApprovalTaskState
	Approver    string
	CreatedBy   string
	CreatedAt   time.Time
	CancelledAt *time.Time
}

type GetAllApprovalTasksForDIDHandler struct {
	DB     *sqlx.DB
	ATRepo db.ApprovalTaskRepo
}

func (h *GetAllApprovalTasksForDIDHandler) Handle(ctx context.Context, query GetAllApprovalTasksForDIDQry) ([]GetAllApprovalTasksForDIDResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	approvalTasks, err := h.ATRepo.ReadAllByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	var approvalTaskResults []GetAllApprovalTasksForDIDResult
	for _, data := range approvalTasks {

		state, err := aopprovaltaskstate.NewApprovalTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create approval task state: %w", err)
		}

		approvalTaskResults = append(approvalTaskResults, GetAllApprovalTasksForDIDResult{
			ID:        data.ID,
			DID:       data.DID,
			State:     state,
			Approver:  data.Approver,
			CreatedBy: data.CreatedBy,
			CreatedAt: data.CreatedAt,
		})
	}

	return approvalTaskResults, nil
}
