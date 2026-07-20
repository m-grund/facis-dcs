package validation

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/stretchr/testify/require"
)

// TestOPAEmbedsAndEvaluates is the ADR-11 gate step 1 smoke test: OPA compiles
// and evaluates a trivial Rego policy in-process, proving the embedded engine
// is viable before any ODRL→Rego wiring. A policy allows when the submitted
// value is one of an allowed set — the shape an odrl:isAnyOf constraint takes.
func TestOPAEmbedsAndEvaluates(t *testing.T) {
	module := `
package dcs.policy

import rego.v1

default allow := false

allow if input.value in {"DEU", "AUT", "CHE"}
`
	query, err := rego.New(
		rego.Query("data.dcs.policy.allow"),
		rego.Module("policy.rego", module),
	).PrepareForEval(context.Background())
	require.NoError(t, err)

	satisfied := func(value string) bool {
		rs, err := query.Eval(context.Background(), rego.EvalInput(map[string]any{"value": value}))
		require.NoError(t, err)
		require.Len(t, rs, 1)
		require.Len(t, rs[0].Expressions, 1)
		allow, ok := rs[0].Expressions[0].Value.(bool)
		require.True(t, ok)
		return allow
	}

	require.True(t, satisfied("DEU"))
	require.False(t, satisfied("USA"))
}
