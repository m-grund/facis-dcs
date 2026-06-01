package contracttemplate

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetAuditLogQry struct {
	DID       string
	AuditedBy string
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
		err := tx.Rollback()
		if err != nil {
			log.Printf("failed to rollback transaction: %s", err)
		}
	}(tx)

	result, err := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.ContractTemplateRepo, qry.DID)
	if err != nil {
		return nil, err
	}

	evt := templateevents.AuditEvt{
		DID:           qry.DID,
		ComponentType: componenttype.ContractTemplateRepo,
		AuditedBy:     qry.AuditedBy,
		OccurredAt:    time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
