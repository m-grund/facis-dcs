package dcstodcssynchronizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	db2 "digital-contracting-service/internal/dcstodcssynchronizer/db"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
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

type QueryTaskDataResult struct {
	ApprovalTasks        []*dcstodcs.DCSToDCSContractApprovalTaskItem
	ReviewTasks          []*dcstodcs.DCSToDCSContractReviewTaskItem
	NegotiationTasks     []*dcstodcs.DCSToDCSContractNegotiationTaskItem
	Negotiations         []*dcstodcs.DCSToDCSContractNegotiationItem
	NegotiationDecisions []*dcstodcs.DCSToDCSContractNegotiationDecisionItem
}

type DCSToDCSSynchronizer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	SRepo       db2.SyncRepository
	DIDDocument base.DIDDocument
}

func (s *DCSToDCSSynchronizer) StartSynchronizerJob(ctx context.Context, client *event.CloudEventSubClient) {
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
			if evtType == eventtype.RetrieveAll || evtType == eventtype.RetrieveByID || evtType == eventtype.RetrieveHistoryByDID {
				return
			}

			if evtType == eventtype.RemoteUpdateRequest || evtType == eventtype.RemoteSyncRequest {
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

func ReadAllTasksData(ctx context.Context, db *sqlx.DB,
	cRepo db.ContractRepo,
	rtRepo db.ReviewTaskRepo,
	atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo,
	nRepo db.NegotiationRepo,
	did *string) (*QueryTaskDataResult, error) {

	rtQry := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: middleware.GetParticipantID(ctx),
	}
	rtHandler := query.GetAllReviewTasksForDIDHandler{
		DB:     db,
		RTRepo: rtRepo,
	}
	rtResult, err := rtHandler.Handle(ctx, rtQry)
	if err != nil {
		return nil, err
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
		DB:     db,
		ATRepo: atRepo,
	}
	atResult, err := atHandler.Handle(ctx, atQry)
	if err != nil {
		return nil, err
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
		DB:     db,
		NRepo:  nRepo,
		NTRepo: ntRepo,
	}
	negotiationData, err := nHandler.Handle(ctx, nQry)
	if err != nil {
		return nil, err
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
			ChangeRequest:   negotiation.ChangeRequest,
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
			ID:              negotiationDecision.ID,
			Decision:        decision,
			Negotiator:      negotiationDecision.Negotiator,
			NegotiationID:   negotiationDecision.NegotiationID,
			RejectionReason: negotiationDecision.RejectionReason,
		})
	}

	return &QueryTaskDataResult{
		ReviewTasks:          reviewTasks,
		ApprovalTasks:        approvalTasks,
		NegotiationTasks:     negotiationTasks,
		Negotiations:         negotiations,
		NegotiationDecisions: negotiationDecisions,
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

		return err
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

	result, err := ReadAllTasksData(ctx, s.DB, s.CRepo, s.RTRepo, s.ATRepo, s.NTRepo, s.NRepo, &contractResult.DID)
	if err != nil {
		return err
	}

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return err
	}

	responsibleList := contractResult.Responsible.GetUniqueResponsibleList()

	untrustedPeers, err := s.checkForUntrustedPeers(ctx, origin, responsibleList)
	if err != nil {
		return err
	}

	for _, responsible := range responsibleList {
		if responsible == origin {
			continue
		}

		if slices.Contains(untrustedPeers, responsible) {
			log.Printf(ctx, "synchronization to untrusted peer %s is not allowed", responsible)
			continue
		}

		hostname, err := base.DIDWebToHostname(responsible)
		if err != nil {
			return err
		}

		client := NewDCSToDCSHttpClient(hostname)

		time.Sleep(time.Second * 3)

		_, remoteSyncErr := client.Sync(ctx, &dcstodcs.DCSToDCSContractSyncRequest{
			OriginDid:            origin,
			Contract:             &contractItem,
			ReviewTasks:          result.ReviewTasks,
			ApprovalTasks:        result.ApprovalTasks,
			NegotiationTasks:     result.NegotiationTasks,
			NegotiationItems:     result.Negotiations,
			NegotiationDecisions: result.NegotiationDecisions,
		})

		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf(ctx, "could not rollback transaction: %v", err)
			}
		}(tx)

		if remoteSyncErr != nil {

			err = s.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, responsible)
			if err != nil {
				return fmt.Errorf("could not create or update sync fail entry: %w", err)
			}

		} else {

			err = s.SRepo.DeleteSyncFailEntry(ctx, tx, responsible)
			if err != nil {
				return fmt.Errorf("could not create or update sync fail entry: %w", err)
			}

		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}
	}

	return nil
}

func (s *DCSToDCSSynchronizer) checkForUntrustedPeers(ctx context.Context, origin string, responsible []string) ([]string, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	var untrustedPeers []string
	for _, peer := range responsible {
		if peer == origin {
			continue
		}

		trusted, err := s.SRepo.IsTrustedPeer(ctx, tx, peer)
		if err != nil {
			return nil, fmt.Errorf("could not check trusted peer: %w", err)
		}

		if !trusted {
			untrustedPeers = append(untrustedPeers, peer)
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return untrustedPeers, nil
}
