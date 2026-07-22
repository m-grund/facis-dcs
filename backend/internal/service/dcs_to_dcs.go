package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"goa.design/clue/log"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"

	trustedpeer "digital-contracting-service/internal/dcstodcs"

	"digital-contracting-service/internal/contractworkflowengine/command"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/contractworkflowengine/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"

	"github.com/jmoiron/sqlx"
)

type dcsToDcssrvc struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	CTRepo      db.ContractTemplateRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
	TrustPool   *identity.EUTrustPool
	IPFSClient  *ipfs.APIClient
	PDFCore     *pdfcore.Client
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, syncRepo db2.SyncRepository,
	trustPool *identity.EUTrustPool,
	didDocument identity.DIDDocument, ipfsClient *ipfs.APIClient, pdfCore *pdfcore.Client) dcstodcs.Service {

	return &dcsToDcssrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		CTRepo:           ctRepo,
		SRepo:            syncRepo,
		DIDDocument:      didDocument,
		TrustPool:        trustPool,
		IPFSClient:       ipfsClient,
		PDFCore:          pdfCore,
	}
}

// PostPdf receives a contract PDF a counterparty shipped (ADR-13). It
// authenticates the peer (the same layers post_sync applied), asks pdf-core to
// extract the embedded JSON-LD, and upserts this instance's own local copy of
// the contract. No tasks cross the boundary — each DCS runs its own workflow.
func (s *dcsToDcssrvc) PostPdf(ctx context.Context, req *dcstodcs.DCSToDCSContractPdfRequest) (res *dcstodcs.DCSToDCSContractPdfResponse, err error) {
	senderHostname, err := identity.DIDWebToHostname(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	remoteDIDDocument, err := identity.FetchDIDDocumentFromHostname(senderHostname)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := remoteDIDDocument.VerifyEIDASCertificate(s.TrustPool); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}
	if err := remoteDIDDocument.Verify([]byte(req.SecretValue), req.SecretHash); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if req.FromPeerDid == localPeer {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("shipping a contract PDF to the same peer is not allowed"))
	}
	untrustedPeers, err := trustedpeer.CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, []string{req.FromPeerDid})
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if len(untrustedPeers) > 0 {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: peer %s is not in the trusted_peers allowlist", req.FromPeerDid))
	}

	// Legal gate: the received PDF's human-readable page content MUST be the
	// deterministic re-render of its own embedded machine-readable payload, or
	// the two forms of the contract have diverged and we refuse it. pdf-core
	// /verify/content compares only the page content streams, so the C2PA,
	// signature and amendment layers a peer legitimately appended do not trip
	// it; genuine tampering does.
	contentMatch, mismatchDetail, verr := s.PDFCore.VerifyContent(ctx, req.Pdf)
	if verr != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: could not content-verify received PDF: %w", verr))
	}
	if !contentMatch {
		// Diagnostic (rejection path only; the fatal gate is unchanged): surface
		// WHICH page diverged + a snippet of both renders, plus the embedded
		// payload's len+hash, so the exact human↔machine divergence is visible in
		// the peer's log.
		embedded, _ := s.PDFCore.ExtractPayload(ctx, req.Pdf)
		esum := sha256.Sum256(embedded)
		log.Printf(ctx, "post_pdf VerifyContent mismatch for %s: %s | embedded payload len=%d sha256=%s",
			req.ContractIri, mismatchDetail, len(embedded), hex.EncodeToString(esum[:8]))
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf(
			"post_pdf rejected: received PDF's human-readable page content does not match its embedded machine-readable payload: %s", mismatchDetail))
	}

	payload, err := s.PDFCore.ExtractPayload(ctx, req.Pdf)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: could not extract contract payload from PDF: %w", err))
	}

	receiver := command.PeerPdfReceiver{DB: s.DB, CRepo: s.CRepo, RTRepo: s.RTRepo, ATRepo: s.ATRepo, NTRepo: s.NTRepo, IPFSClient: s.IPFSClient}
	if err := receiver.Handle(ctx, command.PeerPdfReceiveCmd{
		ContractIRI:  req.ContractIri,
		Counterparty: req.FromPeerDid,
		LocalPeer:    localPeer,
		Payload:      payload,
		Pdf:          req.Pdf,
	}); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractPdfResponse{FromPeerDid: localPeer}, nil
}

// GetProvenance returns the stored JAdES provenance artifact for a contract
// this instance received from a peer (DCS-FR-SM-02).
func (s *dcsToDcssrvc) GetProvenance(ctx context.Context, p *dcstodcs.GetProvenancePayload) (res *dcstodcs.DCSToDCSSyncProvenanceResponse, err error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	sig, err := s.SRepo.GetSyncSignature(ctx, tx, p.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if sig == nil {
		return nil, dcstodcs.MakeNotFound(fmt.Errorf("no sync provenance stored for contract %s", p.Did))
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSSyncProvenanceResponse{
		Did:             sig.DID,
		ContractVersion: sig.ContractVersion,
		FromPeerDid:     sig.FromPeerDID,
		JadesSignature:  sig.JadesSignature,
		ReceivedAt:      sig.ReceivedAt.UTC().Format(time.RFC3339),
	}, nil
}
