package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type PeerUpdateRequestCmd struct {
	FromPeerDID string
	ContractDID string
	UpdatedAt   time.Time
}

type PeerUpdateRequester struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	DIDDocument base.DIDDocument
}

func (h *PeerUpdateRequester) Handle(ctx context.Context, cmd PeerUpdateRequestCmd) error {

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.ContractDID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if localPeer != processData.Origin {
		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		err = remoteaction.PeerUpdate.Execute(ctx, h.DB, localPeer, processData.Origin, processData.DID, cmd)
		if err != nil {
			return err
		}

		return nil
	}

	evt := contractevents.OutdatedPeerEvent{
		DID:             cmd.ContractDID,
		OutdatedPeerDID: cmd.FromPeerDID,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
