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
	SHACLShapes []ContractSHACLShape        `json:"shaclShapes"`
	SHACL       *ContractSHACLPolicy        `json:"shacl"`
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

type ContractSHACLPolicy struct {
	Shapes []ContractSHACLShape `json:"shapes"`
}

type ContractSHACLShape struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	Severity     string                  `json:"severity"`
	TargetClass  string                  `json:"targetClass"`
	Requirement  string                  `json:"requirement"`
	OntologyTerm string                  `json:"ontologyTerm"`
	Properties   []ContractSHACLProperty `json:"properties"`
	Property     []ContractSHACLProperty `json:"property"`
}

type ContractSHACLProperty struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	MinCount     *int     `json:"minCount"`
	MaxCount     *int     `json:"maxCount"`
	Datatype     string   `json:"datatype"`
	Class        string   `json:"class"`
	In           []string `json:"in"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	Requirement  string   `json:"requirement"`
	OntologyTerm string   `json:"ontologyTerm"`
}

func AuditContractContent(contractDocument any, policyDocument any, metadata ContractContentAuditMetadata) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	policy, err := normalizeContractContentPolicy(policyDocument, metadata)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.PolicyVersion) != "" {
		policy.Version = metadata.PolicyVersion
	}

	findings := []PolicyFinding{}
	findings = append(findings, auditJSONLDContract(contract, policy)...)
	shapes := contractSHACLShapes(policy)
	for _, shape := range shapes {
		findings = append(findings, auditContractSHACLShape(contract, policy, shape)...)
	}
	for _, rule := range policy.Rules {
		findings = append(findings, auditContractContentRule(contract, rule)...)
	}
	for i := range findings {
		findings[i].PolicySetID = policy.PolicySetID
		findings[i].PolicyVersion = policy.Version
	}
	return findings, nil
}

func normalizeContractContentPolicy(raw any, metadata ContractContentAuditMetadata) (ContractContentPolicy, error) {
	if raw == nil {
		return defaultContractContentPolicy(metadata), nil
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
		policy.PolicySetID = defaultContractPolicySetID
	}
	if strings.TrimSpace(policy.Version) == "" {
		policy.Version = metadata.PolicyVersion
		if strings.TrimSpace(policy.Version) == "" {
			policy.Version = defaultContractPolicyVersion
		}
	}
	return policy, nil
}

const (
	defaultContractPolicySetID   = "facis.dcs.contract.structure-semantics"
	defaultContractPolicyVersion = "v1"
)

func defaultContractContentPolicy(metadata ContractContentAuditMetadata) ContractContentPolicy {
	version := metadata.PolicyVersion
	if strings.TrimSpace(version) == "" {
		version = defaultContractPolicyVersion
	}
	return ContractContentPolicy{
		PolicySetID: defaultContractPolicySetID,
		Version:     version,
		SHACLShapes: []ContractSHACLShape{
			{
				ID:          "FACIS-CONTRACT-SHACL-CORE",
				Title:       "Contract JSON-LD must satisfy the FACIS core SHACL shape",
				Severity:    "error",
				TargetClass: "dcs:Contract",
				Requirement: "DCS-FR-PACM-03",
				Properties: []ContractSHACLProperty{
					{Path: "@id", MinCount: intPtr(1), MaxCount: intPtr(1), Datatype: "xsd:anyURI", Name: "Contract identifier"},
					{Path: "@type", MinCount: intPtr(1), In: []string{"dcs:Contract", "Contract"}, Name: "Contract type"},
					{Path: "provider", MinCount: intPtr(1), Class: "dcs:Company", Name: "Provider"},
					{Path: "customer", MinCount: intPtr(1), Class: "dcs:Company", Name: "Customer"},
					{Path: "contract.jurisdiction", MinCount: intPtr(1), Datatype: "xsd:string", Name: "Jurisdiction"},
				},
			},
		},
	}
}

func intPtr(value int) *int {
	return &value
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

func auditJSONLDContract(contract map[string]any, policy ContractContentPolicy) []PolicyFinding {
	findings := []PolicyFinding{}
	if _, ok := contract["@context"]; !ok {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-001", "Contract must declare a JSON-LD context", "error", "@context is required for JSON-LD contract documents", "@context", "jsonld:@context"))
	} else if !validJSONLDContext(contract["@context"]) {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-001", "Contract must declare a valid JSON-LD context", "error", "@context must be a string, object, or non-empty array of strings/objects", "@context", "jsonld:@context"))
	} else {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-001", "Contract declares a valid JSON-LD context", "info", "@context is present and structurally valid", "@context", "jsonld:@context"))
	}

	if id, ok := contractString(contract, "@id"); !ok {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-002", "Contract must have a JSON-LD identifier", "error", "@id is required", "@id", "jsonld:@id"))
	} else if !validIRIOrURN(id) {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-002", "Contract must have a JSON-LD identifier", "error", fmt.Sprintf("@id %q is not an absolute IRI or URN", id), "@id", "jsonld:@id"))
	} else {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-002", "Contract has a JSON-LD identifier", "info", "@id is present and structurally valid", "@id", "jsonld:@id"))
	}

	if values := valuesAtPath(contract, "@type"); len(values) == 0 {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-003", "Contract must declare a JSON-LD type", "error", "@type is required", "@type", "jsonld:@type"))
	} else if !allJSONLDTypesValid(values) {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-003", "Contract must declare valid JSON-LD types", "error", "@type must contain compact IRIs or absolute IRIs", "@type", "jsonld:@type"))
	} else {
		findings = append(findings, contractStructureFinding(policy, "FACIS-CONTRACT-JSONLD-003", "Contract declares valid JSON-LD types", "info", "@type is present and structurally valid", "@type", "jsonld:@type"))
	}

	return findings
}

func contractSHACLShapes(policy ContractContentPolicy) []ContractSHACLShape {
	shapes := make([]ContractSHACLShape, 0, len(policy.SHACLShapes))
	shapes = append(shapes, policy.SHACLShapes...)
	if policy.SHACL != nil {
		shapes = append(shapes, policy.SHACL.Shapes...)
	}
	if len(shapes) == 0 {
		defaultPolicy := defaultContractContentPolicy(ContractContentAuditMetadata{PolicyVersion: policy.Version})
		return defaultPolicy.SHACLShapes
	}
	return shapes
}

func auditContractSHACLShape(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape) []PolicyFinding {
	shape = normalizeContractSHACLShape(shape)
	if shape.TargetClass != "" && !jsonLDTypeMatches(valuesAtPath(contract, "@type"), shape.TargetClass) {
		return []PolicyFinding{contractStructureFinding(policy, shape.ID, shape.Title, shape.Severity, fmt.Sprintf("target class %q does not match contract @type", shape.TargetClass), "@type", shape.TargetClass)}
	}

	properties := shape.Properties
	properties = append(properties, shape.Property...)
	findings := []PolicyFinding{}
	for index, property := range properties {
		findings = append(findings, auditContractSHACLProperty(contract, policy, shape, normalizeContractSHACLProperty(shape, property, index))...)
	}
	if len(findings) == 0 {
		findings = append(findings, contractStructureFinding(policy, shape.ID, shape.Title, "info", "SHACL shape conforms", shape.TargetClass, shape.TargetClass))
	}
	return findings
}

func auditContractSHACLProperty(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty) []PolicyFinding {
	values := valuesAtPath(contract, property.Path)
	findings := []PolicyFinding{}
	if property.MinCount != nil && len(nonEmptyValues(values)) < *property.MinCount {
		findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s requires at least %d value(s)", propertyLabel(property), *property.MinCount)))
	}
	if property.MaxCount != nil && len(nonEmptyValues(values)) > *property.MaxCount {
		findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s allows at most %d value(s)", propertyLabel(property), *property.MaxCount)))
	}
	if len(nonEmptyValues(values)) == 0 {
		return findings
	}
	if property.Datatype != "" {
		for _, value := range values {
			if !valueConformsDatatype(value, property.Datatype) {
				findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s must use datatype %s", propertyLabel(property), property.Datatype)))
				break
			}
		}
	}
	if len(property.In) > 0 {
		allowed := normalizedSet(property.In)
		matched := false
		for _, value := range values {
			if allowed[strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(value))))] {
				matched = true
				break
			}
		}
		if !matched {
			findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s must be one of %s", propertyLabel(property), strings.Join(property.In, ", "))))
		}
	}
	if property.Class != "" {
		for _, value := range values {
			if !valueHasClass(value, property.Class) {
				findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s must reference class %s", propertyLabel(property), property.Class)))
				break
			}
		}
	}
	if len(findings) == 0 {
		return []PolicyFinding{contractFinding(property.ID, shape.Title, "info", fmt.Sprintf("%s conforms", propertyLabel(property)), property.Path, property.Path, property.OntologyTerm, property.Requirement)}
	}
	return findings
}

func normalizeContractSHACLShape(shape ContractSHACLShape) ContractSHACLShape {
	if strings.TrimSpace(shape.ID) == "" {
		shape.ID = "FACIS-CONTRACT-SHACL-CUSTOM"
	}
	if strings.TrimSpace(shape.Title) == "" {
		shape.Title = shape.ID
	}
	if strings.TrimSpace(shape.Severity) == "" {
		shape.Severity = "error"
	}
	if strings.TrimSpace(shape.Requirement) == "" {
		shape.Requirement = "DCS-FR-PACM-03"
	}
	return shape
}

func normalizeContractSHACLProperty(shape ContractSHACLShape, property ContractSHACLProperty, index int) ContractSHACLProperty {
	if strings.TrimSpace(property.ID) == "" {
		property.ID = fmt.Sprintf("%s-PROP-%03d", shape.ID, index+1)
	}
	if strings.TrimSpace(property.Severity) == "" {
		property.Severity = shape.Severity
	}
	if strings.TrimSpace(property.Requirement) == "" {
		property.Requirement = shape.Requirement
	}
	if strings.TrimSpace(property.OntologyTerm) == "" {
		property.OntologyTerm = shape.OntologyTerm
	}
	return property
}

func contractStructureFinding(policy ContractContentPolicy, ruleID, title, severity, message, path, ontologyTerm string) PolicyFinding {
	finding := contractFinding(ruleID, title, severity, message, path, path, ontologyTerm, "DCS-FR-PACM-03")
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	return finding
}

func shaclPropertyFinding(policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty, fallbackMessage string) PolicyFinding {
	message := property.Message
	if strings.TrimSpace(message) == "" {
		message = fallbackMessage
	}
	finding := contractFinding(property.ID, shape.Title, property.Severity, message, property.Path, property.Path, property.OntologyTerm, property.Requirement)
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	return finding
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

func valuesAtPath(contract map[string]any, semanticPath string) []any {
	value, ok := contractValue(contract, semanticPath)
	if !ok {
		return nil
	}
	if values, ok := value.([]any); ok {
		result := make([]any, 0, len(values))
		for _, item := range values {
			result = append(result, compactJSONLDValue(item))
		}
		return result
	}
	return []any{compactJSONLDValue(value)}
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

func validJSONLDContext(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]any:
		return len(typed) > 0
	case []any:
		if len(typed) == 0 {
			return false
		}
		for _, item := range typed {
			if !validJSONLDContext(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validIRIOrURN(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "urn:")
}

func allJSONLDTypesValid(values []any) bool {
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return false
		}
		if !validJSONLDTerm(text) {
			return false
		}
	}
	return true
}

func validJSONLDTerm(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.Contains(trimmed, ":") || strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "urn:")
}

func jsonLDTypeMatches(values []any, targetClass string) bool {
	target := compactTerm(strings.TrimSpace(targetClass))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text == targetClass || compactTerm(text) == target {
			return true
		}
	}
	return false
}

func nonEmptyValues(values []any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if !isEmptyAuditValue(compactJSONLDValue(value)) {
			result = append(result, value)
		}
	}
	return result
}

func valueConformsDatatype(value any, datatype string) bool {
	value = compactJSONLDValue(value)
	normalized := strings.ToLower(compactTerm(datatype))
	switch normalized {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer", "int", "long":
		float, ok := toFloat(value)
		return ok && math.Trunc(float) == float
	case "decimal", "double", "float":
		_, ok := toFloat(value)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "anyuri", "uri":
		text, ok := value.(string)
		return ok && validIRIOrURN(text)
	default:
		return true
	}
}

func valueHasClass(value any, class string) bool {
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return jsonLDTypeMatches(valuesAtObjectPath(obj, "@type"), class)
}

func valuesAtObjectPath(obj map[string]any, semanticPath string) []any {
	value, ok := contractValue(obj, semanticPath)
	if !ok {
		return nil
	}
	if values, ok := value.([]any); ok {
		return values
	}
	return []any{value}
}

func propertyLabel(property ContractSHACLProperty) string {
	if strings.TrimSpace(property.Name) != "" {
		return property.Name
	}
	return property.Path
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
