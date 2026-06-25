package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type GetAllReviewTasksForDIDQry struct {
	DID         string
	RetrievedBy string
}

type GetAllReviewTasksForDIDResult struct {
	ID        string
	DID       string
	State     reviewtaskstate.ReviewTaskState
	Reviewer  string
	CreatedBy string
	CreatedAt time.Time
}

type GetAllReviewTasksForDIDHandler struct {
	DB     *sqlx.DB
	RTRepo db.ReviewTaskRepo
}

func (h *GetAllReviewTasksForDIDHandler) Handle(ctx context.Context, query GetAllReviewTasksForDIDQry) ([]GetAllReviewTasksForDIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	reviewTasks, err := h.RTRepo.ReadAllByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	result := make([]GetAllReviewTasksForDIDResult, len(reviewTasks))
	for i, data := range reviewTasks {

		state, err := reviewtaskstate.NewReviewTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create review task state: %w", err)
		}

		result[i] = GetAllReviewTasksForDIDResult{
			ID:        data.ID,
			DID:       data.DID,
			State:     state,
			Reviewer:  data.Reviewer,
			CreatedBy: data.CreatedBy,
			CreatedAt: data.CreatedAt,
		}
	}

	return result, nil
}
