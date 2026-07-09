// Package remoteaction is the outbound half of the single-writer-per-
// aggregate forwarding used throughout contractworkflowengine/command: when
// a handler finds it is not running on a contract's Origin peer, it calls
// RemoteAction.Execute here to forward the exact same command, unmutated,
// to the Origin over a did:web challenge-response-signed RPC (see
// dcstodcs.NewDCSToDCSHttpClient and ADR-0004/ADR-0005).
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
	Offer             RemoteAction = "OFFER"
	Withdraw          RemoteAction = "WITHDRAW"
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
	Terminate:         true,
	Offer:             true,
	Withdraw:          true,
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

// Execute signs a fresh random secret with this node's private key and
// sends it alongside the forwarded command payload to mainPeer, which
// resolves this node's public key via did:web and verifies the signature
// (proof of possession) instead of relying on a shared token — there is no
// common auth authority across independently operated DCS instances. It
// then records a RemoteActionRequestEvent locally for audit purposes.
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
