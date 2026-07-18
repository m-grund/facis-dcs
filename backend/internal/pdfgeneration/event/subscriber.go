// Package event subscribes to CWE lifecycle state-change events and appends
// a new C2PA manifest to the contract's stored PDF for each transition
// (DCS-OR-C2PA-001, DCS-OR-C2PA-003, DCS-OR-C2PA-008).
package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tplevttype "digital-contracting-service/internal/templaterepository/datatype/eventtype"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

// contractLifecycleEventTypes is the set of CWE event types that change a
// contract's rendered content or lifecycle state and therefore require the PDF
// to be regenerated in the background — including Update (a content edit), so
// the exported PDF is never generated on demand.
var contractLifecycleEventTypes = map[string]bool{
	cweeventtype.Create.String():                  true,
	cweeventtype.Update.String():                  true,
	cweeventtype.Submit.String():                  true,
	cweeventtype.Approve.String():                 true,
	cweeventtype.Reject.String():                  true,
	cweeventtype.Terminate.String():               true,
	cweeventtype.ContractExpired.String():         true,
	cweeventtype.Negotiation.String():             true,
	cweeventtype.IncreaseContractVersion.String(): true,
}

// templateLifecycleEventTypes is the set of template repository event types
// that change a template's content or state and require background PDF
// regeneration — including Update (a content edit).
var templateLifecycleEventTypes = map[string]bool{
	tplevttype.Create.String():   true,
	tplevttype.Update.String():   true,
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
	CRepo      cwedb.ContractRepo
	TRepo      tpldb.ContractTemplateRepo
	PDFCore    *pdfcore.Client
	IssuerDID  string
	// VCIssuer issues and signs a W3C VC for each lifecycle event (DCS-OR-C2PA-004/005).
	VCIssuer provenance.VCIssuer
}

// Start registers the event handler with the NATS sub-client and begins
// consuming events. It returns immediately; the subscription runs in the
// background until the sub-client is closed.
func (s *Subscriber) Start(subClient *event.CloudEventSubClient) error {
	return subClient.Subscribe(func(evt cloudevent.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		// The regenerator has no user JWT; present the in-cluster system
		// credential so pdf-core can reach the internal signing primitives.
		ctx = middleware.InjectBearerToken(ctx, conf.SystemToken())
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

	// The outbox publisher passes the domain event straight through as
	// json.RawMessage (cloudeventprovider.go: marshalling a RawMessage is the
	// identity), so the CloudEvent data IS the domain event object.
	var cweEvt minimalCWEEvent
	if err := json.Unmarshal(evt.Data(), &cweEvt); err != nil {
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
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Serialize regeneration per contract IRI: concurrent lifecycle events for
	// the same contract queue on this lock instead of racing the read-modify-
	// write of the PDF state (which would double-render and could fork the C2PA
	// chain). Released on tx commit/rollback.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", cweEvt.DID); err != nil {
		return fmt.Errorf("acquire per-contract PDF regeneration lock for %s: %w", cweEvt.DID, err)
	}

	// Fetch current contract state and JSON-LD.
	contract, err := s.CRepo.ReadDataByDID(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read contract %s: %w", cweEvt.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}
	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	// Map the contract's committed state to the SRS-defined C2PA vocabulary
	// (DCS-OR-C2PA-003). The record's state is the source of truth: the genesis
	// CreateEvent carries no new_state, and the event is emitted only after the
	// transition commits, so the record always reflects the state the PDF must
	// assert.
	c2paState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	pdfState, err := s.CRepo.ReadPDFState(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for contract %s: %w", cweEvt.DID, err)
	}

	// A frozen PDF is a PAdES-signed artifact (DCS-FR-SM-16): the signing
	// command already produced the final signed bytes and stored them, and any
	// post-signing C2PA lifecycle update runs through the explicit signing/
	// revoke endpoints — never this background regenerator. Re-rendering here
	// would replace the signed PDF with an unsigned one and destroy the
	// signature's /ByteRange, so leave a frozen artifact untouched.
	if pdfState.IPFSCID != "" && provenance.IsFrozenC2PAState(pdfState.C2PAState) {
		return nil
	}

	contentChanged := pdfState.PayloadHash != currentPayloadHash
	stateChanged := pdfState.C2PAState != c2paState
	if pdfState.IPFSCID != "" && !contentChanged && !stateChanged {
		return nil // already up to date — idempotent re-delivery
	}

	// A pure state transition appends to the existing PDF to preserve the C2PA
	// chain; the genesis render or a content change (a DRAFT edit) starts from a
	// freshly rendered PDF that reflects the current content.
	var basePDF []byte
	if pdfState.IPFSCID != "" && !contentChanged {
		ipfsResult, err := s.IPFSClient.FetchFile(pdfState.IPFSCID)
		if err != nil || len(ipfsResult.Data) == 0 {
			return fmt.Errorf("fetch PDF from IPFS %s for contract %s: %w", pdfState.IPFSCID, cweEvt.DID, err)
		}
		basePDF = ipfsResult.Data
	} else {
		basePDF, _, err = s.PDFCore.Download(ctx, jsonldBytes)
		if err != nil {
			return fmt.Errorf("pdf-core render for contract %s: %w", cweEvt.DID, err)
		}
	}

	// Compute asset hash for the VC credentialSubject (DCS-OR-C2PA-004).
	h := sha256.Sum256(basePDF)
	fileHash := hex.EncodeToString(h[:])

	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, cweEvt.DID, fileHash, c2paState, cweEvt.Reason, s.IssuerDID, cweEvt.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, basePDF, jsonldBytes, vcBytes, provenance.RemoteManifestURL(cweEvt.DID))
	if err != nil {
		return fmt.Errorf("pdf-core update for contract %s: %w", cweEvt.DID, err)
	}

	// Store updated PDF in IPFS. CreateFile must receive the raw PDF bytes, not
	// a pre-base64-encoded string: passed a string, it JSON-marshals the value
	// (wrapping it in an extra quoted layer) instead of using it as the raw
	// upload body, so a later plain FetchFile (export/verify) would decode back
	// a JSON-string literal rather than the PDF (the same raw-bytes contract
	// query/appendAndCache and signingmanagement/apply.go's CreateFile calls use).
	storeResult, err := s.IPFSClient.CreateFile(ctx, updatedPDF)
	if err != nil {
		return fmt.Errorf("store updated PDF in IPFS for contract %s: %w", cweEvt.DID, err)
	}

	if err = s.CRepo.UpdatePDFState(ctx, tx, cweEvt.DID, cwedb.ContractPDFState{IPFSCID: storeResult.Identifier.Value, RendererVersion: rendererVersion, C2PAState: c2paState, PayloadHash: currentPayloadHash}); err != nil {
		return fmt.Errorf("update pdf_ipfs_cid for %s: %w", cweEvt.DID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf_ipfs_cid update for %s: %w", cweEvt.DID, err)
	}

	log.Printf("pdfgeneration: regenerated PDF for contract %s (state=%s, contentChanged=%t) → IPFS CID %s", cweEvt.DID, contract.State, contentChanged, storeResult.Identifier.Value)
	return nil
}

// appendTemplateC2PA appends a C2PA lifecycle assertion to a contract template's
// stored PDF in response to a template state-change event (DCS-OR-C2PA-003).
func (s *Subscriber) appendTemplateC2PA(ctx context.Context, tplEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tplEvt.DID, err)
	}

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		jsonldBytes = []byte(*tpl.TemplateData)
	}

	// The template record's state is the source of truth (the genesis CreateEvent
	// carries no new_state); the event is emitted only after the transition commits.
	c2paState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
	}

	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	tplPDFState, err := s.TRepo.ReadPDFState(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for template %s: %w", tplEvt.DID, err)
	}

	contentChanged := tplPDFState.PayloadHash != currentPayloadHash
	stateChanged := tplPDFState.C2PAState != c2paState
	if tplPDFState.IPFSCID != "" && !contentChanged && !stateChanged {
		return nil // already up to date
	}

	// State transition appends to preserve the chain; genesis or a content edit
	// renders fresh from the current content.
	var pdfBytes []byte
	if tplPDFState.IPFSCID != "" && !contentChanged {
		ipfsResult, err := s.IPFSClient.FetchFile(tplPDFState.IPFSCID)
		if err != nil || len(ipfsResult.Data) == 0 {
			return fmt.Errorf("fetch PDF from IPFS %s for template %s: %w", tplPDFState.IPFSCID, tplEvt.DID, err)
		}
		pdfBytes = ipfsResult.Data
	} else {
		pdfBytes, _, err = s.PDFCore.Download(ctx, jsonldBytes)
		if err != nil {
			return fmt.Errorf("pdf-core download for template %s: %w", tplEvt.DID, err)
		}
	}

	pdfBytes, err = s.appendOneTemplateManifest(ctx, tx, tplEvt.DID, tpl.State, jsonldBytes, pdfBytes, tplEvt.OccurredAt)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for template %s: %w", tplEvt.DID, err)
	}
	_ = pdfBytes // result stored in IPFS and DB inside appendOneTemplateManifest

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit C2PA update for template %s: %w", tplEvt.DID, err)
	}

	log.Printf("pdfgeneration: regenerated PDF for template %s (state=%s, contentChanged=%t)", tplEvt.DID, tpl.State, contentChanged)
	return nil
}

// appendOneTemplateManifest issues a VC, calls pdf-core /update, stores the result
// in IPFS, and updates pdf_ipfs_cid / pdf_c2pa_state in contract_templates within tx.
// It returns the updated PDF bytes.
func (s *Subscriber) appendOneTemplateManifest(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, effectiveAt time.Time,
) ([]byte, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", state, err)
	}

	h := sha256.Sum256(pdfBytes)
	fileHash := hex.EncodeToString(h[:])
	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, did, fileHash, c2paState, "", s.IssuerDID, effectiveAt,
	)
	if err != nil {
		return nil, fmt.Errorf("issue lifecycle VC: %w", err)
	}

	// pdf-core appends C2PA incremental update with VC attachment.
	// vcBytes being non-nil bypasses the "no-changes" guard for genesis VC attachment.
	// Templates have no public /c2pa/manifest/{contract_did} endpoint, so no
	// remote_manifests reference is embedded for the template PDF path.
	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, "")
	if err != nil {
		return nil, fmt.Errorf("pdf-core update for template %s: %w", did, err)
	}

	// See appendC2PA: CreateFile must receive raw bytes, not a base64 string.
	storeResult, err := s.IPFSClient.CreateFile(ctx, updatedPDF)
	if err != nil {
		return nil, fmt.Errorf("store updated PDF in IPFS for template %s: %w", did, err)
	}

	if err := s.TRepo.UpdatePDFState(ctx, tx, did, tpldb.ContractTemplatePDFState{IPFSCID: storeResult.Identifier.Value, RendererVersion: rendererVersion, C2PAState: c2paState, PayloadHash: currentPayloadHash}); err != nil {
		return nil, fmt.Errorf("update contract_templates pdf_ipfs_cid: %w", err)
	}

	return updatedPDF, nil
}
