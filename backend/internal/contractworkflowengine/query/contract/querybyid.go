package contract

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

// ErrContractAccessDenied is returned when the caller is not an authorized
// party of the contract (DCS: created contracts are accessible only to
// authorized parties). Mapped to HTTP 403 by the service layer.
var ErrContractAccessDenied = errors.New("not authorized to access this contract")

// privilegedReadRoles may read any contract regardless of party membership:
// the Sys.* machine roles (cross-org automation), the system administrator,
// and the Auditor (whose function is org-independent audit access).
var privilegedReadRoles = map[userrole.UserRole]bool{
	userrole.SystemContractCreator:  true,
	userrole.SystemContractReviewer: true,
	userrole.SystemContractApprover: true,
	userrole.SystemContractManager:  true,
	userrole.SystemContractSigner:   true,
	userrole.SystemAdministrator:    true,
	userrole.Auditor:                true,
}

type GetByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
	// Internal marks a trusted in-process caller (e.g. the dcs-to-dcs
	// synchronizer reading as "System") that bypasses party read-scoping.
	// Never set from an HTTP request path.
	Internal bool
	// LocalPeer is this instance's own peer DID. Contracts whose Origin is a
	// DIFFERENT peer were adopted via trusted-peer sync — the remote origin
	// only syncs to its responsible peers, so an adopted copy is by
	// construction shared with this instance's organization and is readable
	// by its authenticated users. Party scoping applies strictly to
	// contracts originated locally.
	LocalPeer string
}

type GetByIDResult struct {
	DID             string
	ContractVersion int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContractData    *datatype.JSON
	Negotiations    []db.NegotiationData
	TemplateDID     string
	TemplateVersion int
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Responsible     *db.Responsible
	Origin          string
}

type GetByIDHandler struct {
	Ctx   context.Context
	DB    *sqlx.DB
	CRepo db.ContractRepo
	NRepo db.NegotiationRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadDataByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract data: %w", err)
	}

	if !callerMayReadContract(query, data) {
		// The denial itself is auditable access history ("the access denial
		// is logged with timestamp"), so commit the denied-event before
		// returning the sentinel.
		deniedEvt := contractevents.RetrieveByIDDeniedEvent{
			DID:         query.DID,
			RetrievedBy: query.RetrievedBy,
			OccurredAt:  time.Now().UTC(),
			HolderDID:   query.HolderDID,
			UserRoles:   query.UserRoles,
		}
		if err := event.Create(h.Ctx, tx, deniedEvt, componenttype.ContractWorkflowEngine); err != nil {
			return nil, fmt.Errorf("could not create denied event: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit denied event: %w", err)
		}
		return nil, ErrContractAccessDenied
	}

	negotiations, err := h.NRepo.ReadAllByContractDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get negotiations: %w", err)
	}

	evt := contractevents.RetrieveByIDEvent{
		DID:         query.DID,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(h.Ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	state, err := contractstate.NewContractState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract state: %w", err)
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if data.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*data.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	result := &GetByIDResult{
		DID:             query.DID,
		ContractVersion: data.ContractVersion,
		State:           state,
		Name:            data.Name,
		Description:     data.Description,
		CreatedBy:       data.CreatedBy,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
		ContractData:    data.ContractData,
		TemplateDID:     data.TemplateDID,
		TemplateVersion: data.TemplateVersion,
		Negotiations:    negotiations,
		StartDate:       data.StartDate,
		ExpDate:         data.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: data.ExpNoticePeriod,
		Responsible:     data.Responsible,
		Origin:          data.Origin,
	}
	return result, nil
}

// callerMayReadContract enforces party read-scoping: the caller's
// organization (middleware.GetParticipantID, the OID4VP-disclosed
// organization claim — the same value persisted as CreatedBy on create) must
// be the contract's creating organization or one of the organizations listed
// in the contract document's top-level "dcs:parties" array. Privileged
// org-independent roles (Sys.* automation, Sys. Administrator, Auditor)
// bypass the check.
func callerMayReadContract(query GetByIDQry, data *db.Contract) bool {
	if query.Internal {
		return true
	}
	if query.LocalPeer != "" && data.Origin != "" && data.Origin != query.LocalPeer {
		return true
	}
	for _, role := range query.UserRoles {
		if privilegedReadRoles[role] {
			return true
		}
	}
	if query.RetrievedBy != "" && query.RetrievedBy == data.CreatedBy {
		return true
	}
	for _, party := range contractParties(data.ContractData) {
		if query.RetrievedBy != "" && query.RetrievedBy == party {
			return true
		}
	}
	return false
}

// contractParties reads the optional top-level "dcs:parties" array (plain
// organization-name strings) from the contract JSON-LD document. Absence
// simply means no additional parties beyond the creating organization.
func contractParties(raw *datatype.JSON) []string {
	if raw == nil {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil
	}
	entries, ok := doc["dcs:parties"].([]any)
	if !ok {
		return nil
	}
	parties := make([]string, 0, len(entries))
	for _, entry := range entries {
		if s, ok := entry.(string); ok {
			parties = append(parties, s)
		}
	}
	return parties
}
