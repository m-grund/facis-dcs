package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type ContractContentAuditMetadata struct {
	ContractDID     string
	ContractVersion string
	PolicyVersion   string
	AuditedBy       string
}

type ContractContentPolicy struct {
	PolicySetID string                      `json:"policySetId"`
	Version     string                      `json:"version"`
	Rules       []ContractContentPolicyRule `json:"rules"`
}

type ContractContentPolicyRule struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Severity     string   `json:"severity"`
	Builtin      string   `json:"builtin"`
	SemanticPath string   `json:"semanticPath"`
	Values       []string `json:"values"`
	Min          *float64 `json:"min"`
	Max          *float64 `json:"max"`
	Required     string   `json:"required"`
	OntologyTerm string   `json:"ontologyTerm"`
	Requirement  string   `json:"requirement"`
}

func AuditContractContent(contractDocument any, policyDocument any, metadata ContractContentAuditMetadata) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	policy, err := normalizeContractContentPolicy(policyDocument)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.PolicyVersion) != "" {
		policy.Version = metadata.PolicyVersion
	}

	findings := []PolicyFinding{}
	for _, rule := range policy.Rules {
		findings = append(findings, auditContractContentRule(contract, rule)...)
	}
	for i := range findings {
		findings[i].PolicySetID = policy.PolicySetID
		findings[i].PolicyVersion = policy.Version
	}
	return findings, nil
}

func normalizeContractContentPolicy(raw any) (ContractContentPolicy, error) {
	if raw == nil {
		return ContractContentPolicy{}, fmt.Errorf("contract content policy with explicit rules is required")
	}

	normalized, err := normalizeObject(raw)
	if err != nil {
		return ContractContentPolicy{}, fmt.Errorf("decode contract content policy: %w", err)
	}
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	var policy ContractContentPolicy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return ContractContentPolicy{}, fmt.Errorf("decode contract content policy: %w", err)
	}
	if strings.TrimSpace(policy.PolicySetID) == "" {
		return ContractContentPolicy{}, fmt.Errorf("contract content policy requires policySetId")
	}
	if strings.TrimSpace(policy.Version) == "" {
		return ContractContentPolicy{}, fmt.Errorf("contract content policy requires version")
	}
	if len(policy.Rules) == 0 {
		return ContractContentPolicy{}, fmt.Errorf("contract content policy requires at least one explicit rule")
	}
	return policy, nil
}

func auditContractContentRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	rule = normalizeContractContentRule(rule)
	switch rule.Builtin {
	case "required_field":
		return auditRequiredFieldRule(contract, rule)
	case "value_not_in":
		return auditValueNotInRule(contract, rule)
	case "value_in":
		return auditValueInRule(contract, rule)
	case "min_number":
		return auditMinNumberRule(contract, rule)
	case "max_number":
		return auditMaxNumberRule(contract, rule)
	case "signature_level_at_least":
		return auditSignatureLevelRule(contract, rule)
	default:
		return []PolicyFinding{ruleFinding(rule, "error", fmt.Sprintf("unsupported static contract content rule builtin %q", rule.Builtin))}
	}
}

func normalizeContractContentRule(rule ContractContentPolicyRule) ContractContentPolicyRule {
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = "FACIS-CONTRACT-STATIC-CUSTOM"
	}
	if strings.TrimSpace(rule.Title) == "" {
		rule.Title = rule.ID
	}
	if strings.TrimSpace(rule.Severity) == "" {
		rule.Severity = "error"
	}
	if strings.TrimSpace(rule.Requirement) == "" {
		rule.Requirement = "DCS-FR-PACM-03"
	}
	return rule
}

func auditRequiredFieldRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	if value, ok := contractValue(contract, rule.SemanticPath); ok && !isEmptyAuditValue(value) {
		return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("required field %q is present", rule.SemanticPath))}
	}
	return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("required field %q is missing", rule.SemanticPath))}
}

func auditValueNotInRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	value, ok := contractString(contract, rule.SemanticPath)
	if !ok {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s is missing", rule.SemanticPath))}
	}
	blocked := normalizedSet(rule.Values)
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if blocked[normalized] {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s %q is blocked by current policy", rule.SemanticPath, normalized))}
	}
	return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("%s %q is not blocked", rule.SemanticPath, normalized))}
}

func auditValueInRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	value, ok := contractString(contract, rule.SemanticPath)
	if !ok {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s is missing", rule.SemanticPath))}
	}
	allowed := normalizedSet(rule.Values)
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if !allowed[normalized] {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s %q is not allowed by current policy", rule.SemanticPath, normalized))}
	}
	return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("%s %q is allowed", rule.SemanticPath, normalized))}
}

func auditMinNumberRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	if rule.Min == nil {
		return nil
	}
	value, ok := contractFloat(contract, rule.SemanticPath)
	if !ok {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s is missing or not numeric", rule.SemanticPath))}
	}
	if value+floatTolerance < *rule.Min {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s %.4g is below policy minimum %.4g", rule.SemanticPath, value, *rule.Min))}
	}
	return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("%s %.4g satisfies policy minimum %.4g", rule.SemanticPath, value, *rule.Min))}
}

func auditMaxNumberRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	if rule.Max == nil {
		return nil
	}
	value, ok := contractFloat(contract, rule.SemanticPath)
	if !ok {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s is missing or not numeric", rule.SemanticPath))}
	}
	if value > *rule.Max+floatTolerance {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s %.4g exceeds policy maximum %.4g", rule.SemanticPath, value, *rule.Max))}
	}
	return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("%s %.4g satisfies policy maximum %.4g", rule.SemanticPath, value, *rule.Max))}
}

func auditSignatureLevelRule(contract map[string]any, rule ContractContentPolicyRule) []PolicyFinding {
	value, ok := contractString(contract, rule.SemanticPath)
	if !ok {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s is missing", rule.SemanticPath))}
	}
	if !signatureLevelSatisfies(value, rule.Required) {
		return []PolicyFinding{ruleFinding(rule, rule.Severity, fmt.Sprintf("%s %q does not satisfy required level %q", rule.SemanticPath, value, rule.Required))}
	}
	return []PolicyFinding{ruleFinding(rule, "info", fmt.Sprintf("%s %q satisfies required level %q", rule.SemanticPath, value, rule.Required))}
}

func ruleFinding(rule ContractContentPolicyRule, severity string, message string) PolicyFinding {
	return contractFinding(rule.ID, rule.Title, severity, message, rule.SemanticPath, rule.SemanticPath, rule.OntologyTerm, rule.Requirement)
}

func contractFinding(ruleID, title, severity, message, path, semanticPath, ontologyTerm, requirement string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
		SemanticPath: semanticPath,
		OntologyTerm: ontologyTerm,
		Requirement:  requirement,
	}
}

const floatTolerance = 0.0000001

func normalizeObject(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, fmt.Errorf("document is required")
	}
	if doc, ok := raw.(map[string]any); ok {
		return doc, nil
	}
	if bytes, ok := raw.([]byte); ok {
		var doc map[string]any
		if err := json.Unmarshal(bytes, &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	if text, ok := raw.(string); ok {
		var doc map[string]any
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func contractValue(contract map[string]any, semanticPath string) (any, bool) {
	if value, ok := nestedValue(contract, strings.Split(semanticPath, ".")); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveExactKeyValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveSemanticPathValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	return nil, false
}

func contractString(contract map[string]any, semanticPath string) (string, bool) {
	value, ok := contractValue(contract, semanticPath)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func contractFloat(contract map[string]any, semanticPath string) (float64, bool) {
	value, ok := contractValue(contract, semanticPath)
	if !ok {
		return 0, false
	}
	return toFloat(value)
}

func nestedValue(current any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return current, true
	}
	obj, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	part := parts[0]
	candidates := []string{part, compactTerm(part)}
	for _, candidate := range candidates {
		if value, ok := obj[candidate]; ok {
			return nestedValue(value, parts[1:])
		}
	}
	return nil, false
}

func recursiveExactKeyValue(current any, key string) (any, bool) {
	switch value := current.(type) {
	case map[string]any:
		for candidateKey, candidateValue := range value {
			if candidateKey == key || compactTerm(candidateKey) == key {
				return candidateValue, true
			}
		}
		for _, candidateValue := range value {
			if found, ok := recursiveExactKeyValue(candidateValue, key); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range value {
			if found, ok := recursiveExactKeyValue(item, key); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func recursiveSemanticPathValue(current any, semanticPath string) (any, bool) {
	switch value := current.(type) {
	case map[string]any:
		pathValue, _ := value["semanticPath"].(string)
		if pathValue == "" {
			pathValue, _ = value["dcs:semanticPath"].(string)
		}
		if pathValue == semanticPath {
			for key, found := range value {
				switch compactTerm(key) {
				case "value", "hasTargetValue", "targetValue", "actualValue", "hasActualValue":
					return found, true
				}
			}
			for key, threshold := range value {
				if compactTerm(key) == "hasThreshold" {
					if found, ok := recursiveExactKeyValue(threshold, "hasTargetValue"); ok {
						return found, true
					}
				}
			}
		}
		for _, candidateValue := range value {
			if found, ok := recursiveSemanticPathValue(candidateValue, semanticPath); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range value {
			if found, ok := recursiveSemanticPathValue(item, semanticPath); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func compactJSONLDValue(value any) any {
	if obj, ok := value.(map[string]any); ok {
		for _, key := range []string{"@value", "value", "schema:value"} {
			if nested, ok := obj[key]; ok {
				return compactJSONLDValue(nested)
			}
		}
	}
	return value
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case float32:
		return float64(typed), !math.IsNaN(float64(typed))
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		float, err := typed.Float64()
		return float, err == nil
	case string:
		var parsed float64
		_, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizedSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		normalized := strings.ToUpper(strings.TrimSpace(value))
		if normalized != "" {
			set[normalized] = true
		}
	}
	return set
}

func signatureLevelSatisfies(actual string, required string) bool {
	rank := map[string]int{"SES": 1, "AES": 2, "QES": 3}
	actualRank := rank[strings.ToUpper(strings.TrimSpace(actual))]
	requiredRank := rank[strings.ToUpper(strings.TrimSpace(required))]
	return actualRank >= requiredRank && requiredRank > 0
}

func compactTerm(value string) string {
	if index := strings.LastIndex(value, ":"); index >= 0 && index < len(value)-1 {
		return value[index+1:]
	}
	return value
}

func isEmptyAuditValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}
