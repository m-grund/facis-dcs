package service

import (
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

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
