package command

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EvaluateKPIViolation reports whether the reported value for metric
// violates a contractual SLA threshold declared in the contract's own ODRL
// policy (dcs:policies, DCS-FR-CWE-09). It supports both the legacy flat
// array of bare odrl:Duty/Permission/Prohibition nodes and the odrl:Set-
// enclosed shape (rules under the odrl:duty/odrl:permission/odrl:prohibition/
// odrl:obligation bucket properties).
//
// A policy rule's odrl:constraint governs metric when its
// odrl:leftOperand's @id contains metric as a case-insensitive substring
// (the convention the KPI-reporting fixtures follow, e.g. field
// "urn:uuid:field-provider-coverage" for metric "coverage") — there is no
// separate formal binding between a KPI metric name and an ODRL field IRI
// today.
func EvaluateKPIViolation(contractData []byte, metric string, value string) bool {
	if len(contractData) == 0 || strings.TrimSpace(metric) == "" {
		return false
	}
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return false
	}
	actual, ok := parseKPINumber(value)
	if !ok {
		return false
	}
	for _, rule := range kpiPolicyRules(doc) {
		constraint, ok := rule["odrl:constraint"].(map[string]any)
		if !ok {
			continue
		}
		leftOperand, _ := constraint["odrl:leftOperand"].(map[string]any)
		fieldID, _ := leftOperand["@id"].(string)
		if fieldID == "" || !strings.Contains(strings.ToLower(fieldID), strings.ToLower(metric)) {
			continue
		}
		operatorObj, _ := constraint["odrl:operator"].(map[string]any)
		operator, _ := operatorObj["@id"].(string)
		if operator == "" {
			continue
		}
		expected, ok := parseKPINumber(fmt.Sprint(constraint["odrl:rightOperand"]))
		if !ok {
			continue
		}
		if kpiConstraintViolated(operator, actual, expected) {
			return true
		}
	}
	return false
}

func kpiPolicyRules(contractData map[string]any) []map[string]any {
	raw, ok := contractData["dcs:policies"]
	if !ok {
		raw = contractData["policies"]
	}
	var rules []map[string]any
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if rule, ok := item.(map[string]any); ok {
				rules = append(rules, rule)
			}
		}
	case map[string]any:
		for _, key := range []string{"odrl:duty", "odrl:permission", "odrl:prohibition", "odrl:obligation"} {
			switch bucket := typed[key].(type) {
			case []any:
				for _, item := range bucket {
					if rule, ok := item.(map[string]any); ok {
						rules = append(rules, rule)
					}
				}
			case map[string]any:
				rules = append(rules, bucket)
			}
		}
	}
	return rules
}

func kpiConstraintViolated(operator string, actual, expected float64) bool {
	const tolerance = 1e-9
	switch compactODRLOperator(operator) {
	case "gteq":
		return actual+tolerance < expected
	case "lteq":
		return actual > expected+tolerance
	case "gt":
		return actual <= expected+tolerance
	case "lt":
		return actual >= expected-tolerance
	case "eq":
		return math.Abs(actual-expected) > tolerance
	case "neq":
		return math.Abs(actual-expected) <= tolerance
	default:
		return false
	}
}

func compactODRLOperator(operator string) string {
	if index := strings.LastIndex(operator, ":"); index >= 0 && index < len(operator)-1 {
		return operator[index+1:]
	}
	return operator
}

func parseKPINumber(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
