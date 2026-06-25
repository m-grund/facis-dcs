package service

import (
	"context"
	"errors"
	"time"

	negotiationdescision "digital-contracting-service/internal/contractworkflowengine/datatype/negotiationaction"

	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"

	"digital-contracting-service/internal/contractworkflowengine/datatype/remote"

	"digital-contracting-service/internal/contractworkflowengine/command"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/contractworkflowengine/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"

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
	DIDDocument base.DIDDocument
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo,
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
		DIDDocument:      didDocument,
	}
}

func (s *dcsToDcssrvc) Create(ctx context.Context, req *dcstodcs.DCSToDCSContractCreateRequest) (res *dcstodcs.DCSToDCSContractCreateResponse, err error) {

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	if req.Contract.Origin == origin {
		return nil, errors.New("could not create contract on same peer")
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

	remoteContractData := remote.ContractData{
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

	cmd := command.RemoteCreateCmd{
		Contract:             remoteContractData,
		ReviewTasks:          reviewTasks,
		ApprovalTasks:        approvalTasks,
		NegotiationTasks:     negotiationTasks,
		Negotiations:         negotiations,
		NegotiationDecisions: negotiationDecision,
	}
	handler := command.RemoteCreator{
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

	return &dcstodcs.DCSToDCSContractCreateResponse{
		Did: req.Contract.Did,
	}, nil
}

func (s *dcsToDcssrvc) Update(ctx context.Context, req *dcstodcs.DCSToDCSContractUpdateRequest) (res *dcstodcs.DCSToDCSContractUpdateResponse, err error) {

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	if req.Contract.Origin == origin {
		return nil, errors.New("could not create contract on same peer")
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

	remoteContractData := remote.ContractData{
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

	cmd := command.RemoteUpdateCmd{
		Contract:             remoteContractData,
		ReviewTasks:          reviewTasks,
		ApprovalTasks:        approvalTasks,
		NegotiationTasks:     negotiationTasks,
		Negotiations:         negotiations,
		NegotiationDecisions: negotiationDecision,
	}
	handler := command.RemoteUpdater{
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

	return &dcstodcs.DCSToDCSContractUpdateResponse{
		Did: req.Contract.Did,
	}, nil

}

func (s *dcsToDcssrvc) Status(ctx context.Context, req *dcstodcs.DCSToDCSContractStatusRequest) (res *dcstodcs.DCSToDCSContractStatusResponse, err error) {
	return &dcstodcs.DCSToDCSContractStatusResponse{}, nil
}

func toReviewTaskData(tasks []*dcstodcs.DCSToDCSContractReviewTaskItem) ([]remote.ReviewTaskData, error) {
	var reviewTasks []remote.ReviewTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remote.ReviewTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		reviewTasks = append(reviewTasks, remote.ReviewTaskData{
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

func toApprovalTaskData(tasks []*dcstodcs.DCSToDCSContractApprovalTaskItem) ([]remote.ApprovalTaskData, error) {
	var approvalTasks []remote.ApprovalTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remote.ApprovalTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		approvalTasks = append(approvalTasks, remote.ApprovalTaskData{
			ID:        task.ID,
			DID:       task.Did,
			CreatedBy: task.CreatedBy,
			CreatedAt: createAt,
			State:     task.State,
		})
	}
	return approvalTasks, nil
}

func toNegotiationTaskData(tasks []*dcstodcs.DCSToDCSContractNegotiationTaskItem) ([]remote.NegotiationTaskData, error) {
	var negotiationTasks []remote.NegotiationTaskData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remote.NegotiationTaskData{}, contractworkflowengine.MakeInternalError(err)
		}

		state, err := negotiationtaskstate.NewNegotiationTaskState(task.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		negotiationTasks = append(negotiationTasks, remote.NegotiationTaskData{
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

func toNegotiationData(tasks []*dcstodcs.DCSToDCSContractNegotiationItem) ([]remote.NegotiationData, error) {
	var negotiations []remote.NegotiationData
	for _, task := range tasks {

		createAt, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			return []remote.NegotiationData{}, contractworkflowengine.MakeInternalError(err)
		}

		changeRequest, err := datatype.NewJSON(task.ChangeRequest)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		negotiations = append(negotiations, remote.NegotiationData{
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

func toNegotiationDecisionData(tasks []*dcstodcs.DCSToDCSContractNegotiationDecisionItem) ([]remote.NegotiationDecisionData, error) {
	var negotiationDecisions []remote.NegotiationDecisionData
	for _, task := range tasks {

		var decision *negotiationdescision.NegotiationDecision
		if task.Decision != nil {
			tmpDecision, err := negotiationdescision.NewNegotiationDecision(*task.Decision)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			decision = &tmpDecision
		}

		negotiationDecisions = append(negotiationDecisions, remote.NegotiationDecisionData{
			ID:              task.ID,
			Decision:        decision,
			Negotiator:      task.Negotiator,
			NegotiationID:   task.NegotiationID,
			RejectionReason: task.RejectionReason,
		})
	}
	return negotiationDecisions, nil
}
