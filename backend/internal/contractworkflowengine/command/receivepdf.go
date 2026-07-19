package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/jsonld"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// PeerPdfReceiveCmd carries the machine-readable JSON-LD extracted from a
// contract PDF that a counterparty shipped (ADR-13). The receiver rebuilds its
// own local copy of the contract from it.
type PeerPdfReceiveCmd struct {
	ContractIRI string
	// Counterparty is the peer that shipped the PDF — the contract's origin,
	// from this instance's perspective.
	Counterparty string
	// LocalPeer is this instance's own DID — the other party and the holder of
	// the local RBAC roles.
	LocalPeer string
	// Payload is the JSON-LD contract document pdf-core extracted from the PDF.
	Payload []byte
}

// PeerPdfReceiver upserts a peer-shipped contract into this instance's own
// store and opens its own local workflow tasks (ADR-13): each DCS runs its own
// RBAC; nothing crosses the boundary.
type PeerPdfReceiver struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

// Handle upserts the local copy from the shipped contract's JSON-LD. A first
// ship creates the copy owned by the counterparty (its origin); a later ship
// updates the content and bumps the local version. The contract lands in
// NEGOTIATION — the settlement and signing phases are separate ships (ADR-13).
func (h *PeerPdfReceiver) Handle(ctx context.Context, cmd PeerPdfReceiveCmd) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	existing, err := h.CRepo.ReadProcessDataByDIDOrNil(ctx, tx, cmd.ContractIRI)
	if err != nil {
		return fmt.Errorf("could not read local contract copy: %w", err)
	}

	// pdf-core embeds the canonical, IRI-expanded JSON-LD; re-compact it to the
	// FACIS context so this copy matches the originator's compact form (ADR-13).
	compacted, err := jsonld.CompactToFacis(cmd.Payload)
	if err != nil {
		return fmt.Errorf("could not re-compact shipped contract JSON-LD: %w", err)
	}
	payload := datatype.JSON(compacted)
	templateIRI, templateVersion, name := parseShippedContractMeta(compacted)
	now := time.Now().UTC()

	data := db.Contract{
		DID:             cmd.ContractIRI,
		State:           contractstate.Negotiation.String(),
		UpdatedAt:       now,
		ContractData:    &payload,
		TemplateDID:     templateIRI,
		TemplateVersion: templateVersion,
		Name:            name,
	}

	if existing != nil {
		data.Origin = existing.Origin
		data.CreatedBy = existing.CreatedBy
		data.ContractVersion = existing.ContractVersion + 1
		if err := h.CRepo.RemoteUpdate(ctx, tx, data); err != nil {
			return fmt.Errorf("could not update local contract copy: %w", err)
		}
		return tx.Commit()
	}

	// The two parties are objective on both copies: the origin (the peer that
	// created and offered the contract) and this instance. This instance's own
	// users hold the local RBAC roles.
	resp := db.Responsible{
		Creator:      cmd.Counterparty,
		Counterparty: cmd.LocalPeer,
		Reviewers:    []string{cmd.LocalPeer},
		Approvers:    []string{cmd.LocalPeer},
		Negotiators:  []string{cmd.LocalPeer},
	}
	data.Origin = cmd.Counterparty
	data.CreatedBy = cmd.Counterparty
	data.CreatedAt = now
	data.ContractVersion = 1
	data.Responsible = &resp
	if err := h.CRepo.RemoteCreate(ctx, tx, data); err != nil {
		return fmt.Errorf("could not create local contract copy: %w", err)
	}
	if err := createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd.ContractIRI, cmd.LocalPeer, resp); err != nil {
		return err
	}
	return tx.Commit()
}

// parseShippedContractMeta pulls the template provenance and title out of the
// shipped contract's JSON-LD (derivedFromTemplate.@id ends in the template IRI).
func parseShippedContractMeta(payload []byte) (templateIRI string, templateVersion int, name *string) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return "", 0, nil
	}
	if prov, ok := doc["derivedFromTemplate"].(map[string]any); ok {
		if id, ok := prov["@id"].(string); ok {
			templateIRI = id[strings.LastIndex(id, "/")+1:]
		}
		if v, ok := prov["version"].(float64); ok {
			templateVersion = int(v)
		}
	}
	if meta, ok := doc["dcs:metadata"].(map[string]any); ok {
		if t, ok := meta["dcs:title"].(string); ok && t != "" {
			name = &t
		}
	}
	return templateIRI, templateVersion, name
}
