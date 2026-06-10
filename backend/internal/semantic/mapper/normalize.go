package mapper

import (
	"fmt"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
)

// NormalizeSemanticTemplateData extends validation.NormalizeTemplateDataForPersistence
// with additional semantic normalization:
//   - Canonicalizes legacy operator strings (e.g. "greaterThanOrEqual" →
//     "GreaterThanOrEqual") in the sla block's SLO and MeasurementRule operators.
//
// The base normalization already handles semanticConditions parameter operators,
// placeholderBindings, semanticRules, semanticProfile, and @context/@type injection.
// This function adds the sla-specific operator pass.
func NormalizeSemanticTemplateData(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	normalized, err := validation.NormalizeTemplateDataForPersistence(raw, did)
	if err != nil {
		return nil, fmt.Errorf("base template normalization: %w", err)
	}
	data, err := parseJSONB(normalized)
	if err != nil {
		return nil, fmt.Errorf("re-parse normalized template data: %w", err)
	}
	normalizeSLAOperators(data)
	return encodeMap(data)
}

// NormalizeSemanticContractData extends validation.NormalizeContractDataForPersistence
// with sla operator canonicalization. When requireSemanticValues is false, required
// semantic values may be absent (draft-creation from a template).
func NormalizeSemanticContractData(raw *datatype.JSON, did string, requireSemanticValues bool) (*datatype.JSON, error) {
	normalized, err := validation.NormalizeContractDataForPersistence(raw, did, requireSemanticValues)
	if err != nil {
		return nil, fmt.Errorf("base contract normalization: %w", err)
	}
	data, err := parseJSONB(normalized)
	if err != nil {
		return nil, fmt.Errorf("re-parse normalized contract data: %w", err)
	}
	normalizeSLAOperators(data)
	return encodeMap(data)
}

// normalizeSLAOperators canonicalizes legacy operator strings in the sla block.
// It walks sla.services[].slos[].operator and sla.services[].slos[].measurementRules[].operator.
// Fields are mutated in place; missing or non-string operators are left unchanged.
func normalizeSLAOperators(data map[string]any) {
	sla, ok := data["sla"].(map[string]any)
	if !ok {
		return
	}
	services, _ := sla["services"].([]any)
	for _, rawService := range services {
		service, ok := rawService.(map[string]any)
		if !ok {
			continue
		}
		slos, _ := service["slos"].([]any)
		for _, rawSLO := range slos {
			slo, ok := rawSLO.(map[string]any)
			if !ok {
				continue
			}
			normalizeOperatorField(slo, "operator")
			rules, _ := slo["measurementRules"].([]any)
			for _, rawRule := range rules {
				rule, ok := rawRule.(map[string]any)
				if !ok {
					continue
				}
				normalizeOperatorField(rule, "operator")
			}
		}
	}
}

// normalizeOperatorField canonicalizes the operator string at m[field] in place.
func normalizeOperatorField(m map[string]any, field string) {
	op, ok := m[field].(string)
	if !ok {
		return
	}
	if canonical := canonicalizeOperator(op); canonical != "" {
		m[field] = canonical
	}
}

// canonicalizeOperator normalizes a DCS operator string to its canonical PascalCase form.
// Both the already-canonical form and the legacy camelCase form are accepted.
// Returns an empty string for unknown operator values.
//
// Operator mapping (matches docs/semantic-ontology/README.md §6 and TypeScript
// LEGACY_OPERATOR_TO_DCS in facis-dcs-semantic.ts):
//
//	equal             → Equals
//	notEqual          → NotEquals
//	greaterThan       → GreaterThan
//	greaterThanOrEqual → GreaterThanOrEqual
//	lessThan          → LessThan
//	lessThanOrEqual   → LessThanOrEqual
//	between           → Between
//	contains          → Contains
//	matchesRegex      → MatchesRegex
func canonicalizeOperator(value string) string {
	switch value {
	case "Equals", "NotEquals", "GreaterThan", "GreaterThanOrEqual",
		"LessThan", "LessThanOrEqual", "Between", "Contains", "MatchesRegex":
		return value
	case "equal":
		return "Equals"
	case "notEqual":
		return "NotEquals"
	case "greaterThan":
		return "GreaterThan"
	case "greaterThanOrEqual":
		return "GreaterThanOrEqual"
	case "lessThan":
		return "LessThan"
	case "lessThanOrEqual":
		return "LessThanOrEqual"
	case "between":
		return "Between"
	case "contains":
		return "Contains"
	case "matchesRegex":
		return "MatchesRegex"
	default:
		return ""
	}
}
