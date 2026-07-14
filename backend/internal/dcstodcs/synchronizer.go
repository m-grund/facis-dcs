// Package dcstodcs is the transport/orchestration side of DCS-to-DCS
// federation: it broadcasts a contract's full state (this file's
// DCSToDCSSynchronizer, triggered off the local NATS event bus) to every
// responsible peer via PostSync, resolves/verifies peer identity via
// did:web + eIDAS (trustedpeercheck.go, base/identity), and retries failed
// syncs from a persistent queue. The commands that actually get executed on
// the receiving side (PeerUpdateRequester, LocalPeerUpdater) deliberately
// live in contractworkflowengine/remotesync/command instead of here, since
// that's the domain the mutated data belongs to — this package only moves
// bytes and enforces trust, it does not itself decide contract state.
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

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"

	"digital-contracting-service/internal/base/conf"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
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
	DIDDocument identity.DIDDocument
}

func (s *DCSToDCSSynchronizer) StartSynchronizerJob(ctx context.Context, client *event.CloudEventSubClient) {

	syncHandler := func(evt cloudevent.Event) {

		source, err := componenttype.NewComponentType(evt.Source())
		if err != nil {
			log.Errorf(ctx, err, "failed to parse source component type, %s", evt.Source())
			return
		}

		switch source {
		case componenttype.ContractWorkflowEngine:
			evtType, err := eventtype.NewEventType(evt.Type())
			if err != nil {
				log.Errorf(ctx, err, "failed to parse contract workflow event type, %s", evt.Type())
				return
			}

			if evtType == eventtype.RetrieveAll || evtType == eventtype.RetrieveByID ||
				evtType == eventtype.AccessDenied || evtType == eventtype.RetrieveHistoryByDID {
				return
			}

			// This is really important to avoid synchronization loops
			if evtType == eventtype.RemoteSyncRequest || evtType == eventtype.RemoteActionRequestEvent {
				return
			}

			var data map[string]interface{}
			err = json.Unmarshal(evt.Data(), &data)
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

			err = s.doContractPeerSync(ctx, didString)
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

		attempts, err := s.SRepo.GetPendingSyncFails(ctx, tx)
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

		err = s.doContractPeerSync(ctx, attempt.DID)
		if err != nil {
			return err
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
		log.Printf(ctx, "start retrying failed sync attempts")

		syncFails, err := readSyncFails()
		if err != nil {
			log.Printf(ctx, "could not read sync fails: %v", err)
			continue
		}

		for _, syncFail := range syncFails {
			if err := syncFailHandler(syncFail, peerDID); err != nil {
				log.Printf(ctx, "synchronization was not successful: %v", err)
			}
		}
	}
}

func ReadAllContractTasksData(ctx context.Context, db *sqlx.DB,
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

// doContractPeerSync pushes the contract's full current state (all fields
// plus every review/approval/negotiation task of every responsible peer,
// not just the ones the local peer owns) to each other responsible peer via
// PostSync, in Responsible list order. Any peer not present in the local
// trusted-peer allowlist (see CheckForUntrustedPeers) aborts the remaining
// delivery for this attempt as soon as it's reached — peers earlier in the
// list have already received the update, but the whole attempt is still
// recorded as failed (sync_fails), since the failure is tracked per
// contract, not per peer; startSyncFailScheduler retries the full broadcast
// later.
func (s *DCSToDCSSynchronizer) doContractPeerSync(ctx context.Context, did string) error {

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return err
	}

	qry := contract.GetByIDQry{
		DID:         did,
		RetrievedBy: "System",
		HolderDID:   localPeer,
		UserRoles:   nil,
		Internal:    true,
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

	result, err := ReadAllContractTasksData(ctx, s.DB, s.RTRepo, s.ATRepo, s.NTRepo, s.NRepo, &contractResult.DID)
	if err != nil {
		return err
	}

	responsibleList := contractResult.Responsible.GetUniqueResponsibleList()
	untrustedPeers, err := CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, responsibleList)
	if err != nil {
		return err
	}

	// JAdES-sign the canonical contract representation (DCS-FR-SM-02): every
	// broadcast binds the contract content to this instance's HSM-backed key;
	// the receiving peer verifies signature, key binding, and payload before
	// accepting. A signing failure fails the whole sync (retry queue) — the
	// broadcast must never go out unsigned.
	contractDocBytes := []byte(`{}`)
	if contractResult.ContractData != nil && contractResult.ContractData.IsNotNullValue() {
		contractDocBytes = []byte(*contractResult.ContractData)
	}
	jadesPayload, err := jades.BuildContractPayload(contractResult.DID, contractResult.ContractVersion, contractDocBytes)
	if err != nil {
		return fmt.Errorf("could not build JAdES payload for %s: %w", contractResult.DID, err)
	}
	jadesSignature, err := jades.Sign(&s.DIDDocument, jadesPayload)
	if err != nil {
		return fmt.Errorf("could not JAdES-sign contract %s for peer broadcast: %w", contractResult.DID, err)
	}

	handleSync := func() error {
		for _, responsible := range responsibleList {
			if responsible == localPeer {
				continue
			}
			if slices.Contains(untrustedPeers, responsible) {
				return fmt.Errorf("synchronization to untrusted peer %s is not allowed", responsible)
			}

			hostname, err := identity.DIDWebToHostname(responsible)
			if err != nil {
				return err
			}

			secretValue := rand.Text()
			secretHash, err := s.DIDDocument.Sign([]byte(secretValue))
			if err != nil {
				return err
			}

			client := NewDCSToDCSHttpClient(hostname)
			_, err = client.PostSync(ctx, &dcstodcs.DCSToDCSContractPostSyncRequest{
				FromPeerDid:          localPeer,
				Contract:             &contractItem,
				ReviewTasks:          result.ReviewTasks,
				ApprovalTasks:        result.ApprovalTasks,
				NegotiationTasks:     result.NegotiationTasks,
				NegotiationItems:     result.Negotiations,
				NegotiationDecisions: result.NegotiationDecisions,
				SecretHash:           secretHash,
				SecretValue:          secretValue,
				JadesSignature:       jadesSignature,
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	syncError := handleSync()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	if syncError != nil {
		if err := s.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, contractItem.Did); err != nil {
			return fmt.Errorf("could not create or update sync fail entry: %w", err)
		}
	} else {
		if err := s.SRepo.DeleteSyncFailEntry(ctx, tx, contractItem.Did); err != nil {
			return fmt.Errorf("could not delete sync fail entry: %w", err)
		}
	}

	return tx.Commit()
}
