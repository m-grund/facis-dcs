package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// evaluateODRLConstraint is the retired hand-rolled operator switch, kept
// here as the parity oracle only — it is not compiled into the production
// binary (ADR-11). evaluateODRLConstraintOPA must match it verdict-for-verdict.
const floatTolerance = 0.0000001

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
