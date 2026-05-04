package query

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type GetAuditLogQry struct {
	Scope     componenttype.ComponentType
	AuditedBy string
}

type Auditor struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
}

func (h *Auditor) Handle(ctx context.Context, cmd GetAuditLogQry) ([][]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := h.ATrailReader.ReadAuditLogEntriesByComponent(ctx, tx, cmd.Scope)
	if err != nil {
		return nil, err
	}

	evt := event2.AuditEvent{
		Scope:         cmd.Scope,
		ComponentType: componenttype.ProcessAuditAndCompliance,
		AuditedBy:     cmd.AuditedBy,
		OccurredAt:    time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
