package remoteaction

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base/identity"

	dcstodcs2 "digital-contracting-service/internal/dcstodcs"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type RemoteAction string

const (
	PeerUpdate        RemoteAction = "PEER_UPDATE"
	AcceptNegotiation RemoteAction = "ACCEPT_NEGOTIATION"
	Approve           RemoteAction = "APPROVE"
	Negotiate         RemoteAction = "NEGOTIATE"
	RecordEvidence    RemoteAction = "RECORD_EVIDENCE"
	Reject            RemoteAction = "REJECT"
	RejectNegotiation RemoteAction = "REJECT_NEGOTIATION"
	Submit            RemoteAction = "SUBMIT"
	Terminate         RemoteAction = "TERMINATE"
	Update            RemoteAction = "UPDATE"
)

var validAction = map[RemoteAction]bool{
	PeerUpdate:        true,
	AcceptNegotiation: true,
	Approve:           true,
	Negotiate:         true,
	RecordEvidence:    true,
	Reject:            true,
	RejectNegotiation: true,
	Update:            true,
	Submit:            true,
}

func NewRemoteAction(s string) (RemoteAction, error) {
	flag := RemoteAction(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid remote action value: %s", s)
	}
	return flag, nil
}

// IsValid checks if the RemoteAction is a valid role
func (a RemoteAction) IsValid() bool {
	upper := RemoteAction(strings.ToUpper(string(a)))
	return validAction[upper]
}

// String returns the string representation of the RemoteAction
func (a RemoteAction) String() string {
	return string(a)
}

func (a RemoteAction) Execute(ctx context.Context, db *sqlx.DB, didDocument identity.DIDDocument, mainPeer string, contractDid string, payload any) error {

	localPeer, err := didDocument.GetID()
	if err != nil {
		return err
	}

	hostname, err := identity.DIDWebToHostname(mainPeer)
	if err != nil {
		return err
	}

	secretValue := rand.Text()
	secretHash, err := didDocument.Sign([]byte(secretValue))
	if err != nil {
		return err
	}

	client := dcstodcs2.NewDCSToDCSHttpClient(hostname)
	_, err = client.Action(ctx, &dcstodcs.DCSToDCSContractActionRequest{
		FromPeerDid: localPeer,
		Payload:     payload,
		Action:      a.String(),
		Component:   componenttype.ContractWorkflowEngine.String(),
		SecretHash:  secretHash,
		SecretValue: secretValue,
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
		Action:      a.String(),
		FromPeerDID: localPeer,
		MainPeerDID: mainPeer,
		OccurredAt:  time.Now().UTC(),
		Component:   componenttype.ContractWorkflowEngine.String(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	return tx.Commit()
}
