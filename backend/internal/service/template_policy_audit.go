package service

import (
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/validation"
	templatedb "digital-contracting-service/internal/templaterepository/db"
	"time"
)

func templatePolicyFindingEventData(finding validation.PolicyFinding, template *templatedb.ContractTemplate) map[string]any {
	data := map[string]any{
		"policySetId":   finding.PolicySetID,
		"policyVersion": finding.PolicyVersion,
		"ruleId":        finding.RuleID,
		"title":         finding.Title,
		"severity":      finding.Severity,
		"message":       finding.Message,
		"path":          finding.Path,
		"semanticPath":  finding.SemanticPath,
		"ontologyTerm":  finding.OntologyTerm,
		"requirement":   finding.Requirement,
	}
	if template == nil {
		return data
	}

	data["objectType"] = "contractTemplate"
	data["objectDid"] = template.DID
	data["objectName"] = stringPtrValue(template.Name)
	data["objectDescription"] = stringPtrValue(template.Description)
	data["templateType"] = template.TemplateType
	data["state"] = template.State
	data["documentNumber"] = stringPtrValue(template.DocumentNumber)
	if template.Version != nil {
		data["version"] = *template.Version
	}
	data["createdBy"] = template.CreatedBy
	data["createdAt"] = template.CreatedAt.UTC().Format(time.RFC3339)
	data["updatedAt"] = template.UpdatedAt.UTC().Format(time.RFC3339)
	return data
}

func contractContentPolicyFindingEventData(finding validation.PolicyFinding, metadata validation.ContractContentAuditMetadata) map[string]any {
	data := map[string]any{
		"policySetId":     finding.PolicySetID,
		"policyVersion":   finding.PolicyVersion,
		"ruleId":          finding.RuleID,
		"title":           finding.Title,
		"severity":        finding.Severity,
		"message":         finding.Message,
		"path":            finding.Path,
		"semanticPath":    finding.SemanticPath,
		"ontologyTerm":    finding.OntologyTerm,
		"requirement":     finding.Requirement,
		"objectType":      "contract",
		"objectDid":       metadata.ContractDID,
		"contractVersion": metadata.ContractVersion,
		"auditedBy":       metadata.AuditedBy,
	}
	return data
}

func (s *processAuditAndCompliancesrvc) auditTemplateApprovalProvenanceTrailEntries(did string, logEntries []datatype.AuditLogEntry) []*processauditandcompliance.PACResourceAuditTrailEntry {
	findings := validation.AuditTemplateApprovalProvenance(did, logEntries)
	entries := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(findings))
	now := time.Now().UTC().Format(time.RFC3339)
	for i, finding := range findings {
		templateDID := did
		entries = append(entries, &processauditandcompliance.PACResourceAuditTrailEntry{
			ID:        int64(-2000 - i),
			Component: componenttype.ContractTemplateRepo.String(),
			EventType: "TEMPLATE_APPROVAL_PROVENANCE_AUDIT_FINDING",
			EventData: templateApprovalProvenanceFindingEventData(finding, did),
			Did:       &templateDID,
			CreatedAt: now,
		})
	}
	return entries
}

func templateApprovalProvenanceFindingEventData(finding validation.PolicyFinding, did string) map[string]any {
	return map[string]any{
		"policySetId":   finding.PolicySetID,
		"policyVersion": finding.PolicyVersion,
		"ruleId":        finding.RuleID,
		"title":         finding.Title,
		"severity":      finding.Severity,
		"message":       finding.Message,
		"path":          finding.Path,
		"semanticPath":  finding.SemanticPath,
		"ontologyTerm":  finding.OntologyTerm,
		"requirement":   finding.Requirement,
		"objectType":    "contractTemplate",
		"objectDid":     did,
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
