package service

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	contracttemplate2 "digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationactionflag"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/middleware"
	qry2 "digital-contracting-service/internal/processauditandcompliance/query"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"

	"github.com/jmoiron/sqlx"
)

type contractWorkflowEnginesrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	RTRepo       db.ReviewTaskRepo
	ATRepo       db.ApprovalTaskRepo
	NTRepo       db.NegotiationTaskRepo
	NRepo        db.NegotiationRepo
	CTRepo       db.ContractTemplateRepo
	FCClient     *fcclient.FederatedCatalogueClient
	DIDDocument  base.DIDDocument
	ATrailReader base.AuditTrailReader
	auth.JWTAuthenticator
}

func NewContractWorkflowEngine(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo,
	fcClient *fcclient.FederatedCatalogueClient, auditTrailReader base.AuditTrailReader, didDocument base.DIDDocument) contractworkflowengine.Service {

	return &contractWorkflowEnginesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		CTRepo:           ctRepo,
		FCClient:         fcClient,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
	}
}

func (s *contractWorkflowEnginesrvc) Create(ctx context.Context, req *contractworkflowengine.ContractCreateRequest) (res *contractworkflowengine.ContractCreateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	did, err := base.GenerateID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.CreateCmd{
		DID:         *did,
		DIDDocument: s.DIDDocument,
		TemplateDID: req.Did,
		CreatedBy:   middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		Reviewers:   req.Reviewers,
		Approvers:   req.Approvers,
		Negotiators: req.Negotiators,
	}
	createHandler := command.Creator{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractCreateResponse{
		Did: *did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Update(ctx context.Context, req *contractworkflowengine.ContractUpdateRequest) (res *contractworkflowengine.ContractUpdateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	metaData, err := datatype.NewJSON(req.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var startDate *time.Time
	if req.StartDate != nil {
		startD, err := time.Parse(time.RFC3339, *req.StartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		startDate = &startD
	}

	var expDate *time.Time
	if req.ExpDate != nil {
		expD, err := time.Parse(time.RFC3339, *req.ExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		expDate = &expD
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if req.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*req.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	cmd := command.UpdateCmd{
		DID:             req.Did,
		UpdatedAt:       updatedAt,
		UpdatedBy:       middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
		UserRoles:       middleware.GetUserRoles(ctx),
		Name:            req.Name,
		Description:     req.Description,
		ContractData:    &metaData,
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: req.ExpNoticePeriod,
	}
	handler := command.Updater{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractUpdateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Submit(ctx context.Context, req *contractworkflowengine.ContractSubmitRequest) (res *contractworkflowengine.ContractSubmitResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var actionFlag *actionflag.ActionFlag
	if req.ForwardTo != nil {
		flag, err := actionflag.NewActionFlag(*req.ForwardTo)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		actionFlag = &flag
	}

	cmd := command.SubmitCmd{
		DID:         req.Did,
		DIDDocument: s.DIDDocument,
		UpdatedAt:   updatedAt,
		SubmittedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		ActionFlag:  actionFlag,
		Comments:    req.Comments,
	}
	handler := command.Submitter{
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NRepo:  s.NRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	qry := contract.GetProcessDataByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
	}
	qryHandler := contract.GetProcessDataByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	processData, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractSubmitResponse{
		Did:          req.Did,
		CurrentState: processData.State.String(),
	}, nil
}

func (s *contractWorkflowEnginesrvc) Retrieve(ctx context.Context, req *contractworkflowengine.ContractRetrieveRequest) (res *contractworkflowengine.ContractRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contract.GetAllMetadataQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		Pagination:  pagination,
	}
	qryHandler := contract.GetAllMetadataHandler{
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractItem
	for _, item := range result.Contracts {

		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &contractworkflowengine.ContractItem{
			Did:                  item.DID,
			ContractVersion:      item.ContractVersion,
			State:                item.State.String(),
			Name:                 item.Name,
			Description:          item.Description,
			CreatedBy:            item.CreatedBy,
			CreatedAt:            item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:            item.UpdatedAt.Format(time.RFC3339),
			TemplateDid:          item.TemplateDID,
			TemplateVersion:      item.TemplateVersion,
			StartDate:            startDate,
			ExpDate:              expDate,
			ExpPolicy:            expPolicy,
			ExpNoticePeriod:      item.ExpNoticePeriod,
			Responsible:          item.Responsible,
			LatestTemplateDid:    item.LatestTemplateDID,
			TemplateIsDeprecated: item.TemplateIsDeprecated,
		})
	}

	var reviewTasks []*contractworkflowengine.ContractReviewTaskItem
	for _, item := range result.ReviewerTasks {
		reviewTasks = append(reviewTasks, &contractworkflowengine.ContractReviewTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			Reviewer:        item.Reviewer,
			State:           item.State.String(),
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	var approvalTasks []*contractworkflowengine.ContractApprovalTaskItem
	for _, item := range result.ApprovalTasks {
		approvalTasks = append(approvalTasks, &contractworkflowengine.ContractApprovalTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Approver:        item.Approver,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	var negotiationTasks []*contractworkflowengine.ContractNegotiationTaskItem
	for _, item := range result.NegotiatorTasks {
		negotiationTasks = append(negotiationTasks, &contractworkflowengine.ContractNegotiationTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Negotiator:      item.Negotiator,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	return &contractworkflowengine.ContractRetrieveResponse{
		Contracts:        contracts,
		ReviewTasks:      reviewTasks,
		ApprovalTasks:    approvalTasks,
		NegotiationTasks: negotiationTasks,
	}, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveByID(ctx context.Context, req *contractworkflowengine.ContractRetrieveByIDRequest) (res *contractworkflowengine.ContractRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	qryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
		NRepo: s.NRepo,
	}
	contractResult, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	negotiations := make(map[string]*contractworkflowengine.ContractNegotiationItem)
	for _, item := range contractResult.Negotiations {
		negotiation, ok := negotiations[item.ID]
		if !ok {
			negotiation = &contractworkflowengine.ContractNegotiationItem{
				ID:              item.ID,
				ContractVersion: item.ContractVersion,
				ChangeRequest:   item.ChangeRequest,
				CreatedBy:       item.CreatedBy,
				CreatedAt:       item.CreatedAt.String(),
			}
			negotiations[item.ID] = negotiation
		}

		negotiation.NegotiationDecisions = append(negotiation.NegotiationDecisions, &contractworkflowengine.ContractNegotiationDecisionItem{
			Negotiator:      item.Negotiator,
			Decision:        item.Decision,
			RejectionReason: item.RejectionReason,
		})
	}

	negotiationList := slices.Collect(maps.Values(negotiations))

	var startDate *string
	if contractResult.StartDate != nil {
		s := contractResult.StartDate.Format(time.RFC3339)
		startDate = &s
	}

	var expDate *string
	if contractResult.ExpDate != nil {
		s := contractResult.ExpDate.Format(time.RFC3339)
		expDate = &s
	}

	var expPolicy *string
	if contractResult.ExpPolicy != nil {
		s := contractResult.ExpPolicy.String()
		expPolicy = &s
	}

	return &contractworkflowengine.ContractRetrieveByIDResponse{
		Did:             contractResult.DID,
		ContractVersion: contractResult.ContractVersion,
		State:           contractResult.State.String(),
		Name:            contractResult.Name,
		Description:     contractResult.Description,
		CreatedBy:       contractResult.CreatedBy,
		CreatedAt:       contractResult.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       contractResult.UpdatedAt.Format(time.RFC3339),
		ContractData:    contractResult.ContractData,
		TemplateDid:     contractResult.TemplateDID,
		TemplateVersion: contractResult.TemplateVersion,
		Negotiations:    negotiationList,
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: contractResult.ExpNoticePeriod,
		Responsible:     contractResult.Responsible,
	}, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveHistoryByID(ctx context.Context, req *contractworkflowengine.ContractHistoryRetrieveByIDRequest) (res []*contractworkflowengine.ContractHistoryRetrieveByIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetHistoryByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	qryHandler := contract.GetHistoryByIDHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractHistoryRetrieveByIDResponse
	for _, item := range result {

		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &contractworkflowengine.ContractHistoryRetrieveByIDResponse{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			TemplateDid:     item.TemplateDID,
			TemplateVersion: item.TemplateVersion,
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
			ContractData:    item.ContractData,
		})
	}

	return contracts, nil
}

func (s *contractWorkflowEnginesrvc) Negotiate(ctx context.Context, req *contractworkflowengine.ContractNegotiationRequest) (res *contractworkflowengine.ContractNegotiationResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	changeRequest, err := datatype.NewJSON(req.ChangeRequest)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.NegotiationCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		NegotiatedBy:  middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		ChangeRequest: &changeRequest,
		UserRoles:     middleware.GetUserRoles(ctx),
	}
	handler := command.Negotiator{
		DB:     s.DB,
		CRepo:  s.CRepo,
		NRepo:  s.NRepo,
		RTRepo: s.RTRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractNegotiationResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Respond(ctx context.Context, req *contractworkflowengine.ContractNegotiationRespondRequest) (res *contractworkflowengine.ContractNegotiationRespondResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	actionFlag, err := negotiationactionflag.NewNegotiationActionFlag(req.ActionFlag)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(fmt.Errorf("unknown action flag: %s", req.ActionFlag))
	}

	switch actionFlag {
	case negotiationactionflag.Accepting:
		cmd := command.AcceptNegotiationCmd{
			ID:         req.ID,
			DID:        req.Did,
			AcceptedBy: middleware.GetParticipantID(ctx),
			UserRoles:  middleware.GetUserRoles(ctx),
		}
		handler := command.NegotiationAcceptor{
			DB:     s.DB,
			CRepo:  s.CRepo,
			NRepo:  s.NRepo,
			NTRepo: s.NTRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	case negotiationactionflag.Rejecting:
		cmd := command.RejectNegotiationCmd{
			ID:              req.ID,
			DID:             req.Did,
			RejectedBy:      middleware.GetParticipantID(ctx),
			UserRoles:       middleware.GetUserRoles(ctx),
			RejectionReason: req.RejectionReason,
		}
		handler := command.NegotiationRejector{
			DB:     s.DB,
			CRepo:  s.CRepo,
			NRepo:  s.NRepo,
			NTRepo: s.NTRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	}

	return &contractworkflowengine.ContractNegotiationRespondResponse{
		ID: req.ID,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Review(ctx context.Context, req *contractworkflowengine.ContractReviewRequest) (res *contractworkflowengine.ContractReviewResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	cmd := command.ReviewCmd{
		DID:        req.Did,
		ReviewedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := command.Reviewer{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractReviewResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Search(ctx context.Context, req *contractworkflowengine.ContractSearchRequest) (res []*contractworkflowengine.ContractSearchResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contractstate.ContractState
	if req.State != nil {
		tState, err := contractstate.NewContractState(*req.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		state = &tState
	}

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contract.GetAllMetadataByFilterQry{
		DID:             base.DerefString(req.Did),
		ContractVersion: base.DerefInt(req.ContractVersion),
		State:           state,
		RetrievedBy:     middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
		UserRoles:       middleware.GetUserRoles(ctx),
		Name:            base.DerefString(req.Name),
		Description:     base.DerefString(req.Description),
		ContractData:    base.DerefString(req.ContractData),
		Pagination:      pagination,
	}
	qryHandler := contract.GetAllMetaDataByFilterHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractSearchResponse
	for _, item := range result {

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &contractworkflowengine.ContractSearchResponse{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
		})
	}

	return contracts, nil
}

func (s *contractWorkflowEnginesrvc) Approve(ctx context.Context, req *contractworkflowengine.ContractApproveRequest) (res *contractworkflowengine.ContractApproveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.ApproveCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		ApprovedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := command.Approver{
		DB:     s.DB,
		CRepo:  s.CRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractApproveResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Reject(ctx context.Context, req *contractworkflowengine.ContractRejectRequest) (res *contractworkflowengine.ContractRejectResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.RejectCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		RejectedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		Reason:     req.Reason,
	}
	handler := command.Rejecter{
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractRejectResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Store(ctx context.Context, req *contractworkflowengine.ContractStoreRequest) (res *contractworkflowengine.ContractStoreResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.RecordEvidenceCmd{
		DID:        req.Did,
		RecordedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		UpdatedAt:  updatedAt,
	}
	handler := command.EvidenceRecorder{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractStoreResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Terminate(ctx context.Context, req *contractworkflowengine.ContractTerminateRequest) (res *contractworkflowengine.ContractTerminateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.TerminateCmd{
		DID:          req.Did,
		UpdatedAt:    updatedAt,
		TerminatedBy: middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		Reason:       req.Reason,
	}
	handler := command.Terminator{
		DB:     s.DB,
		CRepo:  s.CRepo,
		NRepo:  s.NRepo,
		NTRepo: s.NTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractTerminateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Audit(ctx context.Context, req *contractworkflowengine.ContractAuditRequest) (res []*contractworkflowengine.ContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := qry2.GetAuditLogByDIDQry{
		DID:       req.Did,
		Scope:     componenttype.ContractWorkflowEngine,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	handler := qry2.AuditLogByDIDAuditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	auditLogHistory, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	history := make([]*contractworkflowengine.ContractAuditResponse, 0)
	for _, entry := range auditLogHistory {
		if !base.IsAuditVisibleEventType(entry.EventType) {
			continue
		}
		history = append(history, &contractworkflowengine.ContractAuditResponse{
			ID:               entry.ID,
			Component:        entry.Component,
			EventType:        entry.EventType,
			EventData:        entry.EventData,
			Did:              entry.DID,
			CreatedAt:        entry.CreatedAt.String(),
			GlobalLogPredCid: entry.GlobalLogPredCID,
			ResLogPredCid:    entry.ResLogPredCID,
		})
	}

	return history, nil
}

// retrieve templates
func (s *contractWorkflowEnginesrvc) RetrieveTemplates(ctx context.Context, req *contractworkflowengine.ApprovedContractTemplateRetrieveRequest) (res []*contractworkflowengine.ApprovedContractTemplateRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate2.GetAllApprovedTemplatesQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := contracttemplate2.GetAllApprovedTemplateHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*contractworkflowengine.ApprovedContractTemplateRetrieveResponse
	for _, item := range result {
		contractTemplates = append(contractTemplates, &contractworkflowengine.ApprovedContractTemplateRetrieveResponse{
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			TemplateType:   item.TemplateType.String(),
			Name:           item.Name,
			Description:    item.Description,
			CreatedBy:      item.CreatedBy,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
			Responsible:    item.Responsible,
		})
	}

	return contractTemplates, nil
}
