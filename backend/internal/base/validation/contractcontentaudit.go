package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ContractContentAuditMetadata struct {
	ContractDID     string
	ContractVersion string
	PolicyVersion   string
	AuditedBy       string
	HolderDID       string
}

type ContractContentPolicy struct {
	PolicySetID string                      `json:"policySetId"`
	Version     string                      `json:"version"`
	Rules       []ContractContentPolicyRule `json:"rules"`
	SHACLShapes []ContractSHACLShape        `json:"shaclShapes"`
	SHACLFiles  []string                    `json:"shaclShapeFiles"`
	Profiles    []string                    `json:"validationProfiles"`
	profiles    []ValidationProfile
	SHACL       *ContractSHACLPolicy `json:"shacl"`
}

type ContractContentPolicyRule struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Severity     string   `json:"severity"`
	Builtin      string   `json:"builtin"`
	SemanticPath string   `json:"semanticPath"`
	Values       []string `json:"values"`
	ValuesRef    string   `json:"valuesRef"`
	Min          *float64 `json:"min"`
	Max          *float64 `json:"max"`
	Required     string   `json:"required"`
	OntologyTerm string   `json:"ontologyTerm"`
}

type ContractSHACLPolicy struct {
	Shapes []ContractSHACLShape `json:"shapes"`
}

type ContractSHACLShape struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	Severity     string                  `json:"severity"`
	TargetClass  string                  `json:"targetClass"`
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
	MinInclusive *float64 `json:"minInclusive"`
	Datatype     string   `json:"datatype"`
	Class        string   `json:"class"`
	Node         string   `json:"node"`
	In           []string `json:"in"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
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
	shapes := contractSHACLShapes(policy)
	shapeIndex := contractSHACLShapeIndex(shapes)
	for _, shape := range shapes {
		if !isRootContractAuditShape(contract, shape) {
			continue
		}
		findings = append(findings, auditContractSHACLShape(contract, policy, shape, shapeIndex)...)
	}
	for _, profile := range policy.profiles {
		findings = append(findings, auditContractValidationProfile(contract, profile)...)
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
		loaded, err := loadDefaultContractContentPolicyDocument()
		if err != nil {
			return ContractContentPolicy{}, err
		}
		raw = loaded
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
	fileShapes, err := loadContractSHACLShapeFiles(policy.SHACLFiles)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	policy.SHACLShapes = append(policy.SHACLShapes, fileShapes...)
	profiles, err := loadContractValidationProfiles(policy.Profiles)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	policy.profiles = profiles
	return policy, nil
}

const (
	defaultContractPolicySetID   = "facis.dcs.contract.structure-semantics"
	defaultContractPolicyVersion = "v1"
	defaultContractPolicyFile    = "docs/policies/facis-contract-content-audit-policies.json"
)

func loadDefaultContractContentPolicyDocument() (map[string]any, error) {
	path, err := resolveContractContentPolicyFile()
	if err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract content policy file %q: %w", path, err)
	}
	var policy map[string]any
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return nil, fmt.Errorf("decode contract content policy file %q: %w", path, err)
	}
	return policy, nil
}

func resolveContractContentPolicyFile() (string, error) {
	if path := strings.TrimSpace(os.Getenv("FACIS_CONTRACT_CONTENT_POLICY_FILE")); path != "" {
		return path, nil
	}
	candidates := []string{
		defaultContractPolicyFile,
		filepath.Join("..", defaultContractPolicyFile),
		filepath.Join("..", "..", defaultContractPolicyFile),
		filepath.Join("..", "..", "..", defaultContractPolicyFile),
		filepath.Join("..", "..", "..", "..", defaultContractPolicyFile),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("contract content policy file not found")
}

func loadContractSHACLShapeFiles(files []string) ([]ContractSHACLShape, error) {
	shapes := []ContractSHACLShape{}
	for _, file := range files {
		path, err := resolveContractContentDocumentFile(file)
		if err != nil {
			return nil, err
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read contract SHACL shapes file %q: %w", path, err)
		}
		parsed, err := parseContractSHACLShapesTTL(string(bytes))
		if err != nil {
			return nil, fmt.Errorf("parse contract SHACL shapes file %q: %w", path, err)
		}
		shapes = append(shapes, parsed...)
	}
	return shapes, nil
}

func loadContractValidationProfiles(files []string) ([]ValidationProfile, error) {
	profiles := []ValidationProfile{}
	for _, file := range files {
		path, err := resolveContractContentDocumentFile(file)
		if err != nil {
			return nil, err
		}
		profile, err := LoadValidationProfileFile(path)
		if err != nil {
			return nil, fmt.Errorf("load contract validation profile %q: %w", path, err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func resolveContractContentDocumentFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("contract content document path is required")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	candidates := []string{
		path,
		filepath.Join("..", path),
		filepath.Join("..", "..", path),
		filepath.Join("..", "..", "..", path),
		filepath.Join("..", "..", "..", "..", path),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("contract content document file %q not found", path)
}

func parseContractSHACLShapesTTL(content string) ([]ContractSHACLShape, error) {
	shapes := []ContractSHACLShape{}
	for _, statement := range ontologyStatements(content) {
		if !contractSHACLStatementHasType(statement, "sh:NodeShape") {
			continue
		}
		targetClass := ontologyResource(statement, "sh:targetClass")
		if targetClass == "" {
			continue
		}
		subject := ontologySubject(statement)
		shape := ContractSHACLShape{
			ID:          subject,
			Title:       fmt.Sprintf("%s SHACL shape", compactTerm(targetClass)),
			Severity:    shaclSeverity(statement),
			TargetClass: targetClass,
			Properties:  parseContractSHACLPropertiesTTL(subject, statement),
		}
		shapes = append(shapes, shape)
	}
	return shapes, nil
}

func contractSHACLStatementHasType(statement string, class string) bool {
	expandedClass := expandOntologyResource(class)
	for _, line := range strings.Split(statement, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		var candidates []string
		switch {
		case fields[0] == "a":
			candidates = fields[1:]
		case len(fields) >= 3 && fields[1] == "a":
			candidates = fields[2:]
		default:
			continue
		}
		for _, rawClass := range candidates {
			candidate := strings.TrimSuffix(strings.TrimSuffix(rawClass, ";"), ",")
			if expandOntologyResource(candidate) == expandedClass {
				return true
			}
		}
	}
	return false
}

func parseContractSHACLPropertiesTTL(shapeID string, statement string) []ContractSHACLProperty {
	properties := []ContractSHACLProperty{}
	remaining := statement
	for {
		start := strings.Index(remaining, "sh:property [")
		if start < 0 {
			break
		}
		blockStart := start + len("sh:property [")
		end := strings.Index(remaining[blockStart:], "]")
		if end < 0 {
			break
		}
		block := remaining[blockStart : blockStart+end]
		index := len(properties)
		property := ContractSHACLProperty{
			ID:           fmt.Sprintf("%s-PROP-%03d", shapeID, index+1),
			Path:         ontologyResource(block, "sh:path"),
			Datatype:     ontologyResource(block, "sh:datatype"),
			Class:        ontologyResource(block, "sh:class"),
			Node:         ontologyResource(block, "sh:node"),
			Message:      ontologyString(block, "sh:message"),
			In:           parseContractSHACLInValues(block),
			MinCount:     ontologyInt(block, "sh:minCount"),
			MaxCount:     ontologyInt(block, "sh:maxCount"),
			MinInclusive: ontologyNumber(block, "sh:minInclusive"),
		}
		properties = append(properties, property)
		remaining = remaining[blockStart+end+1:]
	}
	return properties
}

func parseContractSHACLInValues(block string) []string {
	line := shaclPredicateLine(block, "sh:in")
	if line == "" {
		return nil
	}
	start := strings.Index(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end <= start {
		return nil
	}
	content := line[start+1 : end]
	values := []string{}
	quoted := regexp.MustCompile(`"([^"]*)"`)
	for _, match := range quoted.FindAllStringSubmatch(content, -1) {
		values = append(values, match[1])
	}
	content = quoted.ReplaceAllString(content, " ")
	for _, token := range strings.Fields(content) {
		values = append(values, strings.TrimSpace(token))
	}
	return values
}

func ontologyInt(statement string, predicate string) *int {
	for _, line := range strings.Split(statement, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), predicate+" ") {
			continue
		}
		match := ontologyNumberValue.FindString(line)
		if match == "" {
			return nil
		}
		value, err := strconv.Atoi(match)
		if err == nil {
			return &value
		}
	}
	return nil
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

func auditContractValidationProfile(contract map[string]any, profile ValidationProfile) []PolicyFinding {
	findings := []PolicyFinding{}
	statementRules := []ValidationRule{}
	for _, rule := range profile.Rules {
		if len(rule.Where) > 0 {
			statementRules = append(statementRules, rule)
			continue
		}
		findings = append(findings, auditContractValidationRule(contract, rule)...)
	}
	if len(statementRules) > 0 {
		findings = append(findings, auditContractStatementValidationRules(contract, ValidationProfile{
			ID:          profile.ID,
			Version:     profile.Version,
			Description: profile.Description,
			Rules:       statementRules,
		})...)
	}
	for i := range findings {
		findings[i].PolicySetID = profile.ID
		findings[i].PolicyVersion = profile.Version
	}
	return findings
}

func auditContractValidationRule(contract map[string]any, rule ValidationRule) []PolicyFinding {
	switch rule.Type {
	case ValidationRuleRequiredFields:
		findings := []PolicyFinding{}
		for _, field := range rule.RequiredFields {
			if value, ok := contractValue(contract, field); !ok || isEmptyAuditValue(value) {
				findings = append(findings, validationRuleFinding(rule, field, defaultSeverity(rule.Severity), fmt.Sprintf("required field %q is missing", field)))
			}
		}
		if len(findings) == 0 {
			return []PolicyFinding{validationRuleFinding(rule, strings.Join(rule.RequiredFields, ", "), "info", issueMessage(rule))}
		}
		return findings
	case ValidationRuleFieldValue:
		value, ok := contractValue(contract, rule.Target)
		if !ok || !compareValues(value, defaultOperator(rule.Operator, "eq"), rule.Value) {
			return []PolicyFinding{validationRuleFinding(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule))}
		}
		return []PolicyFinding{validationRuleFinding(rule, rule.Target, "info", issueMessage(rule))}
	case ValidationRuleComparison:
		value, ok := contractValue(contract, rule.Target)
		if !ok || !compareValues(value, rule.Operator, rule.Value) {
			return []PolicyFinding{validationRuleFinding(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule))}
		}
		return []PolicyFinding{validationRuleFinding(rule, rule.Target, "info", issueMessage(rule))}
	case ValidationRuleValueIn:
		value, ok := contractString(contract, rule.Target)
		if !ok || !normalizedSet(rule.Values)[strings.ToUpper(strings.TrimSpace(value))] {
			return []PolicyFinding{validationRuleFinding(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule))}
		}
		return []PolicyFinding{validationRuleFinding(rule, rule.Target, "info", issueMessage(rule))}
	case ValidationRuleSignatureLevel:
		value, ok := contractString(contract, rule.Target)
		required, _ := rule.Value.(string)
		if !ok || !signatureLevelSatisfies(value, required) {
			return []PolicyFinding{validationRuleFinding(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule))}
		}
		return []PolicyFinding{validationRuleFinding(rule, rule.Target, "info", issueMessage(rule))}
	default:
		return nil
	}
}

func auditContractStatementValidationRules(contract map[string]any, profile ValidationProfile) []PolicyFinding {
	statements := contractStatementsFromDocument(contract)
	if len(statements) == 0 {
		return nil
	}
	issues := ValidateContractStatements(statements, profile)
	findings := make([]PolicyFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, contractFinding(issue.RuleID, issue.RuleID, issue.Severity, issue.Message, issue.StatementID, issue.StatementID, "dcs:ContractStatement"))
	}
	return findings
}

func contractStatementsFromDocument(contract map[string]any) []map[string]any {
	statementSet, ok := contract[statementSetDocumentProperty()].(map[string]any)
	if !ok {
		return nil
	}
	rawStatements, ok := asArray(statementSet["statements"])
	if !ok {
		return nil
	}
	statements := make([]map[string]any, 0, len(rawStatements))
	for _, raw := range rawStatements {
		if statement, ok := raw.(map[string]any); ok {
			statements = append(statements, statement)
		}
	}
	return statements
}

func validationRuleFinding(rule ValidationRule, path string, severity string, message string) PolicyFinding {
	return contractFinding(rule.ID, rule.ID, severity, message, path, path, "")
}

func contractSHACLShapes(policy ContractContentPolicy) []ContractSHACLShape {
	shapes := make([]ContractSHACLShape, 0, len(policy.SHACLShapes))
	shapes = append(shapes, policy.SHACLShapes...)
	if policy.SHACL != nil {
		shapes = append(shapes, policy.SHACL.Shapes...)
	}
	return shapes
}

func contractSHACLShapeIndex(shapes []ContractSHACLShape) map[string]ContractSHACLShape {
	index := make(map[string]ContractSHACLShape, len(shapes))
	for _, shape := range shapes {
		normalized := normalizeContractSHACLShape(shape)
		index[normalized.ID] = normalized
		index[compactTerm(normalized.ID)] = normalized
	}
	return index
}

func isRootContractAuditShape(contract map[string]any, shape ContractSHACLShape) bool {
	targetClass := strings.TrimSpace(shape.TargetClass)
	if targetClass == "" {
		return true
	}
	if compactTerm(targetClass) == "Contract" {
		return true
	}
	return jsonLDTypeMatches(valuesAtPath(contract, "@type"), targetClass)
}

func auditContractSHACLShape(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape, shapeIndex map[string]ContractSHACLShape) []PolicyFinding {
	shape = normalizeContractSHACLShape(shape)
	if shape.TargetClass != "" && !jsonLDTypeMatches(valuesAtPath(contract, "@type"), shape.TargetClass) {
		return []PolicyFinding{contractStructureFinding(policy, shape.ID, shape.Title, shape.Severity, fmt.Sprintf("target class %q does not match contract @type", shape.TargetClass), "@type", shape.TargetClass)}
	}

	properties := shape.Properties
	properties = append(properties, shape.Property...)
	findings := []PolicyFinding{}
	for index, property := range properties {
		findings = append(findings, auditContractSHACLProperty(contract, policy, shape, normalizeContractSHACLProperty(shape, property, index), shapeIndex)...)
	}
	if len(findings) == 0 {
		findings = append(findings, contractStructureFinding(policy, shape.ID, shape.Title, "info", "SHACL shape conforms", shape.TargetClass, shape.TargetClass))
	}
	return findings
}

func auditContractSHACLProperty(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty, shapeIndex map[string]ContractSHACLShape) []PolicyFinding {
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
	if property.MinInclusive != nil {
		for _, value := range values {
			number, ok := toFloat(value)
			if !ok || number+floatTolerance < *property.MinInclusive {
				findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s must be at least %.4g", propertyLabel(property), *property.MinInclusive)))
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
	if property.Node != "" {
		if nestedShape, ok := shapeIndex[property.Node]; ok {
			for _, value := range values {
				nested, ok := value.(map[string]any)
				if !ok {
					findings = append(findings, shaclPropertyFinding(policy, shape, property, fmt.Sprintf("%s must be an object conforming to %s", propertyLabel(property), property.Node)))
					break
				}
				for _, nestedFinding := range auditContractSHACLShape(nested, policy, nestedShape, shapeIndex) {
					if nestedFinding.Severity == "info" {
						continue
					}
					if nestedFinding.Path != "" {
						nestedFinding.Path = property.Path + "." + nestedFinding.Path
						nestedFinding.SemanticPath = nestedFinding.Path
					}
					findings = append(findings, nestedFinding)
				}
			}
		}
	}
	if len(findings) == 0 {
		return []PolicyFinding{contractFinding(property.ID, shape.Title, "info", fmt.Sprintf("%s conforms", propertyLabel(property)), property.Path, property.Path, property.OntologyTerm)}
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
	return shape
}

func normalizeContractSHACLProperty(shape ContractSHACLShape, property ContractSHACLProperty, index int) ContractSHACLProperty {
	if strings.TrimSpace(property.ID) == "" {
		property.ID = fmt.Sprintf("%s-PROP-%03d", shape.ID, index+1)
	}
	if strings.TrimSpace(property.Severity) == "" {
		property.Severity = shape.Severity
	}
	if strings.TrimSpace(property.OntologyTerm) == "" {
		property.OntologyTerm = shape.OntologyTerm
	}
	return property
}

func contractStructureFinding(policy ContractContentPolicy, ruleID, title, severity, message, path, ontologyTerm string) PolicyFinding {
	finding := contractFinding(ruleID, title, severity, message, path, path, ontologyTerm)
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	return finding
}

func shaclPropertyFinding(policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty, fallbackMessage string) PolicyFinding {
	message := property.Message
	if strings.TrimSpace(message) == "" {
		message = fallbackMessage
	}
	finding := contractFinding(property.ID, shape.Title, property.Severity, message, property.Path, property.Path, property.OntologyTerm)
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	return finding
}

func normalizeContractContentRule(rule ContractContentPolicyRule) ContractContentPolicyRule {
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = "FACIS-CONTRACT-POLICY-CUSTOM"
	}
	if strings.TrimSpace(rule.Title) == "" {
		rule.Title = rule.ID
	}
	if strings.TrimSpace(rule.Severity) == "" {
		rule.Severity = "error"
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
	allowedValues := rule.Values
	if len(allowedValues) == 0 && strings.TrimSpace(rule.ValuesRef) != "" {
		allowedValues = allowedValuesForConstraint(&valueConstraint{AllowedValuesRef: rule.ValuesRef})
	}
	allowed := normalizedSet(allowedValues)
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
	return contractFinding(rule.ID, rule.Title, severity, message, rule.SemanticPath, rule.SemanticPath, rule.OntologyTerm)
}

func contractFinding(ruleID, title, severity, message, path, semanticPath, ontologyTerm string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
		SemanticPath: semanticPath,
		OntologyTerm: ontologyTerm,
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
	if value, ok := contractSHACLAliasValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
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

func contractSHACLAliasValue(contract map[string]any, semanticPath string) (any, bool) {
	switch compactTerm(semanticPath) {
	case "did":
		return firstExistingValue(contract, "@id", "did", "dcs:did")
	case "party":
		return firstExistingValue(contract, "party", "dcs:party", "parties")
	}
	return nil, false
}

func firstExistingValue(contract map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := contract[key]; ok {
			return value, true
		}
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
		if equivalentSemanticPath(pathValue, semanticPath) {
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

func validIRIOrURN(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "urn:")
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
