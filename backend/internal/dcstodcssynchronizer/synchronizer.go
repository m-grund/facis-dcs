package dcstodcssynchronizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"digital-contracting-service/internal/contractworkflowengine/conf"

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
	syncHandler := func(evt cloudevent.Event) {

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

			// This is really important to avoid synchronization loops
			if evtType == eventtype.RemoteSyncRequest || evtType == eventtype.RemoteActionRequestEvent {
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
				return
			}

			err = s.doPeerSync(ctx, didString)
			if err != nil {
				log.Errorf(ctx, err, "failed to do peer sync, %s", evt.Data())
			}
		}
	}
	go func() {
		if err := client.Subscribe(syncHandler); err != nil {
			log.Errorf(ctx, err, "could not start syncHandler")
		}
	}()

	go s.startSyncFailScheduler(ctx, conf.SyncFailCronJobTimeOut())
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

		attempts, err := s.SRepo.ReadAllSyncFailEntries(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to read sync fail entries: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return attempts, nil
	}

	syncFailHandler := func(attempt db2.SyncFail, peerDID string) error {

		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf(ctx, "could not rollback transaction: %v", err)
			}
		}(tx)

		processData, err := s.CRepo.ReadProcessDataByDID(ctx, tx, attempt.DID)
		if err != nil {
			return fmt.Errorf("could not read contract: %w", err)
		}

		evt := contractevents.OutdatedPeerEvent{
			DID:        attempt.DID,
			OccurredAt: time.Now().UTC(),
			Origin:     processData.Origin,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
		if err != nil {
			return fmt.Errorf("could not create event: %w", err)
		}

		return tx.Commit()
	}

	peerDID, err := s.DIDDocument.GetID()
	if err != nil {
		log.Errorf(ctx, err, "failed to get DID document")
		return
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {

		syncFails, err := readSyncFails()
		if err != nil {
			log.Printf(ctx, "could not read sync fails: %v", err)
			continue
		}

		var successful []string
		for _, syncFail := range syncFails {
			err = syncFailHandler(syncFail, peerDID)
			if err != nil {
				log.Printf(ctx, "synchronization was not successful: %v", err)
				continue
			}

			successful = append(successful, syncFail.DID)
		}

		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			log.Printf(ctx, "could not start transaction: %v", err)
			continue
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf(ctx, "could not rollback transaction: %v", err)
			}
		}(tx)

		for _, entry := range successful {
			err := s.SRepo.DeleteSyncFailEntry(ctx, tx, entry)
			if err != nil {
				log.Printf(ctx, "could not delete sync fail entry: %v", err)
			}
		}

		err = tx.Commit()
		if err != nil {
			log.Printf(ctx, "could not commit transaction: %v", err)
		}
	}
}

func ReadAllTasksData(ctx context.Context, db *sqlx.DB,
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

	result, err := ReadAllTasksData(ctx, s.DB, s.RTRepo, s.ATRepo, s.NTRepo, s.NRepo, &contractResult.DID)
	if err != nil {
		return err
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return err
	}

	responsibleList := contractResult.Responsible.GetUniqueResponsibleList()
	untrustedPeers, err := remotesync.CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, responsibleList)
	if err != nil {
		return err
	}

	for _, responsible := range responsibleList {
		if responsible == localPeer {
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

		_, remoteSyncErr := client.Sync(ctx, &dcstodcs.DCSToDCSContractSyncRequest{
			FromPeerDid:          localPeer,
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

			err = s.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, contractItem.Did)
			if err != nil {
				return fmt.Errorf("could not create or update sync fail entry: %w", err)
			}

		} else {

			err = s.SRepo.DeleteSyncFailEntry(ctx, tx, contractItem.Did)
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
