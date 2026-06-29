package remoteaction

import (
	"context"
	"database/sql"
	"encoding/json"
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

func ConvertAny[T any](raw any) (*T, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &out, nil
}

func CallRemoteAction(ctx context.Context, db *sqlx.DB, sRepo db.SyncRepository, action string, localPeer string, mainPeer string, contractDid string, payload any) error {
	hostname, err := base.DIDWebToHostname(mainPeer)
	if err != nil {
		return err
	}

	client := dcstodcssynchronizer.NewDCSToDCSHttpClient(hostname)
	_, err = client.Action(ctx, &dcstodcs.DCSToDCSContractActionRequest{
		FromPeerDid: localPeer,
		Payload:     payload,
		Action:      action,
	})

	if err != nil {
		return err
	}

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
	return tx.Commit()
}
