// Package event subscribes to CWE lifecycle state-change events and appends
// a new C2PA manifest to the contract's stored PDF for each transition
// (DCS-OR-C2PA-001, DCS-OR-C2PA-003, DCS-OR-C2PA-008).
package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/builder"
	"digital-contracting-service/internal/pdfgeneration/c2pa"
	tplevttype "digital-contracting-service/internal/templaterepository/datatype/eventtype"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
)

// contractLifecycleEventTypes is the set of CWE event types that represent a
// contract state change and therefore require a new C2PA assertion.
var contractLifecycleEventTypes = map[string]bool{
	cweeventtype.Create.String():                  true,
	cweeventtype.Submit.String():                  true,
	cweeventtype.Approve.String():                 true,
	cweeventtype.Reject.String():                  true,
	cweeventtype.Terminate.String():               true,
	cweeventtype.ContractExpired.String():         true,
	cweeventtype.Negotiation.String():             true,
	cweeventtype.IncreaseContractVersion.String(): true,
}

// templateLifecycleEventTypes is the set of template repository event types
// that represent a state change and require a new C2PA assertion.
var templateLifecycleEventTypes = map[string]bool{
	tplevttype.Create.String():   true,
	tplevttype.Submit.String():   true,
	tplevttype.Approve.String():  true,
	tplevttype.Reject.String():   true,
	tplevttype.Verify.String():   true,
	tplevttype.Archive.String():  true,
	tplevttype.Register.String(): true,
}

// minimalCWEEvent extracts common fields present in all CWE event structs.
type minimalCWEEvent struct {
	DID        string    `json:"did"`
	NewState   string    `json:"new_state"`
	OccurredAt time.Time `json:"occurred_at"`
	Reason     string    `json:"reason,omitempty"`
}

// Subscriber listens to the NATS event bus and appends C2PA lifecycle
// assertions to the PDF stored in IPFS for each CWE state-change event.
type Subscriber struct {
	DB         *sqlx.DB
	IPFSClient *ipfs.APIClient
	CRepo      *cwerepo.PostgresContractRepo
	TRepo      *tplrepo.PostgresContractTemplateRepo
	Signer     c2pa.Signer
	TSACfg     c2pa.TSAConfig
	IssuerDID  string
	// VCIssuer issues and signs a W3C VC for each lifecycle event (DCS-OR-C2PA-004/005).
	// When nil, no VC is embedded in the C2PA manifest.
	VCIssuer c2pa.VCIssuer
}

// Start registers the event handler with the NATS sub-client and begins
// consuming events. It returns immediately; the subscription runs in the
// background until the sub-client is closed.
func (s *Subscriber) Start(subClient *event.CloudEventSubClient) error {
	return subClient.Subscribe(func(evt cloudevent.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := s.handle(ctx, evt); err != nil {
			log.Printf("pdfgeneration: failed to handle event %s/%s: %v", evt.Source(), evt.Type(), err)
		}
	})
}

func (s *Subscriber) handle(ctx context.Context, evt cloudevent.Event) error {
	isContract := contractLifecycleEventTypes[evt.Type()]
	isTemplate := templateLifecycleEventTypes[evt.Type()]
	if !isContract && !isTemplate {
		return nil
	}

	// The outbox processor publishes the event as json.Marshal([]byte) which
	// JSON-encodes the payload bytes as a base64 string. DataAs(&[]byte) reverses
	// this automatically.
	var rawPayload []byte
	if err := evt.DataAs(&rawPayload); err != nil {
		return fmt.Errorf("decode event payload: %w", err)
	}

	var cweEvt minimalCWEEvent
	if err := json.Unmarshal(rawPayload, &cweEvt); err != nil {
		return fmt.Errorf("unmarshal CWE event: %w", err)
	}
	if cweEvt.DID == "" {
		return nil // non-entity events (e.g. RetrieveAll) have no DID
	}

	if isTemplate {
		return s.appendTemplateC2PA(ctx, cweEvt)
	}
	return s.appendC2PA(ctx, cweEvt)
}

func (s *Subscriber) appendC2PA(ctx context.Context, cweEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Fetch current contract state.
	contract, err := s.CRepo.ReadDataByID(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read contract %s: %w", cweEvt.DID, err)
	}

	state := cweEvt.NewState
	if state == "" {
		state = contract.State
	}
	effectiveAt := cweEvt.OccurredAt
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}

	// Map the raw CWE state to the SRS-defined C2PA vocabulary (DCS-OR-C2PA-003).
	c2paState, err := c2pa.MapCWEStateToC2PAStrict(state)
	if err != nil {
		return fmt.Errorf("map contract state %q to C2PA state: %w", state, err)
	}

	// Fetch the base PDF and check for idempotency.
	var cidStr, currentPdfC2PAState string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_c2pa_state,'') FROM contracts WHERE did=$1`, cweEvt.DID,
	).Scan(&cidStr, &currentPdfC2PAState)

	if currentPdfC2PAState == c2paState {
		// Already embedded by a concurrent export call — skip.
		return nil
	}

	if cidStr == "" {
		return fmt.Errorf("no cached PDF for contract %s; export must be called before state-change events can chain", cweEvt.DID)
	}
	ipfsResult, err := s.IPFSClient.FetchFile(cidStr)
	if err != nil || len(ipfsResult.Data) == 0 {
		return fmt.Errorf("fetch PDF from IPFS %s for contract %s: %w", cidStr, cweEvt.DID, err)
	}
	existingPDF := []byte(ipfsResult.Data)

	fileHash := c2pa.FileHashOf(existingPDF)
	prevHash := c2pa.PrevManifestHashFrom(existingPDF)
	pdfHash := fileHash

	// Issue W3C VC for this lifecycle event when a VCIssuer is configured (DCS-OR-C2PA-004/005).
	var vcID string
	var vcBytes []byte
	if s.VCIssuer != nil {
		vcID, vcBytes, err = s.VCIssuer.IssueContractLifecycleVC(
			ctx, cweEvt.DID, fileHash, c2paState, cweEvt.Reason, s.IssuerDID, effectiveAt,
		)
		if err != nil {
			return fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
		}
	}

	assertion := c2pa.NewLifecycleAssertion(
		cweEvt.DID, fileHash, pdfHash, builder.RendererVersion,
		c2paState, cweEvt.Reason, s.IssuerDID, vcID, prevHash, effectiveAt,
	)

	// Append the C2PA manifest; store the updated PDF in IPFS.
	result, err := c2pa.AppendManifest(ctx, s.Signer, s.TSACfg, s.IPFSClient, assertion, existingPDF, vcBytes)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for %s: %w", cweEvt.DID, err)
	}

	// Update cached PDF, C2PA state, and standalone remote manifest reference in the DB.
	_, err = tx.ExecContext(ctx,
		`UPDATE contracts SET pdf_ipfs_cid = $1, pdf_renderer_version = $2, pdf_c2pa_state = $3, pdf_manifest_hash = $4, pdf_manifest_ipfs_cid = $5, prev_manifest_hash = NULL WHERE did = $6`,
		result.IPFSCID, builder.RendererVersion, c2paState, result.ManifestHash, result.ManifestIPFSCID, cweEvt.DID,
	)
	if err != nil {
		return fmt.Errorf("update pdf_ipfs_cid for %s: %w", cweEvt.DID, err)
	}

	reason := cweEvt.Reason
	if reason == "" {
		reason = reasonForC2PAState(c2paState)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO c2pa_audit_log (entity_type, entity_did, from_state, to_state, actor_did, reason, vc_id, manifest_hash, occurred_at)
		 VALUES ('contract',$1,$2,$3,$4,$5,$6,$7,$8)`,
		cweEvt.DID, nullableString(currentPdfC2PAState), c2paState, s.IssuerDID,
		reason, nullableString(vcID), result.ManifestHash, effectiveAt,
	); err != nil {
		return fmt.Errorf("insert c2pa_audit_log for contract %s: %w", cweEvt.DID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf_ipfs_cid update for %s: %w", cweEvt.DID, err)
	}

	log.Printf("pdfgeneration: updated PDF for contract %s (state=%s) → IPFS CID %s", cweEvt.DID, state, result.IPFSCID)
	return nil
}

// appendTemplateC2PA appends a C2PA lifecycle assertion to a contract template's
// stored PDF in response to a template state-change event (DCS-OR-C2PA-003).
//
// Per SRS DCS-OR-C2PA-003, each lifecycle assertion must be recorded: this method
// ensures a manifest is always appended for every state transition (CREATE, SUBMIT,
// APPROVE, REJECT, VERIFY, ARCHIVE, REGISTER).
//
// If the template has never been exported (pdf_ipfs_cid is empty), the method
// builds a fresh PDF and appends its initial manifest. For CREATE events on templates
// that skipped the initial export, it emits the genesis manifest in draft state,
// ensuring every lifecycle chain is complete from creation forward.
func (s *Subscriber) appendTemplateC2PA(ctx context.Context, tplEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tplEvt.DID, err)
	}

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		if b, err := json.Marshal(tpl.TemplateData); err == nil {
			jsonldBytes = b
		}
	}

	state := tplEvt.NewState
	if state == "" {
		state = tpl.State
	}
	effectiveAt := tplEvt.OccurredAt
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}

	c2paState, err := c2pa.MapCWEStateToC2PAStrict(state)
	if err != nil {
		return fmt.Errorf("map template state %q to C2PA state: %w", state, err)
	}

	var cidStr, currentPdfC2PAState string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_c2pa_state,'') FROM contract_templates WHERE did=$1`,
		tplEvt.DID,
	).Scan(&cidStr, &currentPdfC2PAState)

	if currentPdfC2PAState == c2paState {
		return nil // Already embedded — skip.
	}

	var pdfBytes []byte
	if cidStr != "" {
		ipfsResult, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil || len(ipfsResult.Data) == 0 {
			return fmt.Errorf("fetch PDF from IPFS %s for template %s: %w", cidStr, tplEvt.DID, err)
		}
		pdfBytes = ipfsResult.Data
	} else {
		// No prior export — build the base PDF from the template data.
		name := ""
		if tpl.Name != nil {
			name = *tpl.Name
		}
		desc := ""
		if tpl.Description != nil {
			desc = *tpl.Description
		}
		docNum := ""
		if tpl.DocumentNumber != nil {
			docNum = *tpl.DocumentNumber
		}
		pdfBytes, err = builder.BuildTemplate(builder.TemplateInput{
			DID:            tpl.DID,
			State:          tpl.State,
			Version:        tpl.Version,
			Name:           name,
			Description:    desc,
			TemplateType:   tpl.TemplateType,
			DocumentNumber: docNum,
			CreatedBy:      tpl.CreatedBy,
			CreatedAt:      tpl.CreatedAt,
			UpdatedAt:      tpl.UpdatedAt,
			TemplateData:   jsonldBytes,
		})
		if err != nil {
			return fmt.Errorf("build template PDF for %s: %w", tplEvt.DID, err)
		}

		// If the current state is not "draft" the template was never captured at
		// creation. Prepend a synthetic draft genesis manifest so the chain
		// starts from the beginning of the lifecycle.
		if c2paState != "draft" {
			pdfBytes, err = s.appendOneTemplateManifest(ctx, tx, tplEvt.DID, "draft", jsonldBytes, pdfBytes, effectiveAt)
			if err != nil {
				return fmt.Errorf("append draft genesis manifest for template %s: %w", tplEvt.DID, err)
			}
		}
	}

	pdfBytes, err = s.appendOneTemplateManifest(ctx, tx, tplEvt.DID, state, jsonldBytes, pdfBytes, effectiveAt)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for template %s: %w", tplEvt.DID, err)
	}

	_ = pdfBytes // result already stored in IPFS and DB inside appendOneTemplateManifest

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit C2PA update for template %s: %w", tplEvt.DID, err)
	}

	log.Printf("pdfgeneration: updated PDF for template %s (state=%s)", tplEvt.DID, state)
	return nil
}

// appendOneTemplateManifest issues a VC, builds a C2PA manifest for the given
// state, appends it to pdfBytes, stores the result in IPFS, and updates
// pdf_ipfs_cid / pdf_c2pa_state in contract_templates within tx.
// It returns the updated PDF bytes.
func (s *Subscriber) appendOneTemplateManifest(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, effectiveAt time.Time,
) ([]byte, error) {
	var fromState string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_c2pa_state,'') FROM contract_templates WHERE did=$1`, did,
	).Scan(&fromState)

	fileHash := c2pa.FileHashOf(pdfBytes)
	pdfHash := fileHash
	prevHash := c2pa.PrevManifestHashFrom(pdfBytes)

	// If the PDF is freshly built (no embedded manifest) fall back to any
	// carry-forward chain link stored by a prior content-changing edit
	// (DCS-OR-C2PA-001 Gap E).
	if prevHash == "" {
		var stored string
		_ = tx.QueryRowContext(ctx,
			`SELECT COALESCE(prev_manifest_hash,'') FROM contract_templates WHERE did=$1`, did,
		).Scan(&stored)
		prevHash = stored
	}
	c2paState, err := c2pa.MapCWEStateToC2PAStrict(state)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", state, err)
	}

	var vcID string
	var vcBytes []byte
	if s.VCIssuer != nil {
		var err error
		vcID, vcBytes, err = s.VCIssuer.IssueContractLifecycleVC(
			ctx, did, fileHash, c2paState, "", s.IssuerDID, effectiveAt,
		)
		if err != nil {
			return nil, fmt.Errorf("issue lifecycle VC: %w", err)
		}
	}

	assertion := c2pa.NewLifecycleAssertion(
		did, fileHash, pdfHash, builder.RendererVersion,
		c2paState, "", s.IssuerDID, vcID, prevHash, effectiveAt,
	)

	result, err := c2pa.AppendManifest(ctx, s.Signer, s.TSACfg, s.IPFSClient, assertion, pdfBytes, vcBytes)
	if err != nil {
		return nil, fmt.Errorf("append C2PA manifest: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE contract_templates SET pdf_ipfs_cid = $1, pdf_renderer_version = $2, pdf_c2pa_state = $3, pdf_manifest_hash = $4, pdf_manifest_ipfs_cid = $5, prev_manifest_hash = NULL WHERE did = $6`,
		result.IPFSCID, builder.RendererVersion, c2paState, result.ManifestHash, result.ManifestIPFSCID, did,
	); err != nil {
		return nil, fmt.Errorf("update contract_templates pdf_ipfs_cid: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO c2pa_audit_log (entity_type, entity_did, from_state, to_state, actor_did, reason, vc_id, manifest_hash, occurred_at)
		 VALUES ('template',$1,$2,$3,$4,$5,$6,$7,$8)`,
		did, nullableString(fromState), c2paState, s.IssuerDID,
		reasonForC2PAState(c2paState), nullableString(vcID), result.ManifestHash, effectiveAt,
	); err != nil {
		return nil, fmt.Errorf("insert c2pa_audit_log for template %s: %w", did, err)
	}

	return result.UpdatedPDF, nil
}

func reasonForC2PAState(state string) string {
	switch state {
	case "draft":
		return "Contract created as draft"
	case "active":
		return "Contract activated for execution"
	case "amended":
		return "Contract amended with new terms"
	case "suspended":
		return "Contract suspended pending review"
	case "terminated":
		return "Contract terminated by parties"
	case "expired":
		return "Contract reached expiration date"
	case "replaced":
		return "Contract replaced with newer version"
	default:
		return "Contract state changed to: " + state
	}
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
