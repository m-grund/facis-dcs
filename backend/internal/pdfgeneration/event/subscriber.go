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
)

// lifecycleEventTypes is the set of CWE event types that represent a
// contract state change and therefore require a new C2PA assertion.
var lifecycleEventTypes = map[string]bool{
	cweeventtype.Create.String():                  true,
	cweeventtype.Submit.String():                  true,
	cweeventtype.Approve.String():                 true,
	cweeventtype.Reject.String():                  true,
	cweeventtype.Terminate.String():               true,
	cweeventtype.ContractExpired.String():         true,
	cweeventtype.Negotiation.String():             true,
	cweeventtype.IncreaseContractVersion.String(): true,
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
	if !lifecycleEventTypes[evt.Type()] {
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
		return nil // non-contract events (e.g. RetrieveAll) have no DID
	}

	return s.appendC2PA(ctx, cweEvt)
}

func (s *Subscriber) appendC2PA(ctx context.Context, cweEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Fetch current contract state and JSON-LD data.
	contract, err := s.CRepo.ReadDataByID(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read contract %s: %w", cweEvt.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		b, err := json.Marshal(contract.ContractData)
		if err != nil {
			return fmt.Errorf("marshal contract data: %w", err)
		}
		jsonldBytes = b
	}

	// Fetch or generate the base PDF.
	var existingPDF []byte
	var cidStr string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid, '') FROM contracts WHERE did = $1`, cweEvt.DID,
	).Scan(&cidStr)
	if cidStr != "" {
		ipfsResult, err := s.IPFSClient.FetchFile(cidStr)
		if err != nil {
			log.Printf("pdfgeneration: could not fetch PDF from IPFS CID %s: %v — regenerating", cidStr, err)
		} else if len(ipfsResult.Data) > 0 {
			existingPDF = []byte(ipfsResult.Data)
		}
	}
	if len(existingPDF) == 0 {
		// Generate base PDF from JSON-LD for the first time.
		name := ""
		if contract.Name != nil {
			name = *contract.Name
		}
		desc := ""
		if contract.Description != nil {
			desc = *contract.Description
		}
		existingPDF, err = builder.BuildContract(builder.ContractInput{
			DID:          contract.DID,
			State:        contract.State,
			Version:      contract.ContractVersion,
			Name:         name,
			Description:  desc,
			CreatedBy:    contract.CreatedBy,
			CreatedAt:    contract.CreatedAt,
			UpdatedAt:    contract.UpdatedAt,
			ContractData: jsonldBytes,
		})
		if err != nil {
			return fmt.Errorf("build base PDF for %s: %w", cweEvt.DID, err)
		}
	}

	// Build lifecycle assertion.
	fileHash := c2pa.FileHashOf(jsonldBytes)
	prevHash := c2pa.PrevManifestHashFrom(existingPDF)
	state := cweEvt.NewState
	if state == "" {
		state = contract.State
	}
	effectiveAt := cweEvt.OccurredAt
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}

	// Map the raw CWE state to the SRS-defined C2PA vocabulary (DCS-OR-C2PA-003).
	c2paState := c2pa.MapCWEStateToC2PA(state)

	pdfHash := c2pa.BasePDFHashOf(existingPDF)

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
	result, err := c2pa.AppendManifest(ctx, s.Signer, s.TSACfg, s.IPFSClient, s.IssuerDID, assertion, existingPDF, vcBytes)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for %s: %w", cweEvt.DID, err)
	}

	// Update pdf_ipfs_cid and pdf_renderer_version in the DB.
	_, err = tx.ExecContext(ctx,
		`UPDATE contracts SET pdf_ipfs_cid = $1, pdf_renderer_version = $2 WHERE did = $3`,
		result.IPFSCID, builder.RendererVersion, cweEvt.DID,
	)
	if err != nil {
		return fmt.Errorf("update pdf_ipfs_cid for %s: %w", cweEvt.DID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf_ipfs_cid update for %s: %w", cweEvt.DID, err)
	}

	log.Printf("pdfgeneration: updated PDF for contract %s (state=%s) → IPFS CID %s", cweEvt.DID, state, result.IPFSCID)
	return nil
}
