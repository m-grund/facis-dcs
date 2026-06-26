package dcstodcssynchronizer

import (
	"context"
	"encoding/json"
	"time"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/contractworkflowengine/remotesync"
	"digital-contracting-service/internal/middleware"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type queryTaskDataResult struct {
	approvalTasks        []*dcstodcs.DCSToDCSContractApprovalTaskItem
	reviewTasks          []*dcstodcs.DCSToDCSContractReviewTaskItem
	negotiationTasks     []*dcstodcs.DCSToDCSContractNegotiationTaskItem
	negotiations         []*dcstodcs.DCSToDCSContractNegotiationItem
	negotiationDecisions []*dcstodcs.DCSToDCSContractNegotiationDecisionItem
}

type DCSToDCSSynchronizer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	DIDDocument base.DIDDocument
}

func (s *DCSToDCSSynchronizer) StartSynchronizing(ctx context.Context, client *event.CloudEventSubClient) {
	eventHandler := func(evt cloudevent.Event) {

		source, err := componenttype.NewComponentType(evt.Source())
		if err != nil {
			log.Errorf(ctx, err, "failed to parse source component type, %s", evt.Source())
			return
		}

		evtType, err := eventtype.NewEventType(evt.Type())
		if err != nil {
			log.Errorf(ctx, err, "failed to parse source event type, %s", evt.Type())
			return
		}

		switch source {
		case componenttype.ContractWorkflowEngine:
			if evtType == eventtype.RemoteSync {
				return
			}

			if evtType == eventtype.RetrieveAll || evtType == eventtype.RetrieveByID || evtType == eventtype.RetrieveHistoryByDID {
				return
			}

			var data map[string]interface{}
			err := json.Unmarshal(evt.Data(), &data)
			if err != nil {
				log.Errorf(ctx, err, "failed to unmarshal event data, %s", evt.Data())
			}

			did, ok := data["did"]
			if !ok {
				log.Errorf(ctx, err, "could not read did")
				return
			}

			didString, ok := did.(string)
			if !ok {
				log.Errorf(ctx, err, "could not convert did")
			}

			err = s.doPeerSync(ctx, didString)
			if err != nil {
				log.Errorf(ctx, err, "failed to do peer sync, %s", evt.Data())
			}
		}
	}
	go func() {
		if err := client.Subscribe(eventHandler); err != nil {
			log.Errorf(ctx, err, "could not start event printer")
		}
	}()
}

func (s *DCSToDCSSynchronizer) readAllTasksData(ctx context.Context, did *string) (*queryTaskDataResult, error) {
	rtQry := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: middleware.GetParticipantID(ctx),
	}
	rtHandler := query.GetAllReviewTasksForDIDHandler{
		DB:     s.DB,
		RTRepo: s.RTRepo,
	}
	rtResult, err := rtHandler.Handle(ctx, rtQry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var reviewTasks []*dcstodcs.DCSToDCSContractReviewTaskItem
	for _, rt := range rtResult {
		reviewTasks = append(reviewTasks, &dcstodcs.DCSToDCSContractReviewTaskItem{
			ID:        rt.ID,
			Did:       rt.DID,
			State:     rt.State.String(),
			Reviewer:  rt.Reviewer,
			CreatedBy: rt.CreatedBy,
			CreatedAt: rt.CreatedAt.Format(time.RFC3339),
		})
	}

	atQry := query.GetAllApprovalTasksForDIDQry{
		DID:         *did,
		RetrievedBy: middleware.GetParticipantID(ctx),
	}
	atHandler := query.GetAllApprovalTasksForDIDHandler{
		DB:     s.DB,
		ATRepo: s.ATRepo,
	}
	atResult, err := atHandler.Handle(ctx, atQry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var approvalTasks []*dcstodcs.DCSToDCSContractApprovalTaskItem
	for _, at := range atResult {
		approvalTasks = append(approvalTasks, &dcstodcs.DCSToDCSContractApprovalTaskItem{
			ID:        at.ID,
			Did:       at.DID,
			State:     at.State.String(),
			Approver:  at.Approver,
			CreatedBy: at.CreatedBy,
			CreatedAt: at.CreatedAt.Format(time.RFC3339),
		})
	}

	nQry := remotesync.GetAllNegotiationsForDIDQry{
		DID:         *did,
		RetrievedBy: middleware.GetParticipantID(ctx),
	}
	nHandler := remotesync.GetAllNegotiationsForDIDHandler{
		DB:     s.DB,
		NRepo:  s.NRepo,
		NTRepo: s.NTRepo,
	}
	negotiationData, err := nHandler.Handle(ctx, nQry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var negotiationTasks []*dcstodcs.DCSToDCSContractNegotiationTaskItem
	for _, task := range negotiationData.NegotiationTasks {
		negotiationTasks = append(negotiationTasks, &dcstodcs.DCSToDCSContractNegotiationTaskItem{
			ID:         task.ID,
			Did:        task.DID,
			State:      task.State.String(),
			CreatedBy:  task.CreatedBy,
			CreatedAt:  task.CreatedAt.Format(time.RFC3339),
			Negotiator: task.Negotiator,
		})
	}

	var negotiations []*dcstodcs.DCSToDCSContractNegotiationItem
	for _, negotiation := range negotiationData.Negotiations {
		negotiations = append(negotiations, &dcstodcs.DCSToDCSContractNegotiationItem{
			ID:              negotiation.ID,
			Did:             negotiation.DID,
			ContractVersion: negotiation.ContractVersion,
			CreatedBy:       negotiation.CreatedBy,
			CreatedAt:       negotiation.CreatedAt.Format(time.RFC3339),
		})
	}

	var negotiationDecisions []*dcstodcs.DCSToDCSContractNegotiationDecisionItem
	for _, negotiationDecision := range negotiationData.NegotiationDecisions {

		var decision *string
		if negotiationDecision.Decision != nil {
			tmpDecision := negotiationDecision.Decision.String()
			decision = &tmpDecision
		}

		negotiationDecisions = append(negotiationDecisions, &dcstodcs.DCSToDCSContractNegotiationDecisionItem{
			ID:       negotiationDecision.ID,
			Decision: decision,
		})
	}

	return &queryTaskDataResult{
		reviewTasks:          reviewTasks,
		approvalTasks:        approvalTasks,
		negotiationTasks:     negotiationTasks,
		negotiations:         negotiations,
		negotiationDecisions: negotiationDecisions,
	}, nil
}

func (s *DCSToDCSSynchronizer) doPeerSync(ctx context.Context, did string) error {
	qry := contract.GetByIDQry{
		DID:         did,
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
		templaterepository.MakeInternalError(err)
	}

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

	contractItem := dcstodcs.DCSToDCSContractItem{
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
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: contractResult.ExpNoticePeriod,
		Responsible:     contractResult.Responsible,
		Origin:          contractResult.Origin,
	}

	result, err := s.readAllTasksData(ctx, &contractResult.DID)
	if err != nil {
		return templaterepository.MakeInternalError(err)
	}

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	responsibleList := contractResult.Responsible.GetUniqueResponsibleList()
	for _, responsible := range responsibleList {
		if responsible == origin {
			continue
		}

		hostname, err := base.DIDWebToHostname(responsible)
		if err != nil {
			return contractworkflowengine.MakeInternalError(err)
		}

		client := newDCSToDCSHttpClient(hostname)
		_, err = client.Sync(ctx, &dcstodcs.DCSToDCSContractSyncRequest{
			OriginDid:            origin,
			Contract:             &contractItem,
			ReviewTasks:          result.reviewTasks,
			ApprovalTasks:        result.approvalTasks,
			NegotiationTasks:     result.negotiationTasks,
			NegotiationItems:     result.negotiations,
			NegotiationDecisions: result.negotiationDecisions,
		})
		if err != nil {
			return contractworkflowengine.MakeInternalError(err)
		}
	}

	return nil
}
