// Package event subscribes to CWE lifecycle state-change events and appends
// a new C2PA manifest to the contract's stored PDF for each transition
// (DCS-OR-C2PA-001, DCS-OR-C2PA-003, DCS-OR-C2PA-008).
package event

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tplevttype "digital-contracting-service/internal/templaterepository/datatype/eventtype"
	tpldb "digital-contracting-service/internal/templaterepository/db"
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
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Fetch current contract state and JSON-LD.
	contract, err := s.CRepo.ReadDataByDID(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read contract %s: %w", cweEvt.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}

	state := cweEvt.NewState
	effectiveAt := cweEvt.OccurredAt

	// Map the raw CWE state to the SRS-defined C2PA vocabulary (DCS-OR-C2PA-003).
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return fmt.Errorf("map contract state %q to C2PA state: %w", state, err)
	}

	// Fetch the base PDF and check for idempotency.
	pdfState, err := s.CRepo.ReadPDFState(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for contract %s: %w", cweEvt.DID, err)
	}

	if pdfState.C2PAState == c2paState {
		// Already embedded by a concurrent export call — skip.
		return nil
	}

	if pdfState.IPFSCID == "" {
		return fmt.Errorf("no cached PDF for contract %s; export must be called before state-change events can chain", cweEvt.DID)
	}
	ipfsResult, err := s.IPFSClient.FetchFile(pdfState.IPFSCID)
	if err != nil || len(ipfsResult.Data) == 0 {
		return fmt.Errorf("fetch PDF from IPFS %s for contract %s: %w", pdfState.IPFSCID, cweEvt.DID, err)
	}
	existingPDF := []byte(ipfsResult.Data)

	// Compute asset hash for the VC credentialSubject (DCS-OR-C2PA-004).
	h := sha256.Sum256(existingPDF)
	fileHash := hex.EncodeToString(h[:])

	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, cweEvt.DID, fileHash, c2paState, cweEvt.Reason, s.IssuerDID, effectiveAt,
	)
	if err != nil {
		return fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, existingPDF, jsonldBytes, vcBytes, provenance.RemoteManifestURL(cweEvt.DID))
	if err != nil {
		return fmt.Errorf("pdf-core update for contract %s: %w", cweEvt.DID, err)
	}

	// Store updated PDF in IPFS.
	storeResult, err := s.IPFSClient.CreateFile(ctx, base64.StdEncoding.EncodeToString(updatedPDF))
	if err != nil {
		return fmt.Errorf("store updated PDF in IPFS for contract %s: %w", cweEvt.DID, err)
	}

	if err = s.CRepo.UpdatePDFState(ctx, tx, cweEvt.DID, cwedb.ContractPDFState{IPFSCID: storeResult.Identifier.Value, RendererVersion: rendererVersion, C2PAState: c2paState}); err != nil {
		return fmt.Errorf("update pdf_ipfs_cid for %s: %w", cweEvt.DID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf_ipfs_cid update for %s: %w", cweEvt.DID, err)
	}

	log.Printf("pdfgeneration: updated PDF for contract %s (state=%s) → IPFS CID %s", cweEvt.DID, state, storeResult.Identifier.Value)
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

	state := tplEvt.NewState
	effectiveAt := tplEvt.OccurredAt

	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return fmt.Errorf("map template state %q to C2PA state: %w", state, err)
	}

	tplPDFState, err := s.TRepo.ReadPDFState(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for template %s: %w", tplEvt.DID, err)
	}

	if tplPDFState.C2PAState == c2paState {
		return nil // Already embedded — skip.
	}

	var pdfBytes []byte
	if tplPDFState.IPFSCID != "" {
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

	pdfBytes, err = s.appendOneTemplateManifest(ctx, tx, tplEvt.DID, state, jsonldBytes, pdfBytes, effectiveAt)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for template %s: %w", tplEvt.DID, err)
	}
	_ = pdfBytes // result stored in IPFS and DB inside appendOneTemplateManifest

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit C2PA update for template %s: %w", tplEvt.DID, err)
	}

	log.Printf("pdfgeneration: updated PDF for template %s (state=%s)", tplEvt.DID, state)
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

	storeResult, err := s.IPFSClient.CreateFile(ctx, base64.StdEncoding.EncodeToString(updatedPDF))
	if err != nil {
		return nil, fmt.Errorf("store updated PDF in IPFS for template %s: %w", did, err)
	}

	if err := s.TRepo.UpdatePDFState(ctx, tx, did, tpldb.ContractTemplatePDFState{IPFSCID: storeResult.Identifier.Value, RendererVersion: rendererVersion, C2PAState: c2paState}); err != nil {
		return nil, fmt.Errorf("update contract_templates pdf_ipfs_cid: %w", err)
	}

	return updatedPDF, nil
}
