package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/contractworkflowengine/event"
)

type GetAuditLogQry struct {
	DID       string
	AuditedBy string
	Username  string
	UserRoles userrole.UserRoles
}

type Auditor struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
}

func (h *Auditor) Handle(ctx context.Context, qry GetAuditLogQry) ([]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	result, err := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.ContractWorkflowEngine, qry.DID)
	if err != nil {
		return nil, err
	}

	evt := event2.AuditEvent{
		DID:           qry.DID,
		ComponentType: componenttype.ContractWorkflowEngine,
		AuditedBy:     qry.AuditedBy,
		OccurredAt:    time.Now().UTC(),
		Username:      qry.Username,
		UserRoles:     qry.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
