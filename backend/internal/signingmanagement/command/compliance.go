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

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

// poaComplianceFindings walks the sealed agreement's party nodes and reports any
// signed party (dcs:hasSignatory present) that signed with no Power of Attorney,
// or under one authorizing a different organization than the party it signed as
// (UC-14, FR-SM-03/-04). The organization rides the party node so this holds for
// a counterparty's signature synced from another instance.
func poaComplianceFindings(raw datatype.JSON) []string {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	nodes, _ := doc["dcs:parties"].([]any)
	var findings []string
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		signatory := nodeIRI(node["dcs:hasSignatory"])
		if signatory == "" {
			continue
		}
		party, _ := node["@id"].(string)
		poaOrg := nodeIRI(node["dcs:hasPowerOfAttorney"])
		switch {
		case poaOrg == "":
			findings = append(findings, fmt.Sprintf("Party %s signed with no Power of Attorney (signatory %s)", party, signatory))
		case poaOrg != party:
			findings = append(findings, fmt.Sprintf("Party %s signed under a Power of Attorney authorizing %s, not this party (signatory %s)", party, poaOrg, signatory))
		}
	}
	return findings
}

// nodeIRI reads an IRI from a JSON-LD value that is either {"@id": iri} or a
// bare string.
func nodeIRI(v any) string {
	switch t := v.(type) {
	case map[string]any:
		id, _ := t["@id"].(string)
		return strings.TrimSpace(id)
	case string:
		return strings.TrimSpace(t)
	}
	return ""
}

type ComplianceCmd struct {
	DID       string
	CheckedBy string
	HolderDID string
	UserRoles userrole.UserRoles
}

type ComplianceValidator struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle evaluates the contract's signatures against the signature
// compliance policy (DCS-FR-SM-21: signature level SES/AES/QES, signature
// status, presence of active signed credentials) and returns the findings;
// the check itself — findings included — is recorded as an audit event.
func (h *ComplianceValidator) Handle(ctx context.Context, cmd ComplianceCmd) ([]string, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	findings, err := h.CRepo.CollectComplianceFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect compliance findings: %w", err)
	}

	// Power of Attorney (UC-14, FR-SM-03/-04): every signed party — this
	// instance's own and any counterparty whose signature arrived over the peer
	// sync — must have signed under a PoA authorizing the very party it signed as.
	// The organization travels on the party node (dcs:hasPowerOfAttorney), so a
	// counterparty running a misconfigured or malicious DCS is caught here.
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract data: %w", err)
	}
	if contract.ContractData != nil {
		findings = append(findings, poaComplianceFindings(*contract.ContractData)...)
	}

	evt := signingmanagementevents.ComplianceValidationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		CheckedBy:       cmd.CheckedBy,
		Findings:        findings,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return findings, nil
}
