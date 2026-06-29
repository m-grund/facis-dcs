package service

import (
	"context"
	"errors"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/remotesync/remoteaction"

	"digital-contracting-service/internal/contractworkflowengine/command"

	db2 "digital-contracting-service/internal/dcstodcssynchronizer/db"

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
	DIDDocument base.DIDDocument
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, syncRepo db2.SyncRepository,
	didDocument base.DIDDocument) dcstodcs.Service {
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
	}
}

func (s *dcsToDcssrvc) Action(ctx context.Context, req *dcstodcs.DCSToDCSContractActionRequest) (res *dcstodcs.DCSToDCSContractActionResponse, err error) {

	switch req.Action {
	case "update":
		cmd, err := remoteaction.ConvertAny[command.UpdateCmd](req.Payload)
		handler := command.Updater{
			DB:          s.DB,
			CRepo:       s.CRepo,
			RTRepo:      s.RTRepo,
			ATRepo:      s.ATRepo,
			NTRepo:      s.NTRepo,
			NRepo:       s.NRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	case "submit":
		cmd, err := remoteaction.ConvertAny[command.SubmitCmd](req.Payload)
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
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

		err = handler.Handle(ctx, *cmd)
		if err != nil {
			return nil, dcstodcs.MakeInternalError(err)
		}

	default:
		log.Printf(ctx, "unknown action: %s", req.Action)
	}

	return &dcstodcs.DCSToDCSContractActionResponse{
		Did: "",
	}, nil
}

func (s *dcsToDcssrvc) Sync(ctx context.Context, req *dcstodcs.DCSToDCSContractSyncRequest) (res *dcstodcs.DCSToDCSContractSyncResponse, err error) {

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

	cmd := remotesync.PeerSyncCmd{
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
	handler := remotesync.PeerSynchronizer{
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

	return &dcstodcs.DCSToDCSContractSyncResponse{
		Did: req.Contract.Did,
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
