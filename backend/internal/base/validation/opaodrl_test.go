package validation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOPAConstraintParityWithHandRolled is the ADR-11 parity gate: the OPA
// ODRL evaluation must return the identical verdict to the hand-rolled
// evaluateODRLConstraint for every operator/value combination, including the
// case-insensitivity, numeric-string coercion, and tolerance edge cases the
// contract audit corpus exercises. Until this passes, the hand-rolled
// evaluator stays as the oracle; once it does, the switch can be retired.
func TestOPAConstraintParityWithHandRolled(t *testing.T) {
	cases := []struct {
		operator string
		actual   any
		right    any
	}{
		// eq: string exact, case-insensitive, numeric-string coercion
		{"eq", "DEU", "DEU"},
		{"eq", "DEU", "deu"},
		{"eq", "DEU", "USA"},
		{"eq", "91448", "91448"},
		{"eq", float64(9500), float64(9500)},
		{"eq", "9500", float64(9500)},
		{"eq", float64(9500), float64(10000)},
		{"eq", float64(99.5), float64(99.5000000001)}, // within tolerance
		{"eq", "DEU", float64(5)},                     // non-numeric string vs number
		// neq
		{"neq", "DEU", "USA"},
		{"neq", "DEU", "deu"},
		{"neq", float64(9500), float64(9500)},
		// gt / gteq / lt / lteq (numeric, boundary + tolerance)
		{"gt", float64(150000), float64(100000)},
		{"gt", float64(100000), float64(150000)},
		{"gt", float64(99.5), float64(99.5)},
		{"gteq", float64(99.5), float64(99.5)},
		{"gteq", float64(99.4), float64(99.5)},
		{"gteq", float64(100), float64(99.5)},
		{"lt", float64(99.4), float64(99.5)},
		{"lt", float64(99.5), float64(99.5)},
		{"lteq", float64(99.5), float64(99.5)},
		{"lteq", float64(99.6), float64(99.5)},
		{"lteq", "9500", float64(10000)}, // numeric-string coercion
		{"gt", "notanumber", float64(1)}, // non-numeric -> false both sides
		// isAnyOf / isNoneOf (upper/trim-normalised membership)
		{"isAnyOf", "DEU", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "deu", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "ZZZ", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "DE", []any{"DE", "AT", "CH"}},
		{"isNoneOf", "RUS", []any{"RUS"}},
		{"isNoneOf", "DEU", []any{"RUS"}},
		{"isNoneOf", "DEU", []any{}},
		// hasPart (substring)
		{"hasPart", "hello world", "world"},
		{"hasPart", "hello", "xyz"},
		// unknown operator -> false
		{"unknownOp", "DEU", "DEU"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%v/%v", tc.operator, tc.actual, tc.right), func(t *testing.T) {
			want := evaluateODRLConstraint(tc.operator, tc.actual, tc.right)
			got, err := evaluateODRLConstraintOPA(context.Background(), tc.operator, tc.actual, tc.right)
			require.NoError(t, err)
			require.Equal(t, want, got, "OPA verdict must match hand-rolled evaluator")
		})
	}
}
