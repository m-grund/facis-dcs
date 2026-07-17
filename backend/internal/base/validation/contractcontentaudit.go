package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ContractContentAuditMetadata struct {
	ContractDID     string
	ContractVersion string
	PolicyVersion   string
	AuditedBy       string
	HolderDID       string
}

type ContractContentPolicy struct {
	PolicySetID string `json:"policySetId"`
	Version     string `json:"version"`
	Policies    []any  `json:"dcs:policies"`
	// EnforceCanonicalShapes/EnforceValidationProfile opt a given audit call
	// into the Semantic Hub's canonical SHACL shapes / SLA validation
	// profile (the default disk policy document sets both; ad-hoc/test
	// policies that want to exercise only ODRL evaluation leave them unset).
	// The hub is the only source for their content — there is no
	// alternative/inline shape format anymore (ADR-8, ADR-9).
	EnforceCanonicalShapes   bool `json:"enforceCanonicalShapes"`
	EnforceValidationProfile bool `json:"enforceValidationProfile"`
	profiles                 []ValidationProfile
	// ShapesVersion/ProfileVersion record which hub version this audit ran
	// against (the pinned version for revalidation, or the currently-active
	// one for newly produced documents) — ADR-8.
	ShapesVersion  int `json:"-"`
	ProfileVersion int `json:"-"`
}

// AuditContractContent checks a produced contract document against its
// governing policies: the Semantic Hub's SHACL shapes (goRDFlib, ADR-9,
// version-pinned per ADR-8), the SLA validation profile, and the contract's
// own embedded ODRL policies.
func AuditContractContent(ctx context.Context, contractDocument any, policyDocument any, metadata ContractContentAuditMetadata) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	policy, err := normalizeContractContentPolicy(ctx, policyDocument, metadata)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.PolicyVersion) != "" {
		policy.Version = metadata.PolicyVersion
	}

	source, err := requireShapeSource()
	if err != nil {
		return nil, err
	}
	// The domain-field ontology backs value normalization during profile
	// audits (compactEntityRole); loading it here hard-fails the audit when
	// the hub cannot serve it.
	if _, err := requireDomainOntology(ctx); err != nil {
		return nil, err
	}

	findings := []PolicyFinding{}
	if policy.EnforceCanonicalShapes {
		shaclFindings, shapesVersion, err := validateAgainstShapeSource(ctx, contract, source)
		if err != nil {
			return nil, fmt.Errorf("SHACL validation: %w", err)
		}
		policy.ShapesVersion = shapesVersion
		findings = append(findings, shaclFindings...)
	}
	root, err := expandForAudit(ctx, contract, source)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion: %w", err)
	}

	for _, profile := range policy.profiles {
		findings = append(findings, auditContractValidationProfile(contract, root, profile)...)
	}
	findings = append(findings, auditExpandedODRLPolicies(root, expandedODRLPolicyRules(root))...)
	externalRules, err := expandExternalODRLRules(ctx, externalODRLPolicies(policy.Policies), source)
	if err != nil {
		return nil, err
	}
	findings = append(findings, auditExpandedODRLPolicies(root, externalRules)...)

	for i := range findings {
		findings[i].PolicySetID = policy.PolicySetID
		findings[i].PolicyVersion = policy.Version
	}
	return findings, nil
}

type ContractPolicySatisfactionError struct {
	Findings []PolicyFinding
}

func (e ContractPolicySatisfactionError) Error() string {
	if len(e.Findings) == 0 {
		return "contract policy validation failed"
	}
	messages := make([]string, 0, len(e.Findings))
	for _, finding := range e.Findings {
		message := strings.TrimSpace(finding.Message)
		if message == "" {
			message = strings.TrimSpace(finding.RuleID)
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return "contract policy validation failed"
	}
	return "contract policy validation failed: " + strings.Join(messages, "; ")
}

// ValidateContractPolicySatisfaction enforces the per-contract ODRL policies
// embedded as dcs:policies against the submitted semanticConditionValues.
func ValidateContractPolicySatisfaction(contractDocument any, metadata ContractContentAuditMetadata) error {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return fmt.Errorf("decode contract document: %w", err)
	}
	source, err := requireShapeSource()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := expandForAudit(ctx, contract, source)
	if err != nil {
		return fmt.Errorf("ODRL evaluation: %w", err)
	}
	findings := auditExpandedODRLPolicies(root, expandedODRLPolicyRules(root))
	blocking := make([]PolicyFinding, 0)
	for _, finding := range findings {
		if isBlockingContractPolicyFinding(finding) {
			finding.PolicySetID = defaultContractPolicySetID
			finding.PolicyVersion = metadata.PolicyVersion
			if strings.TrimSpace(finding.PolicyVersion) == "" {
				finding.PolicyVersion = defaultContractPolicyVersion
			}
			blocking = append(blocking, finding)
		}
	}
	if len(blocking) > 0 {
		return ContractPolicySatisfactionError{Findings: blocking}
	}
	return nil
}

func isBlockingContractPolicyFinding(finding PolicyFinding) bool {
	switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
	case "error", "blocking":
		return true
	default:
		return false
	}
}

func normalizeContractContentPolicy(ctx context.Context, raw any, metadata ContractContentAuditMetadata) (ContractContentPolicy, error) {
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

	// EnforceCanonicalShapes drives validateAgainstHubShapes (called from
	// AuditContractContent, where the document being audited — needed for
	// ADR-8 version pinning — is in scope). Here, only the validation
	// profile: content always comes from the hub (ADR-8/ADR-9 — no disk
	// fallback), the currently-active version (profile pinning is not part
	// of the ShapeSource contract, unlike shapes).
	if policy.EnforceValidationProfile {
		profileContent, profileVersion, err := activeShapeSource.ActiveProfile(ctx)
		if err != nil {
			return ContractContentPolicy{}, fmt.Errorf("load validation profile: %w", err)
		}
		hubProfile, err := LoadValidationProfileYAML([]byte(profileContent))
		if err != nil {
			return ContractContentPolicy{}, fmt.Errorf("parse validation profile (hub version %d): %w", profileVersion, err)
		}
		policy.profiles = append(policy.profiles, hubProfile)
		policy.ProfileVersion = profileVersion
	}

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

func auditContractValidationProfile(contract map[string]any, root map[string]any, profile ValidationProfile) []PolicyFinding {
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
		findings = append(findings, auditContractStatementValidationRules(root, ValidationProfile{
			ID:          profile.ID,
			Version:     profile.Version,
			Description: profile.Description,
			AppliesTo:   profile.AppliesTo,
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
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), rule.Value, nil, defaultOperator(rule.Operator, "eq"))}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, rule.Value, nil, defaultOperator(rule.Operator, "eq"))}
	case ValidationRuleComparison:
		value, ok := contractValue(contract, rule.Target)
		if !ok || !compareValues(value, rule.Operator, rule.Value) {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), rule.Value, nil, rule.Operator)}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, rule.Value, nil, rule.Operator)}
	case ValidationRuleValueIn:
		value, ok := contractString(contract, rule.Target)
		if !ok || !normalizedSet(rule.Values)[strings.ToUpper(strings.TrimSpace(value))] {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), nil, anySliceFromStrings(rule.Values), "in")}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, nil, anySliceFromStrings(rule.Values), "in")}
	case ValidationRuleSignatureLevel:
		value, ok := contractString(contract, rule.Target)
		required, _ := rule.Value.(string)
		if !ok || !signatureLevelSatisfies(value, required) {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), required, nil, "atLeast")}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, required, nil, "atLeast")}
	default:
		return nil
	}
}

func auditContractStatementValidationRules(root map[string]any, profile ValidationProfile) []PolicyFinding {
	statements := expandedStatements(root)
	if len(statements) == 0 || !statementsCoverProfile(statements, profile.AppliesTo) {
		return nil
	}
	issues := ValidateContractStatements(statements, profile)
	findings := make([]PolicyFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, contractFinding(issue.RuleID, issue.RuleID, issue.Severity, issue.Message, issue.StatementID, "dcs:ContractStatement"))
	}
	return findings
}

func validationRuleFinding(rule ValidationRule, path string, severity string, message string) PolicyFinding {
	return validationRuleFindingWithDetails(rule, path, severity, message, nil, nil, nil, "")
}

func validationRuleFindingWithDetails(rule ValidationRule, path string, severity string, message string, actualValue any, expectedValue any, expectedValues []any, operator string) PolicyFinding {
	finding := contractFinding(rule.ID, rule.ID, severity, message, path, "")
	if len(rule.RequiredFields) > 0 && operator == "" {
		operator = "exists"
		expectedValues = anySliceFromStrings(rule.RequiredFields)
	}
	applyPolicyDetails(&finding, path, operator, actualValue, expectedValue, expectedValues)
	return finding
}

func applyODRLPolicyDetails(finding *PolicyFinding, path string, operator string, actualValue any, hasActualValue bool, rightOperand any) {
	expectedValue, expectedValues := odrlExpectedValues(rightOperand)
	if !hasActualValue {
		actualValue = nil
	}
	applyPolicyDetails(finding, path, operator, actualValue, expectedValue, expectedValues)
}

func applyPolicyDetails(finding *PolicyFinding, path string, operator string, actualValue any, expectedValue any, expectedValues []any) {
	normalizedOperator := normalizePolicyOperator(operator)
	if normalizedOperator != "" {
		finding.Operator = normalizedOperator
	}
	if actualValue != nil {
		finding.ActualValue = compactAuditValue(actualValue)
	}
	if expectedValue != nil {
		finding.ExpectedValue = compactAuditValue(expectedValue)
	}
	if len(expectedValues) > 0 {
		finding.ExpectedValues = compactAuditValues(expectedValues)
	}
	if strings.TrimSpace(finding.Requirement) == "" {
		finding.Requirement = policyRequirement(path, normalizedOperator, finding.ExpectedValue, finding.ExpectedValues)
	}
}

func odrlExpectedValues(rightOperand any) (any, []any) {
	value := compactAuditValue(rightOperand)
	if items, ok := value.([]any); ok {
		return nil, items
	}
	return value, nil
}

func policyRequirement(path string, operator string, expectedValue any, expectedValues []any) string {
	path = strings.TrimSpace(path)
	if path == "" || operator == "" {
		return ""
	}
	switch operator {
	case "exists":
		if len(expectedValues) > 0 {
			return fmt.Sprintf("%s must contain %s", path, formatRequirementValues(expectedValues))
		}
		return fmt.Sprintf("%s must be present", path)
	case "in":
		return fmt.Sprintf("%s must be one of %s", path, formatRequirementValues(expectedValues))
	case "notIn":
		return fmt.Sprintf("%s must be none of %s", path, formatRequirementValues(expectedValues))
	case "minCount":
		return fmt.Sprintf("%s requires at least %s value(s)", path, formatRequirementValue(expectedValue))
	case "maxCount":
		return fmt.Sprintf("%s allows at most %s value(s)", path, formatRequirementValue(expectedValue))
	case "datatype":
		return fmt.Sprintf("%s must use datatype %s", path, formatRequirementValue(expectedValue))
	case "class":
		return fmt.Sprintf("%s must reference class %s", path, formatRequirementValue(expectedValue))
	case "node":
		return fmt.Sprintf("%s must conform to %s", path, formatRequirementValue(expectedValue))
	case "atLeast":
		return fmt.Sprintf("%s must be at least %s", path, formatRequirementValue(expectedValue))
	case "eq":
		return fmt.Sprintf("%s must equal %s", path, formatRequirementValue(expectedValue))
	case "neq":
		return fmt.Sprintf("%s must not equal %s", path, formatRequirementValue(expectedValue))
	case "contains":
		return fmt.Sprintf("%s must contain %s", path, formatRequirementValue(expectedValue))
	default:
		if symbol := policyOperatorSymbol(operator); symbol != "" {
			return fmt.Sprintf("%s must be %s %s", path, symbol, formatRequirementValue(expectedValue))
		}
		return fmt.Sprintf("%s must satisfy %s %s", path, operator, formatRequirementValue(expectedValue))
	}
}

// normalizePolicyOperator maps an operator term to its internal evaluation
// name. Accepted vocabulary: the ODRL core constraint operators
// (odrl:eq/neq/gt/lt/gteq/lteq/isAnyOf/isNoneOf, compacted or full IRI) and
// the validation-profile rule operators; anything else passes through
// verbatim and fails evaluation.
func normalizePolicyOperator(operator string) string {
	switch strings.ToLower(strings.TrimSpace(compactTerm(operator))) {
	case "":
		return ""
	case "gte", "gteq":
		return "gte"
	case "lte", "lteq":
		return "lte"
	case "gt":
		return "gt"
	case "lt":
		return "lt"
	case "eq":
		return "eq"
	case "neq":
		return "neq"
	case "isanyof", "in":
		return "in"
	case "isnoneof", "notin":
		return "notIn"
	case "contains":
		return "contains"
	case "mincount":
		return "minCount"
	case "maxcount":
		return "maxCount"
	case "datatype":
		return "datatype"
	case "class":
		return "class"
	case "node":
		return "node"
	case "exists":
		return "exists"
	case "atleast":
		return "atLeast"
	default:
		return strings.TrimSpace(compactTerm(operator))
	}
}

func policyOperatorSymbol(operator string) string {
	switch operator {
	case "gte":
		return ">="
	case "lte":
		return "<="
	case "gt":
		return ">"
	case "lt":
		return "<"
	default:
		return ""
	}
}

func formatRequirementValues(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatRequirementValue(value))
	}
	return strings.Join(parts, ", ")
}

func formatRequirementValue(value any) string {
	switch typed := compactAuditValue(value).(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		return formatRequirementValues(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func optionalActualValue(value any, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func anySliceFromStrings(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func compactAuditValues(values []any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, compactAuditValue(value))
	}
	return result
}

func compactAuditValue(value any) any {
	value = compactJSONLDValue(value)
	switch typed := value.(type) {
	case []any:
		return compactAuditValues(typed)
	default:
		return typed
	}
}

func externalODRLPolicies(raw []any) []map[string]any {
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if policy, ok := item.(map[string]any); ok {
			result = append(result, policy)
		}
	}
	return result
}

type odrlFieldInfo struct {
	parameterName string
}

func evaluateODRLConstraint(operator string, actualValue any, rightOperand any) bool {
	op := compactTerm(operator)
	actualValue = compactJSONLDValue(actualValue)
	rightOperand = compactJSONLDValue(rightOperand)
	switch op {
	case "eq":
		return odrlValuesEqual(actualValue, rightOperand)
	case "neq":
		return !odrlValuesEqual(actualValue, rightOperand)
	case "gt":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 > f2+floatTolerance
	case "gteq":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1+floatTolerance >= f2
	case "lt":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 < f2-floatTolerance
	case "lteq":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 <= f2+floatTolerance
	case "isAnyOf":
		items, ok := asArray(rightOperand)
		if !ok {
			return false
		}
		normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
		for _, item := range items {
			if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) == normalized {
				return true
			}
		}
		return false
	case "isNoneOf":
		items, ok := asArray(rightOperand)
		if !ok {
			return true
		}
		normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
		for _, item := range items {
			if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) == normalized {
				return false
			}
		}
		return true
	case "hasPart":
		str, ok := actualValue.(string)
		if !ok {
			return false
		}
		return strings.Contains(str, fmt.Sprint(compactJSONLDValue(rightOperand)))
	default:
		return false
	}
}

func odrlValuesEqual(a, b any) bool {
	a = compactJSONLDValue(a)
	b = compactJSONLDValue(b)
	sa, saOk := a.(string)
	sb, sbOk := b.(string)
	if saOk && sbOk {
		return strings.EqualFold(sa, sb)
	}
	fa, faOk := toFloat(a)
	fb, fbOk := toFloat(b)
	if faOk && fbOk {
		return math.Abs(fa-fb) <= floatTolerance
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func contractFinding(ruleID, title, severity, message, path, ontologyTerm string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
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

// contractValue resolves a validation-profile rule target (a document
// property path like "contract.jurisdiction", or a declared
// dcs:parameterName) to the document's runtime value.
func contractValue(contract map[string]any, target string) (any, bool) {
	if value, ok := contractSHACLAliasValue(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := nestedValue(contract, strings.Split(target, ".")); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := semanticConditionValuesByParameterName(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveExactKeyValue(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	return nil, false
}

func contractSHACLAliasValue(contract map[string]any, target string) (any, bool) {
	switch compactTerm(target) {
	case "did":
		return firstExistingValue(contract, "@id", "did", "dcs:did")
	case "party":
		if value, ok := firstExistingValue(contract, "party", "dcs:party", "parties"); ok {
			return value, true
		}
		return companyPartiesFromSemanticValues(contract)
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

// valueForField resolves a semanticConditionValues entry's dcs:forField
// reference (an IRI string once normalized, a node reference beforehand).
func valueForField(value map[string]any) string {
	switch forField := value["forField"].(type) {
	case string:
		return forField
	case map[string]any:
		iri, _ := forField["@id"].(string)
		return iri
	}
	return ""
}

func semanticConditionValuesByParameterName(contract map[string]any, parameterName string) (any, bool) {
	values, ok := asArray(contract["semanticConditionValues"])
	if !ok {
		return nil, false
	}
	fields := contractDataFieldsByID(contract)
	matches := []any{}
	for _, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok {
			continue
		}
		field := fields[valueForField(value)]
		if field.parameterName != parameterName {
			continue
		}
		parameterValue, exists := value["parameterValue"]
		if exists && !isEmptyAuditValue(parameterValue) {
			matches = append(matches, parameterValue)
		}
	}
	if len(matches) == 0 {
		return nil, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return matches, true
}

func companyPartiesFromSemanticValues(contract map[string]any) ([]any, bool) {
	values, ok := asArray(contract["semanticConditionValues"])
	if !ok {
		return nil, false
	}
	requirements := contractDataRequirementsByConditionID(contract)
	fields := contractDataFieldsByID(contract)
	partiesByCondition := map[string]map[string]any{}
	order := []string{}
	for _, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok {
			continue
		}
		field := fields[valueForField(value)]
		conditionID := field.conditionID
		parameterName := field.parameterName
		if !strings.HasPrefix(parameterName, "company.") {
			continue
		}
		requirement := requirements[conditionID]
		if !isCompanyPartyRequirement(requirement) && parameterName != "company.role" {
			continue
		}
		party := partiesByCondition[conditionID]
		if party == nil {
			party = map[string]any{
				"@type": "dcs:CompanyParty",
			}
			if role := companyPartyRole(requirement); role != "" {
				party["role"] = role
			}
			partiesByCondition[conditionID] = party
			order = append(order, conditionID)
		}
		parameterValue := compactJSONLDValue(value["parameterValue"])
		switch parameterName {
		case "company.legalName":
			party["legalName"] = parameterValue
			party["dcs:legalName"] = parameterValue
		case "company.role":
			if text, ok := parameterValue.(string); ok {
				party["role"] = compactEntityRole(text)
			}
		}
	}
	if len(order) == 0 {
		return nil, false
	}
	parties := make([]any, 0, len(order))
	for _, conditionID := range order {
		parties = append(parties, partiesByCondition[conditionID])
	}
	return parties, true
}

func contractDataRequirementsByConditionID(contract map[string]any) map[string]map[string]any {
	requirements := map[string]map[string]any{}
	collectContractDataRequirements(contract, requirements)
	return requirements
}

type contractFieldInfo struct {
	conditionID   string
	parameterName string
}

// contractDataFieldsByID indexes the document's declared requirement fields
// by their @id — the IRI a semanticConditionValues entry's dcs:forField and
// an ODRL constraint's odrl:leftOperand both reference.
func contractDataFieldsByID(contract map[string]any) map[string]contractFieldInfo {
	fields := map[string]contractFieldInfo{}
	for conditionID, requirement := range contractDataRequirementsByConditionID(contract) {
		rawFields, ok := asArray(firstOf(requirement, "dcs:fields", "fields"))
		if !ok {
			continue
		}
		for _, rawField := range rawFields {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			fieldID, _ := field["@id"].(string)
			if fieldID == "" {
				continue
			}
			parameterName, _ := firstOf(field, "dcs:parameterName", "parameterName").(string)
			fields[fieldID] = contractFieldInfo{conditionID: conditionID, parameterName: parameterName}
		}
	}
	return fields
}

func firstOf(node map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := node[key]; ok {
			return value
		}
	}
	return nil
}

func collectContractDataRequirements(current any, requirements map[string]map[string]any) {
	switch value := current.(type) {
	case map[string]any:
		if rawRequirements, ok := topLevelValue(documentData(value), "contractData").([]any); ok {
			for _, rawRequirement := range rawRequirements {
				requirement, ok := rawRequirement.(map[string]any)
				if !ok {
					continue
				}
				conditionID, _ := requirement["dcs:conditionId"].(string)
				if conditionID == "" {
					conditionID, _ = requirement["conditionId"].(string)
				}
				if conditionID != "" {
					requirements[conditionID] = requirement
				}
			}
		}
		for _, nested := range value {
			collectContractDataRequirements(nested, requirements)
		}
	case []any:
		for _, nested := range value {
			collectContractDataRequirements(nested, requirements)
		}
	}
}

func isCompanyPartyRequirement(requirement map[string]any) bool {
	entityType, _ := requirement["dcs:entityType"].(string)
	if entityType == "" {
		entityType, _ = requirement["entityType"].(string)
	}
	return compactTerm(entityType) == "CompanyParty"
}

func companyPartyRole(requirement map[string]any) string {
	role, _ := requirement["dcs:entityRole"].(string)
	if role == "" {
		role, _ = requirement["entityRole"].(string)
	}
	return compactEntityRole(role)
}

func contractString(contract map[string]any, target string) (string, bool) {
	value, ok := contractValue(contract, target)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
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
