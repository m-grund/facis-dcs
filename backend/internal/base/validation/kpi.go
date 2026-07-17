package validation

import (
	"context"
	"strings"
)

// EvaluateKPIViolation reports whether a target-reported KPI value violates
// an obligation the contract's own ODRL policies declare for it
// (DCS-FR-CWE-09/-31). The metric binds to the RequirementFields whose
// dcs:parameterName equals it (case-insensitive); every constraint whose
// odrl:leftOperand references a bound field is evaluated with the reported
// value as the actual value, under the same rule semantics as the content
// audit (a Prohibition is violated when satisfied).
func EvaluateKPIViolation(ctx context.Context, contractDocument any, metric, value string) (bool, error) {
	if strings.TrimSpace(metric) == "" {
		return false, nil
	}
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return false, err
	}
	source, err := requireShapeSource()
	if err != nil {
		return false, err
	}
	root, err := expandForAudit(ctx, contract, source)
	if err != nil {
		return false, err
	}

	boundFields := map[string]bool{}
	for fieldID, info := range expandedODRLFieldIndex(root) {
		if strings.EqualFold(info.parameterName, metric) {
			boundFields[fieldID] = true
		}
	}
	if len(boundFields) == 0 {
		return false, nil
	}

	for _, rule := range expandedODRLPolicyRules(root) {
		constraint, ok := expandedFirst(rule, odrlIRI+"constraint")
		if !ok {
			continue
		}
		leftOperand, ok := expandedFirst(constraint, odrlIRI+"leftOperand")
		if !ok || !boundFields[expandedID(leftOperand)] {
			continue
		}
		operatorNode, ok := expandedFirst(constraint, odrlIRI+"operator")
		if !ok {
			continue
		}
		operator := shaclLocalName(expandedID(operatorNode))
		if operator == "" {
			continue
		}
		satisfied, err := evaluateODRLConstraintOPA(ctx, operator, strings.TrimSpace(value), expandedRightOperand(constraint, operator))
		if err != nil {
			return false, err
		}
		isProhibition := expandedTypeLocalName(rule) == "Prohibition"
		if (isProhibition && satisfied) || (!isProhibition && !satisfied) {
			return true, nil
		}
	}
	return false, nil
}
