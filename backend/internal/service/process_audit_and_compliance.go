package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"

	qry2 "digital-contracting-service/internal/processauditandcompliance/query"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type processAuditAndCompliancesrvc struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
	CTRepo       templatedb.ContractTemplateRepo
	CRepo        cwedb.ContractRepo
	auth.JWTAuthenticator
}

func NewProcessAuditAndCompliance(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, auditTrailReader base.AuditTrailReader, ctRepo templatedb.ContractTemplateRepo, cRepo cwedb.ContractRepo) processauditandcompliance.Service {
	return &processAuditAndCompliancesrvc{DB: db, JWTAuthenticator: jwtAuth, ATrailReader: auditTrailReader, CTRepo: ctRepo, CRepo: cRepo}
}

func auditScopeToComponentType(scope string) (componenttype.ComponentType, error) {
	switch strings.ToLower(scope) {
	case "templates":
		return componenttype.ContractTemplateRepo, nil
	case "contracts":
		return componenttype.ContractWorkflowEngine, nil
	case "signatures":
		return componenttype.SignatureManagement, nil
	case "archive":
		return componenttype.ContractStorageArchive, nil
	default:
		return "", fmt.Errorf("invalid audit scope: %s", scope)
	}
}

func (s *processAuditAndCompliancesrvc) Audit(ctx context.Context, req *processauditandcompliance.PACAuditRequest) (res []*processauditandcompliance.PACAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	scope, err := auditScopeToComponentType(req.Scope)
	if err != nil {
		return nil, processauditandcompliance.MakeBadRequest(err)
	}

	if isStaticContractAudit(req) {
		if scope != componenttype.ContractWorkflowEngine {
			return nil, processauditandcompliance.MakeBadRequest(fmt.Errorf("static contract audits require scope %q", "contracts"))
		}

		auditStaticContentQry := qry2.GetStaticContentAuditQry{
			DID:             req.ContractDid,
			RetrievedBy:     middleware.GetParticipantID(ctx),
			HolderDID:       middleware.GetHolderDID(ctx),
			UserRoles:       middleware.GetUserRoles(ctx),
			Policy:          req.Policy,
			PolicyVersion:   req.PolicyVersion,
			ContractVersion: req.ContractVersion,
		}
		auditStaticContentAuditor := qry2.StaticContentAuditor{}
		entries, err := auditStaticContentAuditor.Handle(ctx, auditStaticContentQry)
		if err != nil {
			return nil, processauditandcompliance.MakeBadRequest(err)
		}

		result := []*processauditandcompliance.PACResourceAuditTrailEntry{}
		for _, entry := range entries {
			result = append(result, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:               entry.ID,
				Component:        entry.Component,
				EventType:        entry.EventType,
				EventData:        entry.EventData,
				Did:              entry.DID,
				CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}

		return []*processauditandcompliance.PACAuditResponse{
			{
				Component:  componenttype.ContractWorkflowEngine.String(),
				Did:        base.DerefString(req.ContractDid),
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
				AuditTrail: result,
			},
		}, nil
	}

	qry := qry2.GetAuditLogQry{
		Scope:     scope,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	handler := qry2.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	resLogHistories, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	contractContentEntriesByDID := make(map[string][]datatype.AuditLogEntry)
	if scope == componenttype.ContractWorkflowEngine && s.CRepo != nil {
		contractContentTrailQry := qry2.GetContractContentTrailQry{
			RetrievedBy:   middleware.GetParticipantID(ctx),
			HolderDID:     middleware.GetHolderDID(ctx),
			UserRoles:     middleware.GetUserRoles(ctx),
			Policy:        req.Policy,
			PolicyVersion: req.PolicyVersion,
		}
		contractContentTrailHandler := qry2.ContractContentTrailAuditor{
			DB:    s.DB,
			CRepo: s.CRepo,
		}
		result, err := contractContentTrailHandler.Handle(ctx, contractContentTrailQry)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		contractContentEntriesByDID = result
	}

	templatePolicyEntriesByDID := make(map[string][]datatype.AuditLogEntry)
	if scope == componenttype.ContractTemplateRepo && s.CTRepo != nil {
		policyTrailQry := qry2.GetContractPolicyTrailQry{
			RetrievedBy: middleware.GetParticipantID(ctx),
			HolderDID:   middleware.GetHolderDID(ctx),
			UserRoles:   middleware.GetUserRoles(ctx),
		}
		policyTrailHandler := qry2.ContractPolicyTrailAuditor{
			DB:     s.DB,
			CTRepo: s.CTRepo,
		}
		result, err := policyTrailHandler.Handle(ctx, policyTrailQry)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		templatePolicyEntriesByDID = result
	}

	result := make([]*processauditandcompliance.PACAuditResponse, 0)
	seenDIDs := map[string]bool{}
	for _, resLog := range resLogHistories {

		var did string
		history := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0)
		for _, entry := range resLog {

			if entry.DID != nil {
				did = *entry.DID
			}
			if !base.IsAuditVisibleEventType(entry.EventType) {
				continue
			}

			history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:               entry.ID,
				Component:        entry.Component,
				EventType:        entry.EventType,
				EventData:        entry.EventData,
				Did:              entry.DID,
				CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}
		if scope == componenttype.ContractTemplateRepo && did != "" {
			for _, entry := range templatePolicyEntriesByDID[did] {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:               entry.ID,
					Component:        entry.Component,
					EventType:        entry.EventType,
					EventData:        entry.EventData,
					Did:              entry.DID,
					CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
					GlobalLogPredCid: entry.GlobalLogPredCID,
					ResLogPredCid:    entry.ResLogPredCID,
				})
			}
			seenDIDs[did] = true
		}
		if scope == componenttype.ContractTemplateRepo && did != "" {

			provenanceQuery := qry2.GetTemplateApprovalProvenanceTrailQry{
				RetrievedBy: middleware.GetParticipantID(ctx),
				HolderDID:   middleware.GetHolderDID(ctx),
				UserRoles:   middleware.GetUserRoles(ctx),
				DID:         did,
				LogEntries:  resLog,
			}
			provenanceHandler := qry2.TemplateApprovalProvenanceTrailAuditor{}
			provenanceResult, err := provenanceHandler.Handle(ctx, provenanceQuery)
			if err != nil {
				return nil, processauditandcompliance.MakeInternalError(err)
			}

			for _, entry := range provenanceResult {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:               entry.ID,
					Component:        entry.Component,
					EventType:        entry.EventType,
					EventData:        entry.EventData,
					Did:              entry.DID,
					CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
					GlobalLogPredCid: entry.GlobalLogPredCID,
					ResLogPredCid:    entry.ResLogPredCID,
				})
			}
		}
		if scope == componenttype.ContractWorkflowEngine && did != "" {
			for _, entry := range contractContentEntriesByDID[did] {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:               entry.ID,
					Component:        entry.Component,
					EventType:        entry.EventType,
					EventData:        entry.EventData,
					Did:              entry.DID,
					CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
					GlobalLogPredCid: entry.GlobalLogPredCID,
					ResLogPredCid:    entry.ResLogPredCID,
				})
			}
			seenDIDs[did] = true
		}
		if len(history) == 0 {
			continue
		}

		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component:  scope.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: history,
		})
	}
	for did, entries := range templatePolicyEntriesByDID {
		if seenDIDs[did] || len(entries) == 0 {
			continue
		}

		auditTrail := []*processauditandcompliance.PACResourceAuditTrailEntry{}
		for _, entry := range entries {
			auditTrail = append(auditTrail, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:               entry.ID,
				Component:        entry.Component,
				EventType:        entry.EventType,
				EventData:        entry.EventData,
				Did:              entry.DID,
				CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}

		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component:  componenttype.ContractTemplateRepo.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: auditTrail,
		})
	}
	for did, entries := range contractContentEntriesByDID {
		if seenDIDs[did] || len(entries) == 0 {
			continue
		}

		auditTrail := []*processauditandcompliance.PACResourceAuditTrailEntry{}
		for _, entry := range entries {
			auditTrail = append(auditTrail, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:               entry.ID,
				Component:        entry.Component,
				EventType:        entry.EventType,
				EventData:        entry.EventData,
				Did:              entry.DID,
				CreatedAt:        entry.CreatedAt.Format(time.RFC3339),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}

		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component:  componenttype.ContractWorkflowEngine.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: auditTrail,
		})
	}

	return result, nil
}

func isStaticContractAudit(req *processauditandcompliance.PACAuditRequest) bool {
	if req == nil {
		return false
	}
	if req.ContractDocument != nil {
		return true
	}
	return req.AuditMode != nil && strings.EqualFold(strings.TrimSpace(*req.AuditMode), "static_contract")
}

func (s *processAuditAndCompliancesrvc) AuditReport(ctx context.Context, p *processauditandcompliance.AuditReportPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.audit_report")
	return
}

func (s *processAuditAndCompliancesrvc) Monitor(ctx context.Context, p *processauditandcompliance.MonitorPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.monitor")
	return
}

func (s *processAuditAndCompliancesrvc) IncidentReport(ctx context.Context, p *processauditandcompliance.IncidentReportPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.incident_report")
	return
}
