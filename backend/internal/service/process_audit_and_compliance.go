package service

import (
	"context"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/query"
	templatedb "digital-contracting-service/internal/templaterepository/db"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type processAuditAndCompliancesrvc struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
	CTRepo       templatedb.ContractTemplateRepo
	auth.JWTAuthenticator
}

func NewProcessAuditAndCompliance(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, auditTrailReader base.AuditTrailReader, ctRepo templatedb.ContractTemplateRepo) processauditandcompliance.Service {
	return &processAuditAndCompliancesrvc{DB: db, JWTAuthenticator: jwtAuth, ATrailReader: auditTrailReader, CTRepo: ctRepo}
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
		result, err := s.auditStaticContractContent(ctx, req)
		if err != nil {
			return nil, processauditandcompliance.MakeBadRequest(err)
		}
		return result, nil
	}

	qry := query.GetAuditLogQry{
		Scope:     scope,
		AuditedBy: middleware.GetUsername(ctx),
	}
	handler := query.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	resLogHistories, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	result := make([]*processauditandcompliance.PACAuditResponse, 0)
	for _, resLog := range resLogHistories {

		var did string
		history := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0)
		for _, entry := range resLog {

			if entry.DID != nil {
				did = *entry.DID
			}
			if scope == componenttype.ContractTemplateRepo {
				continue
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
				CreatedAt:        entry.CreatedAt.String(),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}
		if scope == componenttype.ContractTemplateRepo && s.CTRepo != nil && did != "" {
			policyEntries, err := s.auditTemplatePolicyTrailEntries(ctx, did)
			if err != nil {
				return nil, processauditandcompliance.MakeInternalError(err)
			}
			history = append(history, policyEntries...)
		}
		if scope == componenttype.ContractTemplateRepo && did != "" {
			history = append(history, s.auditTemplateApprovalProvenanceTrailEntries(did, resLog)...)
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

func (s *processAuditAndCompliancesrvc) auditStaticContractContent(ctx context.Context, req *processauditandcompliance.PACAuditRequest) ([]*processauditandcompliance.PACAuditResponse, error) {
	if req.ContractDocument == nil {
		return nil, fmt.Errorf("contract_document is required for static contract audits")
	}
	contractDID := stringPtrValue(req.ContractDid)
	if contractDID == "" {
		contractDID = "inline-contract"
	}
	metadata := validation.ContractContentAuditMetadata{
		ContractDID:     contractDID,
		ContractVersion: stringPtrValue(req.ContractVersion),
		PolicyVersion:   stringPtrValue(req.PolicyVersion),
		AuditedBy:       middleware.GetUsername(ctx),
	}
	findings, err := validation.AuditContractContent(req.ContractDocument, req.Policy, metadata)
	if err != nil {
		return nil, err
	}
	entries := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(findings))
	now := time.Now().UTC().Format(time.RFC3339)
	for i, finding := range findings {
		did := contractDID
		entries = append(entries, &processauditandcompliance.PACResourceAuditTrailEntry{
			ID:        int64(-1000 - i),
			Component: componenttype.ContractWorkflowEngine.String(),
			EventType: "CONTRACT_CONTENT_POLICY_AUDIT_FINDING",
			EventData: contractContentPolicyFindingEventData(finding, metadata),
			Did:       &did,
			CreatedAt: now,
		})
	}
	return []*processauditandcompliance.PACAuditResponse{
		{
			Component:  componenttype.ContractWorkflowEngine.String(),
			Did:        contractDID,
			CreatedAt:  now,
			AuditTrail: entries,
		},
	}, nil
}

func (s *processAuditAndCompliancesrvc) auditTemplatePolicyTrailEntries(ctx context.Context, did string) ([]*processauditandcompliance.PACResourceAuditTrailEntry, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	template, err := s.CTRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	findings, err := validation.AuditTemplatePolicies(template.TemplateData, validation.TemplatePolicyAuditMetadata{
		DID:          template.DID,
		TemplateType: template.TemplateType,
		State:        template.State,
	})
	if err != nil {
		return nil, err
	}
	entries := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(findings))
	for i, finding := range findings {
		templateDID := did
		entries = append(entries, &processauditandcompliance.PACResourceAuditTrailEntry{
			ID:        int64(-1 - i),
			Component: componenttype.ContractTemplateRepo.String(),
			EventType: "TEMPLATE_POLICY_AUDIT_FINDING",
			EventData: templatePolicyFindingEventData(finding, template),
			Did:       &templateDID,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}
	return entries, nil
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
