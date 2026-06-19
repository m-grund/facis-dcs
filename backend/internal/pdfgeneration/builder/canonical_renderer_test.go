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
	requirement, err := json.Marshal(canonicalRequirementJSON{
		Type:        "dcs:DataRequirement",
		ConditionID: "customer",
		Fields: []canonicalFieldJSON{
			{
				ID:            "did:example:contract#field-customer-legalName",
				ParameterName: "legalName",
				SemanticPath:  "company.legalName",
			},
		},
	})
	require.NoError(t, err)
	envelope := canonicalEnvelopeJSON{
		ContractData: []json.RawMessage{requirement},
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

func TestCanonicalPlaceholderValuesResolveFinalContractData(t *testing.T) {
	provider := json.RawMessage(`{
		"@id": "did:example:contract#provider",
		"dcs:legalName": {"@type": "xsd:string", "@value": "Provider AG"},
		"dcs:country": {"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-AUT"}
	}`)
	envelope := canonicalEnvelopeJSON{
		ContractData: []json.RawMessage{provider},
		ContractFields: []canonicalContractFieldJSON{
			{
				ID:           "did:example:contract#field-provider-legalName",
				SourceObject: canonicalReference{ID: "did:example:contract#provider"},
				Path:         "dcs:legalName",
			},
			{
				ID:           "did:example:contract#field-provider-country",
				SourceObject: canonicalReference{ID: "did:example:contract#provider"},
				Path:         "dcs:country",
			},
		},
	}

	values := canonicalPlaceholderValues(envelope)
	require.JSONEq(t, `"Provider AG"`, string(values["did:example:contract#field-provider-legalName"][""]))
	require.JSONEq(t, `"AUT"`, string(values["did:example:contract#field-provider-country"][""]))
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
