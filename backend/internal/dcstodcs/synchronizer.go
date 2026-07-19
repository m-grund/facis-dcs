// Package dcstodcs runs the DCS-to-DCS federation on the sending side: it
// listens for the PDF-regenerated event (and the signature-applied event),
// and ships the contract's PDF — the self-contained wire format carrying the
// machine-readable JSON-LD, the C2PA provenance chain, and any signatures — to
// the counterparty peer, resolving/verifying peer identity via did:web + eIDAS
// (trustedpeercheck.go, base/identity) and retrying failed ships from the
// sync_fails table. A signed contract additionally carries the JAdES
// (DCS-FR-SM-02). No contract state or task ledger crosses the boundary: each
// DCS runs its own workflow/RBAC (ADR-13).
package dcstodcs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/db"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type DCSToDCSSynchronizer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	SRepo       db2.SyncRepository
	IPFSClient  *ipfs.APIClient
	DIDDocument identity.DIDDocument
}

// shippableStates are the contract states whose PDF is shipped to the
// counterparty (ADR-13): a first offer, each negotiation counter, and the
// signed agreement. Internal states (DRAFT, SUBMITTED, REVIEWED, APPROVED,
// ACTIVE, TERMINATED) stay local — review/approval never cross the boundary.
var shippableStates = map[string]bool{
	contractstate.Offered.String():     true,
	contractstate.Negotiation.String(): true,
	contractstate.Signed.String():      true,
}

func (s *DCSToDCSSynchronizer) StartSynchronizerJob(ctx context.Context, client *event.CloudEventSubClient) {
	syncHandler := func(evt cloudevent.Event) {
		source, err := componenttype.NewComponentType(evt.Source())
		if err != nil {
			log.Errorf(ctx, err, "failed to parse source component type, %s", evt.Source())
			return
		}

		// A PDF is shipped when the regenerator has produced a fresh one
		// (PDF_REGENERATED, content changes: offer/negotiate) or when a
		// signature has been applied (APPLIED_SIGNATURE, which stores the
		// signed PDF directly). shipContractPDF gates on the shippable state.
		switch source {
		case componenttype.ContractWorkflowEngine:
			if evt.Type() != eventtype.PDFRegenerated.String() {
				return
			}
		case componenttype.SignatureManagement:
			smType, err := smeventtype.NewEventType(evt.Type())
			if err != nil || smType != smeventtype.Applied {
				return
			}
		default:
			return
		}

		did, err := didFromEvent(evt)
		if err != nil {
			log.Errorf(ctx, err, "could not read did from event %s", evt.Data())
			return
		}
		if err := s.shipContractPDF(ctx, did); err != nil {
			log.Errorf(ctx, err, "failed to ship contract PDF, %s", evt.Data())
		}
	}

	go func() {
		if err := client.Subscribe(syncHandler); err != nil {
			log.Errorf(ctx, err, "could not start syncHandler")
		}
	}()

	go s.startSyncFailScheduler(ctx, conf.SyncFailCronJobTimeOut())
}

func didFromEvent(evt cloudevent.Event) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(evt.Data(), &data); err != nil {
		return "", fmt.Errorf("unmarshal event data: %w", err)
	}
	did, ok := data["did"].(string)
	if !ok || did == "" {
		return "", errors.New("event carries no did")
	}
	return did, nil
}

func (s *DCSToDCSSynchronizer) startSyncFailScheduler(ctx context.Context, interval time.Duration) {
	readSyncFails := func() ([]db2.SyncFail, error) {
		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf(ctx, "could not rollback transaction: %v", err)
			}
		}(tx)

		attempts, err := s.SRepo.GetPendingSyncFails(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to read sync fail entries: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}
		return attempts, nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		log.Printf(ctx, "start retrying failed contract PDF ships")
		syncFails, err := readSyncFails()
		if err != nil {
			log.Printf(ctx, "could not read sync fails: %v", err)
			continue
		}
		for _, syncFail := range syncFails {
			if err := s.shipContractPDF(ctx, syncFail.DID); err != nil {
				log.Printf(ctx, "contract PDF ship retry was not successful: %v", err)
			}
		}
	}
}

// shipContractPDF fetches the contract's current PDF and ships it to every
// counterparty peer (ADR-13). A signed contract additionally carries the
// JAdES. A failed ship is recorded in sync_fails for later retry; a clean ship
// clears any prior failure. Non-shippable states and contracts without a PDF
// yet are no-ops.
func (s *DCSToDCSSynchronizer) shipContractPDF(ctx context.Context, did string) error {
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return err
	}

	readTx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	contractData, err := s.CRepo.ReadDataByDID(ctx, readTx, did)
	if err != nil {
		_ = readTx.Rollback()
		return fmt.Errorf("could not read contract %s: %w", did, err)
	}
	pdfState, err := s.CRepo.ReadPDFState(ctx, readTx, did)
	if err != nil {
		_ = readTx.Rollback()
		return fmt.Errorf("could not read PDF state for %s: %w", did, err)
	}
	_ = readTx.Rollback()

	state := string(contractData.State)
	if !shippableStates[state] || pdfState.IPFSCID == "" {
		return nil
	}

	recipients := contractData.Responsible.GetParties()
	untrustedPeers, err := CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, recipients)
	if err != nil {
		return err
	}

	pdfResult, err := s.IPFSClient.FetchFile(pdfState.IPFSCID)
	if err != nil || len(pdfResult.Data) == 0 {
		return fmt.Errorf("fetch PDF %s from IPFS for %s: %w", pdfState.IPFSCID, did, err)
	}
	pdfBytes := []byte(pdfResult.Data)

	jadesSignature, err := s.jadesForSignedContract(state, contractData)
	if err != nil {
		return err
	}

	shipError := s.shipToPeers(ctx, localPeer, did, pdfBytes, jadesSignature, recipients, untrustedPeers)
	return s.recordShipOutcome(ctx, did, shipError)
}

// jadesForSignedContract returns the JAdES a signed contract's ship carries, or
// an empty string for a proposal ship (ADR-13, DCS-FR-SM-02).
func (s *DCSToDCSSynchronizer) jadesForSignedContract(state string, contractData *db.Contract) (string, error) {
	if state != contractstate.Signed.String() {
		return "", nil
	}
	contractDocBytes := []byte(`{}`)
	if contractData.ContractData != nil && contractData.ContractData.IsNotNullValue() {
		contractDocBytes = []byte(*contractData.ContractData)
	}
	payload, err := jades.BuildContractPayload(contractData.DID, contractData.ContractVersion, contractDocBytes)
	if err != nil {
		return "", fmt.Errorf("build JAdES payload for %s: %w", contractData.DID, err)
	}
	signature, err := jades.Sign(&s.DIDDocument, payload)
	if err != nil {
		return "", fmt.Errorf("JAdES-sign %s: %w", contractData.DID, err)
	}
	return signature, nil
}

func (s *DCSToDCSSynchronizer) shipToPeers(ctx context.Context, localPeer, did string, pdfBytes []byte, jadesSignature string, recipients, untrustedPeers []string) error {
	for _, peer := range recipients {
		if peer == localPeer {
			continue
		}
		if slices.Contains(untrustedPeers, peer) {
			return fmt.Errorf("shipping to untrusted peer %s is not allowed", peer)
		}
		hostname, err := identity.DIDWebToHostname(peer)
		if err != nil {
			return err
		}
		secretValue := rand.Text()
		secretHash, err := s.DIDDocument.Sign([]byte(secretValue))
		if err != nil {
			return err
		}
		client := NewDCSToDCSHttpClient(hostname)
		if _, err := client.PostPdf(ctx, &dcstodcs.DCSToDCSContractPdfRequest{
			FromPeerDid:    localPeer,
			ContractIri:    did,
			Pdf:            pdfBytes,
			SecretValue:    secretValue,
			SecretHash:     secretHash,
			JadesSignature: &jadesSignature,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *DCSToDCSSynchronizer) recordShipOutcome(ctx context.Context, did string, shipError error) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	if shipError != nil {
		if err := s.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, did); err != nil {
			return fmt.Errorf("could not create or update sync fail entry: %w", err)
		}
	} else if err := s.SRepo.DeleteSyncFailEntry(ctx, tx, did); err != nil {
		return fmt.Errorf("could not delete sync fail entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return shipError
}
