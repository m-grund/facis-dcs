package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// contractWithBoundary builds a minimal contract whose permission is bounded by
// a spatial constraint whose boundary is the negotiated "region" field. The
// field is filled only when value != "".
func contractWithBoundary(value string) map[string]any {
	field := map[string]any{
		"@id":          "urn:dcs:field:region",
		"@type":        "dcs:Placeholder",
		"dcs:label":    "region",
		"dcs:datatype": "xsd:string",
		"dcs:required": true,
	}
	if value != "" {
		field["dcs:value"] = value
	}
	return map[string]any{
		"@type":            "dcs:Contract",
		"dcs:contractData": []any{field},
		"dcs:policies": map[string]any{
			"@type": "odrl:Agreement",
			"odrl:permission": []any{
				map[string]any{
					"@id":         "R1",
					"@type":       "odrl:Permission",
					"odrl:action": map[string]any{"@id": "odrl:use"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": "odrl:spatial"},
							"odrl:operator":     map[string]any{"@id": "odrl:eq"},
							"odrl:rightOperand": map[string]any{"@id": "urn:dcs:field:region"},
						},
					},
				},
			},
		},
	}
}

func TestValidateContractClosedFlagsUnfilledNegotiatedBoundary(t *testing.T) {
	err := ValidateContractClosed(contractWithBoundary(""))
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "negotiated boundary")
}

func TestValidateContractClosedAcceptsFilledBoundary(t *testing.T) {
	require.NoError(t, ValidateContractClosed(contractWithBoundary("DE")))
}

func TestValidateContractClosedFlagsUnfilledRequiredField(t *testing.T) {
	// The permission constrains the field directly (not as a boundary); a
	// required field a policy enforces must carry a value.
	doc := map[string]any{
		"@type": "dcs:Contract",
		"dcs:contractData": []any{
			map[string]any{
				"@id": "urn:dcs:field:amount", "@type": "dcs:Placeholder",
				"dcs:label": "amount", "dcs:datatype": "xsd:decimal", "dcs:required": true,
			},
		},
		"dcs:policies": map[string]any{
			"@type": "odrl:Agreement",
			"odrl:obligation": []any{
				map[string]any{
					"@id": "D1", "@type": "odrl:Duty",
					"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": "urn:dcs:field:amount"},
							"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
							"odrl:rightOperand": map[string]any{"@value": "100", "@type": "xsd:decimal"},
						},
					},
				},
			},
		},
	}
	err := ValidateContractClosed(doc)
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "required data field")
}

func TestValidateContractClosedFlagsUnfilledProsePlaceholder(t *testing.T) {
	doc := contractWithBoundary("DE") // boundary filled, but a prose placeholder is not
	doc["dcs:documentStructure"] = map[string]any{
		"@type": "dcs:DocumentStructure",
		"dcs:blocks": map[string]any{"@list": []any{
			map[string]any{
				"@id": "urn:dcs:block:1", "@type": "dcs:Clause",
				"dcs:content": map[string]any{"@list": []any{
					"The party in ",
					map[string]any{"@id": "urn:dcs:field:unfilled"},
				}},
			},
		}},
	}
	err := ValidateContractClosed(doc)
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "prose placeholder")
}
