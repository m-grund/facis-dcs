package validation

import (
	"context"
	"strings"
)

// EvaluateKPIViolation reports whether a target-reported KPI value violates
// an obligation the contract's own ODRL policies declare for it
// (DCS-FR-CWE-09/-31). The metric is the @id of the contract-data placeholder
// node the KPI reports on — the same IRI the ODRL odrl:leftOperand names — so
// the binding is by node IRI, exactly like the content-audit enforcement path
// (no fragile label-string hop). Every constraint whose odrl:leftOperand
// references the bound node is evaluated with the reported value as the actual
// value, under the same rule semantics as the content audit (a Prohibition is
// violated when satisfied).
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

	fieldIndex := expandedODRLFieldIndex(root)
	if _, bound := fieldIndex[metric]; !bound {
		return false, nil
	}
	boundFields := map[string]bool{metric: true}

	for _, rule := range expandedODRLPolicyRules(root) {
		isProhibition := expandedTypeLocalName(rule) == "Prohibition"
		for _, rawConstraint := range expandedValues(rule, odrlIRI+"constraint") {
			constraint, ok := rawConstraint.(map[string]any)
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
			satisfied, err := evaluateODRLConstraintOPA(ctx, operator, strings.TrimSpace(value), resolveRightOperand(constraint, operator, fieldIndex))
			if err != nil {
				return false, err
			}
			if (isProhibition && satisfied) || (!isProhibition && !satisfied) {
				return true, nil
			}
		}
	}
	return false, nil
}
