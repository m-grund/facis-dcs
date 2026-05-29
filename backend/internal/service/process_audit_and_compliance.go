package service

import (
	"context"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/validation"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
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

	contractContentEntriesByDID := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	if scope == componenttype.ContractWorkflowEngine && s.CRepo != nil {
		contractContentEntriesByDID, err = s.auditExistingContractContentTrailEntries(ctx, req)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
	}
	templatePolicyEntriesByDID := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	if scope == componenttype.ContractTemplateRepo && s.CTRepo != nil {
		templatePolicyEntriesByDID, err = s.auditExistingTemplatePolicyTrailEntries(ctx)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
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
				CreatedAt:        entry.CreatedAt.String(),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}
		if scope == componenttype.ContractTemplateRepo && did != "" {
			history = append(history, templatePolicyEntriesByDID[did]...)
			seenDIDs[did] = true
		}
		if scope == componenttype.ContractTemplateRepo && did != "" {
			history = append(history, s.auditTemplateApprovalProvenanceTrailEntries(did, resLog)...)
		}
		if scope == componenttype.ContractWorkflowEngine && did != "" {
			history = append(history, contractContentEntriesByDID[did]...)
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
		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component:  componenttype.ContractTemplateRepo.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: entries,
		})
	}
	for did, entries := range contractContentEntriesByDID {
		if seenDIDs[did] || len(entries) == 0 {
			continue
		}
		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component:  componenttype.ContractWorkflowEngine.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: entries,
		})
	}

	return result, nil
}

func (s *processAuditAndCompliancesrvc) auditExistingTemplatePolicyTrailEntries(ctx context.Context) (map[string][]*processauditandcompliance.PACResourceAuditTrailEntry, error) {
	result := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	templates, err := s.CTRepo.ReadAllMetaData(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for templateIndex, metadata := range templates {
		template, err := s.CTRepo.ReadDataByID(ctx, tx, metadata.DID)
		if err != nil {
			return nil, err
		}
		if template.TemplateData == nil || !template.TemplateData.IsNotNullValue() {
			continue
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
		for findingIndex, finding := range findings {
			templateDID := template.DID
			entries = append(entries, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:        int64(-4000000 - (templateIndex * 10000) - findingIndex),
				Component: componenttype.ContractTemplateRepo.String(),
				EventType: "TEMPLATE_POLICY_AUDIT_FINDING",
				EventData: templatePolicyFindingEventData(finding, template),
				Did:       &templateDID,
				CreatedAt: now,
			})
		}
		result[template.DID] = entries
	}
	if err := tx.Commit(); err != nil {
		return nil, err
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

func (s *processAuditAndCompliancesrvc) auditExistingContractContentTrailEntries(ctx context.Context, req *processauditandcompliance.PACAuditRequest) (map[string][]*processauditandcompliance.PACResourceAuditTrailEntry, error) {
	result := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	contracts, err := s.CRepo.ReadAllMetaData(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	policy := req.Policy
	if policy == nil {
		policy = defaultContractContentAuditPolicy()
	}
	for contractIndex, metadata := range contracts {
		contract, err := s.CRepo.ReadDataByID(ctx, tx, metadata.DID)
		if err != nil {
			return nil, err
		}
		if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
			continue
		}
		auditMetadata := validation.ContractContentAuditMetadata{
			ContractDID:     contract.DID,
			ContractVersion: fmt.Sprint(contract.ContractVersion),
			PolicyVersion:   stringPtrValue(req.PolicyVersion),
			AuditedBy:       middleware.GetUsername(ctx),
		}
		findings, err := validation.AuditContractContent(contract.ContractData, policy, auditMetadata)
		if err != nil {
			return nil, err
		}
		entries := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(findings))
		for findingIndex, finding := range findings {
			did := contract.DID
			entries = append(entries, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:        int64(-3000000 - (contractIndex * 10000) - findingIndex),
				Component: componenttype.ContractWorkflowEngine.String(),
				EventType: "CONTRACT_CONTENT_POLICY_AUDIT_FINDING",
				EventData: contractContentPolicyFindingEventData(finding, auditMetadata),
				Did:       &did,
				CreatedAt: now,
			})
		}
		result[contract.DID] = entries
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func defaultContractContentAuditPolicy() map[string]any {
	return map[string]any{
		"policySetId": "facis.dcs.contract.structure-semantics",
		"version":     "v1",
		"shaclShapes": []map[string]any{
			{
				"id":          "FACIS-CONTRACT-SHACL-SLA",
				"title":       "Contract JSON-LD must satisfy the SLA SHACL shape",
				"targetClass": "dcs:Contract",
				"severity":    "error",
				"requirement": "DCS-FR-PACM-03",
				"properties": []map[string]any{
					{"path": "@id", "name": "Contract identifier", "minCount": 1, "maxCount": 1, "datatype": "xsd:anyURI"},
					{"path": "@type", "name": "Contract type", "minCount": 1, "in": []string{"dcs:Contract", "Contract"}},
					{"path": "parties", "name": "Contract parties", "minCount": 2, "class": "dcs:CompanyParty"},
					{"path": "contract.jurisdiction", "name": "Jurisdiction", "minCount": 1, "datatype": "xsd:string"},
					{"path": "service.sla.availability", "name": "SLA availability", "minCount": 1, "datatype": "xsd:decimal"},
				},
			},
		},
		"rules": []map[string]any{
			{
				"id":           "FACIS-CONTRACT-STATIC-002",
				"title":        "Contract jurisdiction must be allowed",
				"builtin":      "value_in",
				"semanticPath": "contract.jurisdiction",
				"values":       []string{"DEU", "AUT", "CHE", "FRA", "NLD", "BEL", "LUX", "POL", "CZE", "ESP", "ITA", "GBR", "USA"},
				"ontologyTerm": "dcs:Contract",
				"requirement":  "DCS-FR-PACM-03",
			},
			{
				"id":           "FACIS-CONTRACT-STATIC-003",
				"title":        "Service availability must satisfy policy minimum",
				"builtin":      "min_number",
				"semanticPath": "service.sla.availability",
				"min":          99.9,
				"ontologyTerm": "sla:AvailabilityMetric",
				"requirement":  "DCS-FR-CWE-09",
			},
			{
				"id":           "FACIS-CONTRACT-STATIC-004",
				"title":        "Service response time must satisfy policy maximum",
				"builtin":      "max_number",
				"semanticPath": "service.sla.responseTime",
				"max":          15,
				"ontologyTerm": "sla:ResponseTimeMetric",
				"requirement":  "DCS-FR-CWE-09",
			},
			{
				"id":           "FACIS-CONTRACT-STATIC-005",
				"title":        "Service resolution time must satisfy policy maximum",
				"builtin":      "max_number",
				"semanticPath": "service.sla.resolutionTime",
				"max":          240,
				"ontologyTerm": "sla:ResolutionTimeMetric",
				"requirement":  "DCS-FR-CWE-09",
			},
			{
				"id":           "FACIS-CONTRACT-STATIC-006",
				"title":        "Signature level must satisfy policy",
				"builtin":      "signature_level_at_least",
				"semanticPath": "signature.requiredLevel",
				"required":     "AES",
				"ontologyTerm": "dcs:SignatureLevelCode",
				"requirement":  "DCS-FR-PACM-03",
			},
		},
	}
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
