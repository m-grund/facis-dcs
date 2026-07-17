package validation

import (
	"context"
	"sync"

	"github.com/open-policy-agent/opa/rego"
)

// ODRL constraint satisfaction on Open Policy Agent (ADR-11). The operator
// semantics that were a hand-rolled Go switch (evaluateODRLConstraint) are
// expressed once as Rego and evaluated by the embedded engine: string
// equality is case-insensitive, numeric strings coerce to numbers, numeric
// comparisons carry the same 1e-7 tolerance, and set membership is
// upper/trim-normalised — matching evaluateODRLConstraint verdict-for-verdict
// (opaodrl_test.go is the parity gate).
const odrlRegoModule = `
package dcs.odrl

import rego.v1

tol := 0.0000001

to_num(x) := x if is_number(x)
to_num(x) := to_number(trim_space(x)) if is_string(x)

both_num(a, b) if {
	to_num(a)
	to_num(b)
}

both_string(a, b) if {
	is_string(a)
	is_string(b)
}

norm(x) := upper(trim_space(sprintf("%v", [x])))

values_equal(a, b) if {
	both_string(a, b)
	lower(trim_space(a)) == lower(trim_space(b))
}

values_equal(a, b) if {
	not both_string(a, b)
	both_num(a, b)
	abs(to_num(a) - to_num(b)) <= tol
}

values_equal(a, b) if {
	not both_string(a, b)
	not both_num(a, b)
	sprintf("%v", [a]) == sprintf("%v", [b])
}

any_match if {
	some item in input.right
	norm(item) == norm(input.actual)
}

default satisfied := false

satisfied if {
	input.operator == "eq"
	values_equal(input.actual, input.right)
}

satisfied if {
	input.operator == "neq"
	not values_equal(input.actual, input.right)
}

satisfied if {
	input.operator == "gt"
	both_num(input.actual, input.right)
	to_num(input.actual) > to_num(input.right) + tol
}

satisfied if {
	input.operator == "gteq"
	both_num(input.actual, input.right)
	to_num(input.actual) + tol >= to_num(input.right)
}

satisfied if {
	input.operator == "lt"
	both_num(input.actual, input.right)
	to_num(input.actual) < to_num(input.right) - tol
}

satisfied if {
	input.operator == "lteq"
	both_num(input.actual, input.right)
	to_num(input.actual) <= to_num(input.right) + tol
}

satisfied if {
	input.operator == "isAnyOf"
	any_match
}

satisfied if {
	input.operator == "isNoneOf"
	not any_match
}

satisfied if {
	input.operator == "hasPart"
	is_string(input.actual)
	contains(input.actual, sprintf("%v", [input.right]))
}
`

var (
	odrlRegoOnce  sync.Once
	odrlRegoQuery rego.PreparedEvalQuery
	odrlRegoErr   error
)

func preparedODRLQuery() (rego.PreparedEvalQuery, error) {
	odrlRegoOnce.Do(func() {
		odrlRegoQuery, odrlRegoErr = rego.New(
			rego.Query("data.dcs.odrl.satisfied"),
			rego.Module("odrl.rego", odrlRegoModule),
		).PrepareForEval(context.Background())
	})
	return odrlRegoQuery, odrlRegoErr
}

// evaluateODRLConstraintOPA reports whether an actual value satisfies an ODRL
// constraint operator against its right operand, evaluated on OPA. The
// operator is reduced to its local name and the values compacted exactly as
// evaluateODRLConstraint does, so the two agree on every verdict.
func evaluateODRLConstraintOPA(ctx context.Context, operator string, actualValue, rightOperand any) (bool, error) {
	query, err := preparedODRLQuery()
	if err != nil {
		return false, err
	}
	input := map[string]any{
		"operator": compactTerm(operator),
		"actual":   compactJSONLDValue(actualValue),
		"right":    compactJSONLDValue(rightOperand),
	}
	rs, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return false, nil
	}
	satisfied, _ := rs[0].Expressions[0].Value.(bool)
	return satisfied, nil
}
