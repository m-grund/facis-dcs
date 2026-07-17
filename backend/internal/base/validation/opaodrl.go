package validation

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
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

is_ts_string(x) if {
	is_string(x)
	regex.match("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$", x)
}

to_ts(x) := time.parse_rfc3339_ns(x) if is_ts_string(x)

both_ts(a, b) if {
	to_ts(a)
	to_ts(b)
}

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

# Ordering over RFC3339 timestamps (SRS Appendix C dateTime constraints):
# when the operands are not numbers but parse as instants, compare instants.
satisfied if {
	input.operator == "gt"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) > to_ts(input.right)
}

satisfied if {
	input.operator == "gteq"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) >= to_ts(input.right)
}

satisfied if {
	input.operator == "lt"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) < to_ts(input.right)
}

satisfied if {
	input.operator == "lteq"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) <= to_ts(input.right)
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

var (
	tzLessDateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?$`)
	dateOnly       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// normalizeTemporal renders a timezone-less ISO-8601 dateTime (or a bare date)
// as RFC3339 UTC so the Rego engine's time.parse_rfc3339_ns can order it;
// non-temporal values pass through untouched. The ODRL vocabulary the UI emits
// carries such tz-less dateTimes (SRS Appendix C: "2025-05-10T23:59:59").
func normalizeTemporal(value any) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	trimmed := strings.TrimSpace(s)
	switch {
	case tzLessDateTime.MatchString(trimmed):
		return trimmed + "Z"
	case dateOnly.MatchString(trimmed):
		return trimmed + "T00:00:00Z"
	default:
		return value
	}
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
		"actual":   normalizeTemporal(compactJSONLDValue(actualValue)),
		"right":    normalizeTemporal(compactJSONLDValue(rightOperand)),
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
