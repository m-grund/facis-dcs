package builder

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalClauseTextRendersPlaceholderWithoutPolicyData(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"@list": []any{
			"Provider country: ",
			map[string]any{
				"@type":       "dcs:Placeholder",
				"dcs:token":   "{{provider.country}}",
				"dcs:bindsTo": map[string]any{"@id": "did:example:template#field-provider-country"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Provider country: __________", canonicalClauseText(raw, "clause", nil))
}

func TestCanonicalClauseTextResolvesSubmittedSemanticValue(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"@list": []any{
			"Customer: ",
			map[string]any{
				"@type":       "dcs:Placeholder",
				"dcs:token":   "{{customer.company.legalName}}",
				"dcs:bindsTo": map[string]any{"@id": "did:example:contract#field-customer-legalName"},
			},
		},
	})
	require.NoError(t, err)

	values := map[string]map[string]json.RawMessage{
		"did:example:contract#field-customer-legalName": {
			"clause": json.RawMessage(`"A"`),
		},
	}
	require.Equal(t, "Customer: A", canonicalClauseText(raw, "clause", values))
}

func TestCanonicalPlaceholderValuesMatchesSemanticPathParameterName(t *testing.T) {
	envelope := canonicalEnvelopeJSON{
		ContractData: []canonicalRequirementJSON{
			{
				Type:        "dcs:DataRequirement",
				ConditionID: "customer",
				Fields: []canonicalFieldJSON{
					{
						ID:            "did:example:contract#field-customer-legalName",
						ParameterName: "legalName",
						SemanticPath:  "company.legalName",
					},
				},
			},
		},
		SemanticConditionValues: []conditionValueJSON{
			{
				BlockID:        "clause",
				ConditionID:    "customer",
				ParameterName:  "company.legalName",
				ParameterValue: json.RawMessage(`"A"`),
			},
		},
	}

	values := canonicalPlaceholderValues(envelope)
	require.JSONEq(t, `"A"`, string(values["did:example:contract#field-customer-legalName"]["clause"]))
}

func TestRenderCanonicalEnvelopeRecognizesDocumentStructure(t *testing.T) {
	raw := []byte(`{
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {"@type": "dcs:TemplateMetadata"},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": [{
				"@id": "did:example:template#block-clause",
				"@type": "dcs:Clause",
				"dcs:content": {"@list": ["Payment amount: "]}
			}],
			"dcs:layout": [{
				"@id": "did:example:template#block-root",
				"dcs:isRoot": true,
				"dcs:children": {"@list": [{"@id": "did:example:template#block-clause"}]}
			}]
		},
		"dcs:contractData": [],
		"dcs:policies": []
	}`)

	var envelope canonicalEnvelopeJSON
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Len(t, envelope.DocumentStructure.Blocks, 1)
	require.Len(t, envelope.DocumentStructure.Layout, 1)
}
