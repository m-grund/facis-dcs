package remoteaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/dcstodcssynchronizer/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/dcstodcssynchronizer"

	"github.com/jmoiron/sqlx"
)

func CallRemoteAction(ctx context.Context, db *sqlx.DB, sRepo db.SyncRepository, action string, localPeer string, mainPeer string, contractDid string, payload any) error {
	hostname, err := base.DIDWebToHostname(mainPeer)
	if err != nil {
		return err
	}

	client := dcstodcssynchronizer.NewDCSToDCSHttpClient(hostname)
	_, remoteSyncErr := client.Action(ctx, &dcstodcs.DCSToDCSContractActionRequest{
		FromPeerDid: localPeer,
		Payload:     payload,
		Action:      action,
	})

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	evt := contractevents.RemoteActionRequestEvent{
		DID:         contractDid,
		Action:      action,
		FromPeerDID: localPeer,
		MainPeerDID: mainPeer,
		OccurredAt:  time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	if remoteSyncErr != nil {

		err = sRepo.CreateOrUpdateSyncFailEntry(ctx, tx, contractDid)
		if err != nil {
			return fmt.Errorf("could not create or update sync fail entry: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		return remoteSyncErr

	} else {

		err = sRepo.DeleteSyncFailEntry(ctx, tx, contractDid)
		if err != nil {
			return fmt.Errorf("could not create or update sync fail entry: %w", err)
		}
	}

	return tx.Commit()
}
