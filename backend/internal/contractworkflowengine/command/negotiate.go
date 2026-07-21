package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type NegotiationCmd struct {
	DID           string             `json:"did"`
	NegotiatedBy  string             `json:"negotiated_by"`
	ChangeRequest *datatype.JSON     `json:"change_request"`
	UpdatedAt     time.Time          `json:"updated_at"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
	CauserDID     string             `json:"causer_did"`
}

type Negotiator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Negotiator) Handle(ctx context.Context, cmd NegotiationCmd) error {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not process core data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventNegotiate); err != nil {
		return err
	}

	isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not validate negotiator: %w", err)
	}

	if !isValidNegotiator {
		return ErrNotAParty
	}

	negotiators, err := h.NTRepo.ReadNegotiatorsForDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read negotiators: %w", err)
	}

	data := db.NegotiationCreateData{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		CreatedBy:       cmd.NegotiatedBy,
	}
	_, err = h.NRepo.Create(ctx, tx, data, negotiators)
	if err != nil {
		return fmt.Errorf("could not create negotiation: %w", err)
	}

	err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not reopen negotiation: %w", err)
	}

	// Negotiation is where the participating DCS instances are finalized, so it
	// is where the contract's signature fields are materialized: one
	// dcs:SignatureField per instance, the AcroForm field the wallet-driven
	// signing ceremony signs (ADR-12). Without a pre-placed field, the
	// deterministic two-call remote signing has nothing to sign.
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract for signature-field seeding: %w", err)
	}
	seeded, changed, err := seedSignatureFields(*contract.ContractData, contract.Responsible.GetParties())
	if err != nil {
		return fmt.Errorf("could not seed signature fields: %w", err)
	}
	if changed {
		if err := h.CRepo.Update(ctx, tx, db.ContractUpdateData{DID: cmd.DID, ContractData: &seeded}); err != nil {
			return fmt.Errorf("could not persist seeded signature fields: %w", err)
		}
	}

	evt := contractevents.NegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		NegotiatedBy:    cmd.NegotiatedBy,
		Negotiators:     negotiators,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// seedSignatureFields adds one dcs:SignatureField per participating DCS
// instance to the contract document, its dcs:signatoryName set to that
// instance's DID — the value pdf-core renders as the AcroForm field's /T name,
// which the wallet-driven signing ceremony targets (ADR-12). It is merge-aware
// and idempotent: an instance that already has a field — from a template or an
// earlier negotiation pass — is left untouched, so re-running never duplicates.
// It reports whether the document changed so the caller only persists real
// additions.
func seedSignatureFields(raw datatype.JSON, instanceDIDs []string) (datatype.JSON, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("decode contract data: %w", err)
	}

	fields, _ := doc["dcs:signatureFields"].([]any)
	// Explicit declaration wins: a contract that already declares its signature
	// fields (e.g. a multi-signatory contract naming each signer) is signed
	// against exactly those, so the per-party auto-seed does not add an extra
	// instance-DID field on top of them.
	if len(fields) > 0 {
		return raw, false, nil
	}

	present := map[string]bool{}
	docID, _ := doc["@id"].(string)
	changed := false
	for _, did := range instanceDIDs {
		if present[did] {
			continue
		}
		digest := sha256.Sum256([]byte(did))
		fields = append(fields, map[string]any{
			"@id":               fmt.Sprintf("%s#signature-field-%s", docID, hex.EncodeToString(digest[:8])),
			"@type":             "dcs:SignatureField",
			"dcs:signatoryName": did,
		})
		present[did] = true
		changed = true
	}
	if !changed {
		return raw, false, nil
	}

	doc["dcs:signatureFields"] = fields
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, false, fmt.Errorf("encode contract data: %w", err)
	}
	return encoded, true, nil
}
