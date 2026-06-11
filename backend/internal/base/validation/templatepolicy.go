package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"digital-contracting-service/internal/base/datatype"
)

const defaultTemplatePolicyFile = "docs/policies/facis-template-audit-policies.json"

type TemplatePolicyAuditMetadata struct {
	DID          string
	TemplateType string
	State        string
}

type PolicyFinding struct {
	PolicySetID   string `json:"policySetId"`
	PolicyVersion string `json:"policyVersion"`
	RuleID        string `json:"ruleId"`
	Title         string `json:"title"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Path          string `json:"path,omitempty"`
	SemanticPath  string `json:"semanticPath,omitempty"`
	OntologyTerm  string `json:"ontologyTerm,omitempty"`
	Requirement   string `json:"requirement,omitempty"`
}

type templatePolicySet struct {
	PolicySetID string               `json:"policySetId"`
	Version     string               `json:"version"`
	Ontology    string               `json:"ontology"`
	Rules       []templatePolicyRule `json:"rules"`
}

type templatePolicyRule struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Severity     string         `json:"severity"`
	Builtin      string         `json:"builtin"`
	OntologyTerm string         `json:"ontologyTerm"`
	Parameters   map[string]any `json:"parameters"`
}

func AuditTemplatePolicies(raw *datatype.JSON, metadata TemplatePolicyAuditMetadata) ([]PolicyFinding, error) {
	normalized, err := NormalizeTemplateData(raw)
	if err != nil {
		return []PolicyFinding{
			{
				PolicySetID:   "facis.dcs.template.audit",
				PolicyVersion: "v1",
				RuleID:        "FACIS-TPL-STRUCT-000",
				Title:         "Template data must be structurally valid",
				Severity:      "error",
				Message:       err.Error(),
				Path:          "template_data",
				OntologyTerm:  "dcs:DocumentStructure",
			},
		}, nil
	}

	data, err := decodeDocumentData(normalized)
	if err != nil {
		return nil, err
	}
	policySet, err := loadTemplatePolicySet()
	if err != nil {
		return nil, err
	}

	findings := []PolicyFinding{}
	for _, rule := range policySet.Rules {
		findings = append(findings, evaluateTemplatePolicyRule(policySet, rule, data, metadata)...)
	}
	return findings, nil
}

func loadTemplatePolicySet() (*templatePolicySet, error) {
	path, err := resolveTemplatePolicyFile()
	if err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template policy file %q: %w", path, err)
	}
	var policySet templatePolicySet
	if err := json.Unmarshal(bytes, &policySet); err != nil {
		return nil, fmt.Errorf("decode template policy file %q: %w", path, err)
	}
	if strings.TrimSpace(policySet.PolicySetID) == "" || strings.TrimSpace(policySet.Version) == "" {
		return nil, fmt.Errorf("template policy file %q requires policySetId and version", path)
	}
	return &policySet, nil
}

func resolveTemplatePolicyFile() (string, error) {
	if path := strings.TrimSpace(os.Getenv("FACIS_TEMPLATE_POLICY_FILE")); path != "" {
		return path, nil
	}
	candidates := []string{
		defaultTemplatePolicyFile,
		filepath.Join("..", defaultTemplatePolicyFile),
		filepath.Join("..", "..", defaultTemplatePolicyFile),
		filepath.Join("..", "..", "..", defaultTemplatePolicyFile),
		filepath.Join("..", "..", "..", "..", defaultTemplatePolicyFile),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("template policy file not found")
}

func evaluateTemplatePolicyRule(policySet *templatePolicySet, rule templatePolicyRule, data documentData, metadata TemplatePolicyAuditMetadata) []PolicyFinding {
	switch rule.Builtin {
	case "required_schema_refs":
		return auditRequiredSchemaRefs(policySet, rule, data)
	case "required_policy_refs":
		return auditRequiredPolicyRefs(policySet, rule, data)
	case "finished_template_has_clause_semantics":
		return auditFinishedTemplateHasClauseSemantics(policySet, rule, data)
	case "finished_template_state":
		return auditFinishedTemplateState(policySet, rule, metadata)
	case "canonical_domain_fields":
		return auditCanonicalDomainFields(policySet, rule, data)
	case "constrained_parameters_use_value_constraint":
		return auditConstrainedParameters(policySet, rule, data)
	case "required_domain_fields":
		return auditRequiredDomainFields(policySet, rule, data)
	case "audit_metadata_complete":
		return auditMetadataComplete(policySet, rule, data, metadata)
	default:
		return []PolicyFinding{newPolicyFinding(policySet, rule, "Unsupported policy builtin "+rule.Builtin, "rules."+rule.ID, "")}
	}
}

func auditRequiredSchemaRefs(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	refs, _ := data["schemaRefs"].(map[string]any)
	required := map[string]string{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"templateData":      SchemaTemplateDataV1,
	}
	findings := []PolicyFinding{}
	for key, expected := range required {
		if refs[key] != expected {
			findings = append(findings, newPolicyFinding(policySet, rule, fmt.Sprintf("schemaRefs.%s must be %q", key, expected), "schemaRefs."+key, ""))
		}
	}
	return findings
}

func auditRequiredPolicyRefs(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	policies, _ := asArray(data["policyRefs"])
	seen := map[string]bool{}
	for _, item := range policies {
		policy, ok := item.(map[string]any)
		if !ok {
			continue
		}
		policyID, _ := policy["policyId"].(string)
		seen[policyID] = true
	}
	findings := []PolicyFinding{}
	required := []string{PolicyTemplateStructureV1, PolicyTemplateSemanticConditionsV1}
	for _, policyID := range required {
		if !seen[policyID] {
			findings = append(findings, newPolicyFinding(policySet, rule, fmt.Sprintf("required policy %q is missing", policyID), "policyRefs", ""))
		}
	}
	return findings
}

func auditFinishedTemplateHasClauseSemantics(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := asArray(data["semanticConditions"])
	clauseWithCondition := false
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		refs, _ := asArray(block["conditionIds"])
		if len(refs) > 0 {
			clauseWithCondition = true
			break
		}
	}
	if clauseWithCondition && len(conditions) > 0 {
		return nil
	}
	return []PolicyFinding{newPolicyFinding(policySet, rule, "finished templates should contain at least one clause linked to a semantic condition", "documentBlocks", "")}
}

func auditFinishedTemplateState(policySet *templatePolicySet, rule templatePolicyRule, metadata TemplatePolicyAuditMetadata) []PolicyFinding {
	states := stringSliceParameter(rule.Parameters, "states")
	if len(states) == 0 || containsString(states, metadata.State) {
		return nil
	}
	return []PolicyFinding{newPolicyFinding(policySet, rule, fmt.Sprintf("template state %q is not one of the finished states: %s", metadata.State, strings.Join(states, ", ")), "state", "")}
}

func auditCanonicalDomainFields(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	findings := []PolicyFinding{}
	forEachSemanticParameter(data, func(conditionID string, index int, param map[string]any) {
		semanticPath, _ := param["semanticPath"].(string)
		if _, ok := ontologyDomainFieldIndex[semanticPath]; !ok {
			findings = append(findings, newPolicyFinding(policySet, rule, fmt.Sprintf("semantic condition %q uses unknown domain field %q", conditionID, semanticPath), fmt.Sprintf("semanticConditions.%s.parameters.%d", conditionID, index), semanticPath))
		}
	})
	return findings
}

func auditConstrainedParameters(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	findings := []PolicyFinding{}
	forEachSemanticParameter(data, func(conditionID string, index int, param map[string]any) {
		semanticPath, _ := param["semanticPath"].(string)
		field, ok := ontologyDomainFieldIndex[semanticPath]
		if !ok || field.Constraint == nil {
			return
		}
		if _, ok := param["valueConstraint"].(map[string]any); !ok {
			findings = append(findings, newPolicyFinding(policySet, rule, fmt.Sprintf("semantic field %q requires valueConstraint metadata", semanticPath), fmt.Sprintf("semanticConditions.%s.parameters.%d.valueConstraint", conditionID, index), semanticPath))
		}
	})
	return findings
}

func auditRequiredDomainFields(policySet *templatePolicySet, rule templatePolicyRule, data documentData) []PolicyFinding {
	required := stringSliceParameter(rule.Parameters, "semanticPaths")
	if len(required) == 0 {
		return nil
	}
	seen := map[string]bool{}
	forEachSemanticParameter(data, func(_ string, _ int, param map[string]any) {
		semanticPath, _ := param["semanticPath"].(string)
		seen[canonicalDomainFieldTerm(semanticPath)] = true
	})
	findings := []PolicyFinding{}
	for _, semanticPath := range required {
		if !seen[canonicalDomainFieldTerm(semanticPath)] {
			findings = append(findings, newPolicyFinding(policySet, rule, fmt.Sprintf("required semantic field %q is missing", semanticPath), "semanticConditions.parameters", semanticPath))
		}
	}
	return findings
}

func auditMetadataComplete(policySet *templatePolicySet, rule templatePolicyRule, data documentData, metadata TemplatePolicyAuditMetadata) []PolicyFinding {
	findings := []PolicyFinding{}
	if strings.TrimSpace(metadata.DID) == "" {
		findings = append(findings, newPolicyFinding(policySet, rule, "template DID is missing", "did", ""))
	}
	if strings.TrimSpace(metadata.State) == "" {
		findings = append(findings, newPolicyFinding(policySet, rule, "template state is missing", "state", ""))
	}
	if _, ok := data["validation"].(map[string]any); !ok {
		findings = append(findings, newPolicyFinding(policySet, rule, "validation profile metadata is missing", "validation", ""))
	}
	if _, ok := data["policyRefs"].([]any); !ok {
		if _, ok := asArray(data["policyRefs"]); !ok {
			findings = append(findings, newPolicyFinding(policySet, rule, "policyRefs metadata is missing", "policyRefs", ""))
		}
	}
	return findings
}

func forEachSemanticParameter(data documentData, visit func(conditionID string, index int, param map[string]any)) {
	conditions, _ := asArray(data["semanticConditions"])
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := condition["conditionId"].(string)
		parameters, _ := asArray(condition["parameters"])
		for i, rawParam := range parameters {
			param, ok := rawParam.(map[string]any)
			if !ok {
				continue
			}
			visit(conditionID, i, param)
		}
	}
}

func stringSliceParameter(parameters map[string]any, key string) []string {
	if parameters == nil {
		return nil
	}
	raw, ok := parameters[key]
	if !ok {
		return nil
	}
	items, ok := asArray(raw)
	if !ok {
		return nil
	}
	values := []string{}
	for _, item := range items {
		value, ok := item.(string)
		if ok && strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	return values
}

func newPolicyFinding(policySet *templatePolicySet, rule templatePolicyRule, message string, path string, semanticPath string) PolicyFinding {
	severity := rule.Severity
	if severity == "" {
		severity = "warning"
	}
	return PolicyFinding{
		PolicySetID:   policySet.PolicySetID,
		PolicyVersion: policySet.Version,
		RuleID:        rule.ID,
		Title:         rule.Title,
		Severity:      severity,
		Message:       message,
		Path:          path,
		SemanticPath:  semanticPath,
		OntologyTerm:  rule.OntologyTerm,
	}
}
