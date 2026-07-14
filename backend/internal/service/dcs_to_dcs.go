package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/jades"

	trustedpeer "digital-contracting-service/internal/dcstodcs"

	command2 "digital-contracting-service/internal/contractworkflowengine/remotesync/command"

	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"

	"digital-contracting-service/internal/base/datatype/componenttype"

	"digital-contracting-service/internal/contractworkflowengine/command"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/contractworkflowengine/remotesync"

	negotiationdescision "digital-contracting-service/internal/contractworkflowengine/datatype/negotiationaction"

	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/contractworkflowengine/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
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
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, syncRepo db2.SyncRepository,
	trustPool *identity.EUTrustPool,
	didDocument identity.DIDDocument, ipfsClient *ipfs.APIClient) dcstodcs.Service {

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
	}
}

func (s *dcsToDcssrvc) GetSync(ctx context.Context, req *dcstodcs.DCSToDCSContractGetSyncRequest) (res *dcstodcs.DCSToDCSContractGetSyncResponse, err error) {

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command2.PeerUpdateRequestCmd{
		ContractDID: req.Did,
		FromPeerDID: localPeer,
	}
	handler := command2.PeerUpdateRequester{
		DB:          s.DB,
		CRepo:       s.CRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, dcstodcs.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractGetSyncResponse{
		FromPeerDid: localPeer,
	}, nil
}

func (s *dcsToDcssrvc) Action(ctx context.Context, req *dcstodcs.DCSToDCSContractActionRequest) (res *dcstodcs.DCSToDCSContractActionResponse, err error) {

	senderHostname, err := identity.DIDWebToHostname(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	remoteDIDDocument, err := identity.FetchDIDDocumentFromHostname(senderHostname)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	err = remoteDIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	err = remoteDIDDocument.Verify([]byte(req.SecretValue), req.SecretHash)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	// Third trust layer (see dcstodcs.CheckForUntrustedPeers doc): a
	// cryptographically and regulatorily valid peer identity still must be
	// explicitly listed in this node's trusted_peers allowlist before any
	// peer action is executed locally.
	untrustedPeers, err := trustedpeer.CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, []string{req.FromPeerDid})
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if len(untrustedPeers) > 0 {
		err := fmt.Errorf("peer action rejected: peer %s is not in the trusted_peers allowlist", req.FromPeerDid)
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	component, err := componenttype.NewComponentType(req.Component)
	if err != nil {
		return nil, dcstodcs.MakeInternalError(err)
	}

	if component != componenttype.ContractWorkflowEngine {
		err := fmt.Errorf("unsupported component type for remote action: %s", component)
		return nil, dcstodcs.MakeInternalError(err)
	}

	action, err := remoteaction.NewRemoteAction(req.Action)
	if err != nil {
		return nil, dcstodcs.MakeInternalError(err)
	}

	switch action {
	case remoteaction.PeerUpdate:
		cmd, err := base.ConvertAny[command2.PeerUpdateRequestCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command2.PeerUpdateRequester{
			DB:          s.DB,
			CRepo:       s.CRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.AcceptNegotiation:
		cmd, err := base.ConvertAny[command.AcceptNegotiationCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.NegotiationAcceptor{
			DB:          s.DB,
			CRepo:       s.CRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Approve:
		cmd, err := base.ConvertAny[command.ApproveCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Approver{
			DB:          s.DB,
			CRepo:       s.CRepo,
			ATRepo:      s.ATRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Reject:
		cmd, err := base.ConvertAny[command.RejectCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Rejecter{
			DB:          s.DB,
			CRepo:       s.CRepo,
			RTRepo:      s.RTRepo,
			ATRepo:      s.ATRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.RejectNegotiation:
		cmd, err := base.ConvertAny[command.RejectNegotiationCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.NegotiationRejector{
			DB:          s.DB,
			CRepo:       s.CRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Submit:
		cmd, err := base.ConvertAny[command.SubmitCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Submitter{
			DB:          s.DB,
			CRepo:       s.CRepo,
			RTRepo:      s.RTRepo,
			ATRepo:      s.ATRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Terminate:
		cmd, err := base.ConvertAny[command.TerminateCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Terminator{
			DB:          s.DB,
			CRepo:       s.CRepo,
			RTRepo:      s.RTRepo,
			ATRepo:      s.ATRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Update:
		err := fmt.Errorf("updates are just allowed contract's owner peer")
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Negotiate:
		cmd, err := base.ConvertAny[command.NegotiationCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Negotiator{
			DB:          s.DB,
			CRepo:       s.CRepo,
			RTRepo:      s.RTRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Offer:
		cmd, err := base.ConvertAny[command.OfferCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Offerer{
			DB:          s.DB,
			CRepo:       s.CRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case remoteaction.Withdraw:
		cmd, err := base.ConvertAny[command.WithdrawCmd](req.Payload)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		handler := command.Withdrawer{
			DB:          s.DB,
			CRepo:       s.CRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	default:
		log.Printf(ctx, "unsupported remote action: %s", req.Action)
	}

	return &dcstodcs.DCSToDCSContractActionResponse{
		FromPeerDid: localPeer,
	}, nil
}

func (s *dcsToDcssrvc) PostSync(ctx context.Context, req *dcstodcs.DCSToDCSContractPostSyncRequest) (res *dcstodcs.DCSToDCSContractPostSyncResponse, err error) {

	senderHostname, err := identity.DIDWebToHostname(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	remoteDIDDocument, err := identity.FetchDIDDocumentFromHostname(senderHostname)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	err = remoteDIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	err = remoteDIDDocument.Verify([]byte(req.SecretValue), req.SecretHash)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	if req.FromPeerDid == "" {
		return nil, contractworkflowengine.MakeInternalError(errors.New("origin did is empty"))
	}

	if req.FromPeerDid == localPeer {
		return nil, errors.New("syncing contract to same peer is not allowed")
	}

	// Third trust layer (see dcstodcs.CheckForUntrustedPeers doc): a
	// cryptographically and regulatorily valid peer identity still must be
	// explicitly listed in this node's trusted_peers allowlist before any
	// synced contract data is accepted and stored locally.
	untrustedPeers, err := trustedpeer.CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, []string{req.FromPeerDid})
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if len(untrustedPeers) > 0 {
		err := fmt.Errorf("post_sync rejected: peer %s is not in the trusted_peers allowlist", req.FromPeerDid)
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	// Fourth trust layer (DCS-FR-SM-02): the broadcast itself must carry a
	// JAdES signature by the SENDER over the canonical contract
	// representation. The challenge-response secret above only authenticates
	// the session — this binds the contract CONTENT to the sender's key and
	// leaves an independently verifiable artifact behind.
	jadesPayload, leafKey, err := jades.Verify(req.JadesSignature)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_sync rejected: %w", err))
	}
	peerKey := remoteDIDDocument.PublicKey()
	if peerKey == nil || leafKey.X.Cmp(peerKey.X) != 0 || leafKey.Y.Cmp(peerKey.Y) != 0 {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_sync rejected: JAdES x5c leaf key does not match peer %s's did:web key", req.FromPeerDid))
	}
	contractDocBytes, err := json.Marshal(req.Contract.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	expectedPayload, err := jades.BuildContractPayload(req.Contract.Did, req.Contract.ContractVersion, contractDocBytes)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if !bytes.Equal(jadesPayload, expectedPayload) {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_sync rejected: JAdES payload does not match the synced contract %s", req.Contract.Did))
	}

	createAt, err := time.Parse(time.RFC3339, req.Contract.CreatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	updatedAt, err := time.Parse(time.RFC3339, req.Contract.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contractData, err := datatype.NewJSON(req.Contract.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var startDate *time.Time
	if req.Contract.StartDate != nil {
		startD, err := time.Parse(time.RFC3339, *req.Contract.StartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		startDate = &startD
	}

	var expDate *time.Time
	if req.Contract.ExpDate != nil {
		expD, err := time.Parse(time.RFC3339, *req.Contract.ExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		expDate = &expD
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if req.Contract.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*req.Contract.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	responsible, err := db.ToResponsible(req.Contract.Responsible)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	state, err := contractstate.NewContractState(req.Contract.State)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	remoteContractData := remotesync.ContractData{
		DID:             req.Contract.Did,
		ContractData:    &contractData,
		Origin:          req.Contract.Origin,
		Responsible:     responsible,
		TemplateDID:     req.Contract.Did,
		CreatedBy:       req.Contract.CreatedBy,
		CreatedAt:       createAt,
		TemplateVersion: req.Contract.ContractVersion,
		State:           state,
		ContractVersion: req.Contract.ContractVersion,
		ExpPolicy:       expPolicy,
		ExpDate:         expDate,
		ExpNoticePeriod: req.Contract.ExpNoticePeriod,
		StartDate:       startDate,
		Name:            req.Contract.Name,
		Description:     req.Contract.Description,
		UpdatedAt:       updatedAt,
	}

	reviewTasks, err := toReviewTaskData(req.ReviewTasks)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	approvalTasks, err := toApprovalTaskData(req.ApprovalTasks)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	negotiationTasks, err := toNegotiationTaskData(req.NegotiationTasks)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	negotiations, err := toNegotiationData(req.NegotiationItems)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	negotiationDecision, err := toNegotiationDecisionData(req.NegotiationDecisions)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command2.LocalPeerUpdateCmd{
		FromPeerDID:          req.FromPeerDid,
		LocalPeer:            localPeer,
		ContractOrigin:       remoteContractData.Origin,
		Contract:             remoteContractData,
		ReviewTasks:          reviewTasks,
		ApprovalTasks:        approvalTasks,
		NegotiationTasks:     negotiationTasks,
		Negotiations:         negotiations,
		NegotiationDecisions: negotiationDecision,
		DIDDocument:          s.DIDDocument,
	}
	handler := command2.LocalPeerUpdater{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
		NRepo:  s.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	// Persist the verified JAdES artifact so the synced contract's
	// cross-instance provenance stays independently re-verifiable
	// (GET /peer/contracts/provenance).
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.SRepo.UpsertSyncSignature(ctx, tx, db2.SyncSignature{
		DID:             req.Contract.Did,
		ContractVersion: req.Contract.ContractVersion,
		FromPeerDID:     req.FromPeerDid,
		JadesSignature:  req.JadesSignature,
	}); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractPostSyncResponse{
		FromPeerDid: localPeer,
	}, nil
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

func toReviewTaskData(tasks []*dcstodcs.DCSToDCSContractReviewTaskItem) ([]remotesync.ReviewTaskData, error) {
	var reviewTasks []remotesync.ReviewTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remotesync.ReviewTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		reviewTasks = append(reviewTasks, remotesync.ReviewTaskData{
			ID:        task.ID,
			DID:       task.Did,
			CreatedBy: task.CreatedBy,
			CreatedAt: createAt,
			State:     task.State,
			Reviewer:  task.Reviewer,
		})
	}
	return reviewTasks, nil
}

func toApprovalTaskData(tasks []*dcstodcs.DCSToDCSContractApprovalTaskItem) ([]remotesync.ApprovalTaskData, error) {
	var approvalTasks []remotesync.ApprovalTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remotesync.ApprovalTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		approvalTasks = append(approvalTasks, remotesync.ApprovalTaskData{
			ID:        task.ID,
			DID:       task.Did,
			CreatedBy: task.CreatedBy,
			CreatedAt: createAt,
			State:     task.State,
			Approver:  task.Approver,
		})
	}
	return approvalTasks, nil
}

func toNegotiationTaskData(tasks []*dcstodcs.DCSToDCSContractNegotiationTaskItem) ([]remotesync.NegotiationTaskData, error) {
	var negotiationTasks []remotesync.NegotiationTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remotesync.NegotiationTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		state, err := negotiationtaskstate.NewNegotiationTaskState(task.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		negotiationTasks = append(negotiationTasks, remotesync.NegotiationTaskData{
			ID:         task.ID,
			DID:        task.Did,
			CreatedBy:  task.CreatedBy,
			CreatedAt:  createAt,
			State:      state,
			Negotiator: task.Negotiator,
		})
	}
	return negotiationTasks, nil
}

func toNegotiationData(tasks []*dcstodcs.DCSToDCSContractNegotiationItem) ([]remotesync.NegotiationData, error) {
	var negotiations []remotesync.NegotiationData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remotesync.NegotiationData{}, contractworkflowengine.MakeInternalError(err)
		}

		changeRequest, err := datatype.NewJSON(task.ChangeRequest)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		negotiations = append(negotiations, remotesync.NegotiationData{
			ID:              task.ID,
			DID:             task.Did,
			CreatedBy:       task.CreatedBy,
			CreatedAt:       createAt,
			ContractVersion: task.ContractVersion,
			ChangeRequest:   &changeRequest,
		})
	}
	return negotiations, nil
}

func toNegotiationDecisionData(tasks []*dcstodcs.DCSToDCSContractNegotiationDecisionItem) ([]remotesync.NegotiationDecisionData, error) {
	var negotiationDecisions []remotesync.NegotiationDecisionData
	for _, task := range tasks {

		var decision *negotiationdescision.NegotiationDecision
		if task.Decision != nil {
			tmpDecision, err := negotiationdescision.NewNegotiationDecision(*task.Decision)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			decision = &tmpDecision
		}

		negotiationDecisions = append(negotiationDecisions, remotesync.NegotiationDecisionData{
			ID:              task.ID,
			Decision:        decision,
			Negotiator:      task.Negotiator,
			NegotiationID:   task.NegotiationID,
			RejectionReason: task.RejectionReason,
		})
	}
	return negotiationDecisions, nil
}
