package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype"

	negotiationdescision "digital-contracting-service/internal/contractworkflowengine/datatype/negotiationaction"

	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type GetAllNegotiationsForDIDQry struct {
	DID         string
	RetrievedBy string
}

type NegotiationTaskItem struct {
	ID         string
	DID        string
	State      negotiationtaskstate.NegotiationTaskState
	Negotiator string
	CreatedBy  string
	CreatedAt  time.Time
}

type NegotiationItem struct {
	ID              string
	DID             string
	ContractVersion int
	ChangeRequest   *datatype.JSON
	CreatedBy       string
	CreatedAd       time.Time
}

type NegotiationDecisionItem struct {
	ID            string
	NegotiationID string
	Negotiator    string
	Decision      *negotiationdescision.NegotiationDecision
	RejectReason  *string
}

type GetAllNegotiationsForDIDResult struct {
	NegotiationTasks     []NegotiationTaskItem
	Negotiations         []NegotiationItem
	NegotiationDecisions []NegotiationDecisionItem
}

type GetAllNegotiationsForDIDHandler struct {
	DB     *sqlx.DB
	NTRepo db.NegotiationTaskRepo
	NRepo  db.NegotiationRepo
}

func (h *GetAllNegotiationsForDIDHandler) Handle(ctx context.Context, query GetAllNegotiationsForDIDQry) (*GetAllNegotiationsForDIDResult, error) {

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

	negotiationTasks, err := h.NTRepo.ReadAllByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiation tasks: %w", err)
	}

	var resultNegotiationTasks []NegotiationTaskItem
	for _, negotiationTask := range negotiationTasks {

		state, err := negotiationtaskstate.NewNegotiationTaskState(negotiationTask.State)
		if err != nil {
			return nil, fmt.Errorf("could not create negotiation task state: %w", err)
		}

		resultNegotiationTasks = append(resultNegotiationTasks, NegotiationTaskItem{
			ID:         negotiationTask.ID,
			DID:        negotiationTask.DID,
			State:      state,
			Negotiator: negotiationTask.Negotiator,
			CreatedBy:  negotiationTask.CreatedBy,
			CreatedAt:  negotiationTask.CreatedAt,
		})
	}

	negotiations, err := h.NRepo.ReadAllByContractDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiations: %w", err)
	}

	var resultNegotiations []NegotiationItem
	for _, negotiation := range negotiations {
		resultNegotiations = append(resultNegotiations, NegotiationItem{
			ID:              negotiation.ID,
			DID:             negotiation.DID,
			ContractVersion: negotiation.ContractVersion,
			CreatedBy:       negotiation.CreatedBy,
			CreatedAd:       negotiation.CreatedAt,
			ChangeRequest:   negotiation.ChangeRequest,
		})
	}

	negotiationDecisions, err := h.NRepo.ReadAllNegotiationDecisionsByContractDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiation decision data: %w", err)
	}

	var resultNegotiationDecisions []NegotiationDecisionItem
	for _, negotiationDecision := range negotiationDecisions {

		var decision *negotiationdescision.NegotiationDecision
		if negotiationDecision.Decision != nil {
			result, err := negotiationdescision.NewNegotiationDecision(*negotiationDecision.Decision)
			if err != nil {
				return nil, fmt.Errorf("could not create negotiation decision: %w", err)
			}
			decision = &result
		}

		resultNegotiationDecisions = append(resultNegotiationDecisions, NegotiationDecisionItem{
			ID:            negotiationDecision.ID,
			NegotiationID: negotiationDecision.Negotiator,
			Negotiator:    negotiationDecision.Negotiator,
			RejectReason:  negotiationDecision.RejectionReason,
			Decision:      decision,
		})
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &GetAllNegotiationsForDIDResult{
		NegotiationTasks:     resultNegotiationTasks,
		Negotiations:         resultNegotiations,
		NegotiationDecisions: resultNegotiationDecisions,
	}, nil
}
