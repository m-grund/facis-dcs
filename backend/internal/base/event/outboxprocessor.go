package event

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

type OutboxProcessor struct {
	DB         *sqlx.DB
	PubClient  *CloudEventPubClient
	IPFSClient *ipfs.APIClient
	ARepo      db.AuditTrailRepository
}

func (j OutboxProcessor) Start(ctx context.Context) error {
	go j.startProcessingJob(ctx, conf.OutboxProcessorTimeOut())
	return nil
}

func (j OutboxProcessor) startProcessingJob(ctx context.Context, interval time.Duration) {
	if j.PubClient == nil {
		return
	}

	schedulerLogic := func() error {
		tx, err := j.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer tx.Rollback()

		rows, err := tx.QueryxContext(ctx, `
			SELECT id, component, event_type, event_data, did, created_at
			FROM outbox_events
			WHERE processed = FALSE
			ORDER BY created_at ASC
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		`)
		if err != nil {
			return fmt.Errorf("could not query outbox events: %w", err)
		}

		var events []datatype.OutboxEvent
		for rows.Next() {
			var event datatype.OutboxEvent
			if err := rows.StructScan(&event); err != nil {
				rows.Close()
				return fmt.Errorf("could not scan event: %w", err)
			}
			events = append(events, event)
		}
		rows.Close()

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		if len(events) > 0 {
			log.Println("process ", len(events), " events")
		}

		for _, event := range events {
			if err := j.processEvent(ctx, event); err != nil {
				log.Printf("could not process event %d: %v", event.ID, err)
				return err
			}
		}

		return nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := schedulerLogic(); err != nil {
			log.Printf("could not process outbox entries: %v", err)
		}
	}
}

func (j OutboxProcessor) processEvent(ctx context.Context, event datatype.OutboxEvent) error {
	tx, err := j.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	if err := j.PubClient.Publish(event.Component, event.EventType, event.EventData); err != nil {
		return fmt.Errorf("could not publish event %d: %w", event.ID, err)
	}

	globalLogPredCID, err := j.ARepo.ReadLogCID(ctx, tx, conf.GlobalAuditTrailName(), conf.GlobalAuditTrailName())
	if err != nil {
		return fmt.Errorf("could not read log CID: %w", err)
	}

	var resLogPredCID *string
	switch event.Component {
	case componenttype.ContractTemplateRepo.String():
		if event.DID != nil && len(*event.DID) > 1 {
			resLogPredCID, err = j.ARepo.ReadLogCID(ctx, tx, event.Component, *event.DID)
			if err != nil {
				return fmt.Errorf("could not read log CID: %w", err)
			}
		}
	case componenttype.ContractWorkflowEngine.String():
		if event.DID != nil && len(*event.DID) > 1 {
			resLogPredCID, err = j.ARepo.ReadLogCID(ctx, tx, event.Component, *event.DID)
			if err != nil {
				return fmt.Errorf("could not read log CID: %w", err)
			}
		}
	}

	auditLogEntry := datatype.AuditLogEntry{
		ID:               event.ID,
		Component:        event.Component,
		EventType:        event.EventType,
		EventData:        event.EventData,
		DID:              event.DID,
		CreatedAt:        event.CreatedAt,
		ResLogPredCID:    resLogPredCID,
		GlobalLogPredCID: globalLogPredCID,
	}

	result, err := j.IPFSClient.CreateFile(ctx, auditLogEntry)
	if err != nil {
		return fmt.Errorf("could not create IPFS file for event %d: %w", event.ID, err)
	}
	globalLogPredCID = &result.Identifier.Value

	switch event.Component {
	case componenttype.ContractTemplateRepo.String():
		if event.DID != nil && len(*event.DID) > 1 {
			if err = j.ARepo.UpdateLogCID(ctx, tx, event.Component, *event.DID, &result.Identifier.Value); err != nil {
				return fmt.Errorf("could not update log CID: %w", err)
			}
		}
	case componenttype.ContractWorkflowEngine.String():
		if event.DID != nil && len(*event.DID) > 1 {
			if err = j.ARepo.UpdateLogCID(ctx, tx, event.Component, *event.DID, &result.Identifier.Value); err != nil {
				return fmt.Errorf("could not update log CID: %w", err)
			}
		}
	}

	if err = j.ARepo.UpdateLogCID(ctx, tx, conf.GlobalAuditTrailName(), conf.GlobalAuditTrailName(), &result.Identifier.Value); err != nil {
		return fmt.Errorf("could not update log CID: %w", err)
	}

	err = db.UpdateOutboxEvent(ctx, tx, event.ID)
	if err != nil {
		return fmt.Errorf("could not update outbox event: %w", err)
	}

	return tx.Commit()
}
