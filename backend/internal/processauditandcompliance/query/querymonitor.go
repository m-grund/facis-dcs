package qry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// RiskTypeMissingApproval flags a contract sitting in an approval-pending
// state while at least one required approval task is still OPEN
// (DCS-FR-PACM-03: contracts failing compliance checks are flagged for
// review; the missing approval is the policy violation being monitored).
const RiskTypeMissingApproval = "MISSING_APPROVAL"

// approvalPendingStates are the contract states in which an OPEN approval
// task means the contract is waiting on a required approval decision.
var approvalPendingStates = map[contractstate.ContractState]bool{
	contractstate.Submitted: true,
	contractstate.Reviewed:  true,
}

type MonitorQry struct {
	MonitoredBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type MonitorResult struct {
	CheckedAt time.Time
	Risks     []event2.ComplianceRisk
}

type ComplianceMonitor struct {
	DB     *sqlx.DB
	ATRepo cwedb.ApprovalTaskRepo
	CRepo  cwedb.ContractRepo
}

// Handle sweeps all OPEN approval tasks and flags those whose contract is in
// an approval-pending state as MISSING_APPROVAL risks. The sweep itself is
// recorded in the audit trail via ComplianceMonitorEvent, risks included, so
// a detected risk is both flagged (response) and reported (audit trail).
func (h *ComplianceMonitor) Handle(ctx context.Context, query MonitorQry) (*MonitorResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	openTasks, err := h.ATRepo.ReadAllInState(ctx, tx, approvaltaskstate.Open.String())
	if err != nil {
		return nil, fmt.Errorf("could not read open approval tasks: %w", err)
	}

	checkedAt := time.Now().UTC()
	risks := make([]event2.ComplianceRisk, 0)
	seen := make(map[string]bool)
	for _, task := range openTasks {
		if seen[task.DID+"|"+task.Approver] {
			continue
		}
		seen[task.DID+"|"+task.Approver] = true

		data, err := h.CRepo.ReadDataByDID(ctx, tx, task.DID)
		if err != nil {
			return nil, fmt.Errorf("could not read contract %s for open approval task: %w", task.DID, err)
		}
		state, err := contractstate.NewContractState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not parse contract state for %s: %w", task.DID, err)
		}
		if !approvalPendingStates[state] {
			continue
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:      task.DID,
			RiskType: RiskTypeMissingApproval,
			Detail: fmt.Sprintf(
				"contract in state %s is missing a required approval from %s",
				state, task.Approver),
			DetectedAt: checkedAt,
		})
	}

	evt := event2.ComplianceMonitorEvent{
		MonitoredBy: query.MonitoredBy,
		OccurredAt:  checkedAt,
		Risks:       risks,
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	if err := event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance); err != nil {
		return nil, fmt.Errorf("could not create monitor event: %w", err)
	}
	// Anchor each risk against the affected contract's PAC chain — the
	// sweep event above has no resource DID and only reaches the global
	// chain (see ComplianceRiskEvent doc).
	for _, risk := range risks {
		riskEvt := event2.ComplianceRiskEvent{
			DID:         risk.DID,
			RiskType:    risk.RiskType,
			Detail:      risk.Detail,
			MonitoredBy: query.MonitoredBy,
			OccurredAt:  checkedAt,
			HolderDID:   query.HolderDID,
			UserRoles:   query.UserRoles,
		}
		if err := event.Create(ctx, tx, riskEvt, componenttype.ProcessAuditAndCompliance); err != nil {
			return nil, fmt.Errorf("could not create compliance risk event for %s: %w", risk.DID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &MonitorResult{CheckedAt: checkedAt, Risks: risks}, nil
}
