package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piprate/json-gold/ld"
)

const (
	defaultDCSOntologyIRI = "https://w3id.org/facis/dcs/ontology/v1#"
	odrlIRI               = "http://www.w3.org/ns/odrl/2/"
)

// dcsNamespace returns the dcs: ontology namespace the active hub context
// declares; the historical default applies until the hub anchors are
// installed.
func dcsNamespace() string {
	if iri, ok := canonicalOntologyIRIs["dcs"]; ok && iri != "" {
		return iri
	}
	return defaultDCSOntologyIRI
}

// expandForAudit returns the json-gold expansion of a document, resolved
// hermetically through the ShapeSource. The hub context is merged in as the
// outermost context so prefix-form keys resolve to full IRIs even when the
// document declares only a subset of the hub prefixes (conflicting
// redefinitions are already rejected at creation).
func expandForAudit(ctx context.Context, document map[string]any, source ShapeSource) (map[string]any, error) {
	var contextContent string
	var err error
	if pinned := pinnedHubContextVersion(document); pinned > 0 {
		contextContent, err = source.ContextAt(ctx, pinned)
	} else {
		contextContent, _, err = source.ActiveContext(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("load JSON-LD context: %w", err)
	}
	var hubContextDoc map[string]any
	if err := json.Unmarshal([]byte(contextContent), &hubContextDoc); err != nil {
		return nil, fmt.Errorf("parse hub JSON-LD context: %w", err)
	}

	input := make(map[string]any, len(document))
	for key, value := range document {
		input[key] = value
	}
	mergedContext := []any{hubContextDoc["@context"]}
	switch docContext := document["@context"].(type) {
	case []any:
		mergedContext = append(mergedContext, docContext...)
	case nil:
	default:
		mergedContext = append(mergedContext, docContext)
	}
	input["@context"] = mergedContext

	loader, err := hermeticContextLoader(ctx, contextContent, source)
	if err != nil {
		return nil, err
	}
	options := ld.NewJsonLdOptions("")
	options.DocumentLoader = loader
	expanded, err := ld.NewJsonLdProcessor().Expand(input, options)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion: %w", err)
	}
	if len(expanded) == 0 {
		return map[string]any{}, nil
	}
	root, ok := expanded[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JSON-LD expansion: unexpected root node %T", expanded[0])
	}
	return root, nil
}

// expandedValues returns a property's value nodes on an expanded node.
func expandedValues(node map[string]any, property string) []any {
	values, _ := node[property].([]any)
	return values
}

func expandedFirst(node map[string]any, property string) (map[string]any, bool) {
	for _, value := range expandedValues(node, property) {
		if obj, ok := value.(map[string]any); ok {
			return obj, true
		}
	}
	return nil, false
}

func expandedID(value any) string {
	if obj, ok := value.(map[string]any); ok {
		id, _ := obj["@id"].(string)
		return id
	}
	return ""
}

// expandedLiteral unwraps an expanded value node to a plain Go value.
func expandedLiteral(value any) any {
	obj, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if nested, ok := obj["@value"]; ok {
		return nested
	}
	if id, ok := obj["@id"].(string); ok {
		return id
	}
	if list, ok := obj["@list"].([]any); ok {
		return expandedLiterals(list)
	}
	return value
}

func expandedLiterals(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, expandedLiteral(value))
	}
	return out
}

func expandedFirstLiteralString(node map[string]any, property string) string {
	for _, value := range expandedValues(node, property) {
		if s, ok := expandedLiteral(value).(string); ok {
			return s
		}
	}
	return ""
}

// expandedTypeLocalName returns the local name of the node's first @type.
func expandedTypeLocalName(node map[string]any) string {
	if types, ok := node["@type"].([]any); ok {
		for _, t := range types {
			if iri, ok := t.(string); ok {
				return shaclLocalName(iri)
			}
		}
	}
	return ""
}

var odrlRuleBucketIRIs = []string{
	odrlIRI + "permission",
	odrlIRI + "prohibition",
	odrlIRI + "obligation",
}

// expandedODRLPolicyRules flattens the document's policies into rule nodes:
// a single enclosing policy (odrl:Offer on templates, odrl:Agreement on
// contracts) whose rules live under permission/prohibition/obligation.
func expandedODRLPolicyRules(root map[string]any) []map[string]any {
	rules := []map[string]any{}
	for _, rawSet := range expandedValues(root, dcsNamespace()+"policies") {
		set, ok := rawSet.(map[string]any)
		if !ok {
			continue
		}
		for _, bucket := range odrlRuleBucketIRIs {
			for _, rawRule := range expandedValues(set, bucket) {
				if rule, ok := rawRule.(map[string]any); ok {
					rules = append(rules, rule)
				}
			}
		}
	}
	return rules
}

// expandedODRLFieldIndex maps requirement-field @ids to the condition and
// parameter their values are submitted under.
func expandedODRLFieldIndex(root map[string]any) map[string]odrlFieldInfo {
	index := map[string]odrlFieldInfo{}
	for _, rawReq := range expandedValues(root, dcsNamespace()+"contractData") {
		req, ok := rawReq.(map[string]any)
		if !ok {
			continue
		}
		conditionID := expandedFirstLiteralString(req, dcsNamespace()+"conditionId")
		for _, rawField := range expandedValues(req, dcsNamespace()+"fields") {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			fieldID, _ := field["@id"].(string)
			if fieldID == "" {
				continue
			}
			index[fieldID] = odrlFieldInfo{
				conditionID:   conditionID,
				parameterName: expandedFirstLiteralString(field, dcsNamespace()+"parameterName"),
			}
		}
	}
	return index
}

// expandedSemanticConditionValue looks up a submitted semantic value by
// (conditionId, parameterName).
func expandedSemanticConditionValue(root map[string]any, conditionID, parameterName string) (any, bool) {
	for _, rawEntry := range expandedValues(root, dcsNamespace()+"semanticConditionValues") {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		if expandedFirstLiteralString(entry, dcsNamespace()+"conditionId") != conditionID {
			continue
		}
		if expandedFirstLiteralString(entry, dcsNamespace()+"parameterName") != parameterName {
			continue
		}
		values := expandedValues(entry, dcsNamespace()+"parameterValue")
		if len(values) == 0 {
			return nil, false
		}
		if len(values) == 1 {
			value := expandedLiteral(values[0])
			return value, value != nil
		}
		return expandedLiterals(values), true
	}
	return nil, false
}

// auditExpandedODRLPolicies evaluates every ODRL rule node against the
// document's submitted semantic values.
func auditExpandedODRLPolicies(root map[string]any, rules []map[string]any) []PolicyFinding {
	if len(rules) == 0 {
		return nil
	}
	fieldIndex := expandedODRLFieldIndex(root)
	findings := []PolicyFinding{}
	for _, rule := range rules {
		findings = append(findings, auditExpandedODRLRule(root, rule, fieldIndex)...)
	}
	return findings
}

func auditExpandedODRLRule(root map[string]any, rule map[string]any, fieldIndex map[string]odrlFieldInfo) []PolicyFinding {
	ruleID, _ := rule["@id"].(string)
	if ruleID == "" {
		ruleID = "FACIS-CONTRACT-ODRL-POLICY"
	}
	policyType := expandedTypeLocalName(rule)

	constraint, ok := expandedFirst(rule, odrlIRI+"constraint")
	if !ok {
		return nil
	}
	leftOperand, ok := expandedFirst(constraint, odrlIRI+"leftOperand")
	if !ok {
		return nil
	}
	fieldID, _ := leftOperand["@id"].(string)
	if fieldID == "" {
		return nil
	}
	operatorNode, ok := expandedFirst(constraint, odrlIRI+"operator")
	if !ok {
		return nil
	}
	operator := shaclLocalName(expandedID(operatorNode))
	if operator == "" {
		return nil
	}
	rightOperand := expandedRightOperand(constraint, operator)

	fieldInfo, ok := fieldIndex[fieldID]
	if !ok {
		finding := contractFinding(ruleID, ruleID, "error", fmt.Sprintf("ODRL policy %q references nonexistent contract data field %q", ruleID, fieldID), fieldID, fieldID, "dcs:RequirementField")
		applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
		return []PolicyFinding{finding}
	}
	actualValue, hasValue := expandedSemanticConditionValue(root, fieldInfo.conditionID, fieldInfo.parameterName)

	isProhibition := policyType == "Prohibition"
	isPermission := policyType == "Permission"
	severity := "error"
	if isPermission {
		severity = "info"
	}

	if !hasValue {
		if isProhibition || isPermission {
			finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q: value not present", ruleID), fieldID, fieldID, "")
			applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
			return []PolicyFinding{finding}
		}
		finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q: required value not provided", ruleID), fieldID, fieldID, "")
		applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
		return []PolicyFinding{finding}
	}

	satisfied := evaluateODRLConstraint(operator, actualValue, rightOperand)
	violated := (isProhibition && satisfied) || (!isProhibition && !isPermission && !satisfied)

	if violated {
		finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q violated: value %v does not satisfy %s", ruleID, actualValue, operator), fieldID, fieldID, "")
		applyODRLPolicyDetails(&finding, fieldID, operator, actualValue, true, rightOperand)
		return []PolicyFinding{finding}
	}
	finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q satisfied", ruleID), fieldID, fieldID, "")
	applyODRLPolicyDetails(&finding, fieldID, operator, actualValue, true, rightOperand)
	return []PolicyFinding{finding}
}

// expandedRightOperand converts the constraint's right operand to plain Go
// values. Set operators always receive a list — expansion erases the
// distinction between a one-element set and a scalar.
func expandedRightOperand(constraint map[string]any, operator string) any {
	values := expandedValues(constraint, odrlIRI+"rightOperand")
	switch normalizePolicyOperator(operator) {
	case "in", "notIn":
		return expandedLiterals(values)
	}
	switch len(values) {
	case 0:
		return nil
	case 1:
		return expandedLiteral(values[0])
	default:
		return expandedLiterals(values)
	}
}

// expandExternalODRLRules expands rule nodes supplied by an audit policy
// document (compact JSON with odrl-prefixed keys) with the hub context so
// they evaluate identically to document-embedded rules.
func expandExternalODRLRules(ctx context.Context, rules []map[string]any, source ShapeSource) ([]map[string]any, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	wrapper := map[string]any{"dcs:policies": map[string]any{"odrl:obligation": anySlice(rules)}}
	root, err := expandForAudit(ctx, wrapper, source)
	if err != nil {
		return nil, fmt.Errorf("expand external ODRL policies: %w", err)
	}
	return expandedODRLPolicyRules(root), nil
}

func anySlice[T any](items []T) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// expandedStatements flattens the expanded graph into statement rows for
// the validation-profile engine: every typed node becomes
// {"@id", "@type" (first class IRI), <property local name>: value}.
func expandedStatements(root map[string]any) []map[string]any {
	statements := []map[string]any{}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if types, ok := node["@type"].([]any); ok && len(types) > 0 {
			statement := map[string]any{}
			if id, ok := node["@id"].(string); ok {
				statement["@id"] = id
			}
			if iri, ok := types[0].(string); ok {
				statement["@type"] = iri
			}
			for property, raw := range node {
				if strings.HasPrefix(property, "@") {
					continue
				}
				values, ok := raw.([]any)
				if !ok || len(values) == 0 {
					continue
				}
				if len(values) == 1 {
					statement[shaclLocalName(property)] = expandedLiteral(values[0])
				} else {
					statement[shaclLocalName(property)] = expandedLiterals(values)
				}
			}
			statements = append(statements, statement)
		}
		for property, raw := range node {
			if strings.HasPrefix(property, "@") && property != "@graph" {
				continue
			}
			values, ok := raw.([]any)
			if !ok {
				continue
			}
			for _, value := range values {
				child, ok := value.(map[string]any)
				if !ok {
					continue
				}
				if list, ok := child["@list"].([]any); ok {
					for _, item := range list {
						if node, ok := item.(map[string]any); ok {
							walk(node)
						}
					}
					continue
				}
				walk(child)
			}
		}
	}
	walk(root)
	return statements
}

// statementsCoverProfile reports whether any statement carries one of the
// profile's appliesTo class IRIs; a profile without appliesTo covers
// everything.
func statementsCoverProfile(statements []map[string]any, appliesTo []string) bool {
	if len(appliesTo) == 0 {
		return true
	}
	for _, statement := range statements {
		typeIRI, _ := statement["@type"].(string)
		for _, class := range appliesTo {
			if typeIRI == class {
				return true
			}
		}
	}
	return false
}
