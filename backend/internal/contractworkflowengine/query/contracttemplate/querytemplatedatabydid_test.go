package contracttemplate

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

func TestConvertTemplateDataToContractDataKeepsCanonicalContent(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@id":   "did:web:facis.example:template:1",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:templateType": "dcs:SubContract",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "did:web:facis.example:template:1#block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Availability ",
						map[string]any{
							"@type":       "dcs:Placeholder",
							"dcs:token":   "{{cond-1.percent}}",
							"dcs:bindsTo": map[string]any{"@id": "did:web:facis.example:template:1#field-cond-1-percent"},
						},
					}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":          "did:web:facis.example:template:1#block-root",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "did:web:facis.example:template:1#block-clause-1"}}},
				},
			},
		},
		"dcs:contractData": []any{
			map[string]any{
				"@id":               "did:web:facis.example:template:1#requirement-cond-1",
				"@type":             "dcs:DataRequirement",
				"dcs:conditionId":   "cond-1",
				"dcs:name":          "Availability",
				"dcs:schemaVersion": "v1",
				"dcs:fields": []any{
					map[string]any{
						"@id":               "did:web:facis.example:template:1#field-cond-1-percent",
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": "percent",
						"dcs:domainField": map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-service-sla-availability"},
						"dcs:required":    true,
					},
				},
			},
		},
		"dcs:policies": []any{
			map[string]any{
				"@id":   "did:web:facis.example:template:1#policy-cond-1-percent-0",
				"@type": "odrl:Duty",
				"odrl:constraint": map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  map[string]any{"@id": "did:web:facis.example:template:1#field-cond-1-percent"},
					"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
					"odrl:rightOperand": map[string]any{"@value": "99.95", "@type": "xsd:decimal"},
				},
			},
		},
	})
	require.NoError(t, err)

	converted, err := convertTemplateDataToContractData(&raw, "did:web:facis.example:template:1", 7)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*converted, &data))
	require.Equal(t, "dcs:Contract", data["@type"])
	require.Equal(t, "did:web:facis.example:template:1", data["derivedFromTemplate"])
	require.Equal(t, "did:web:facis.example:template:1", data["sourceTemplate"].(map[string]any)["did"])
	require.Equal(t, float64(7), data["sourceTemplate"].(map[string]any)["version"])
	require.Empty(t, data["semanticConditionValues"])
	structure := data["dcs:documentStructure"].(map[string]any)
	require.Len(t, structure["dcs:blocks"].(map[string]any)["@list"], 1)
	require.Len(t, data["dcs:contractData"], 1)

	persisted, err := validation.NormalizeContractDataForPersistence(
		converted,
		"did:web:facis.example:contract:1",
		nil,
		false,
	)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(*persisted, &data))
	require.Equal(t, "did:web:facis.example:contract:1", data["@id"])
	structure = data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:contract:1#block-clause-1", block["@id"])
	placeholder := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	require.Equal(
		t,
		"did:web:facis.example:contract:1#field-cond-1-percent",
		placeholder["dcs:bindsTo"].(map[string]any)["@id"],
	)
	policy := data["dcs:policies"].([]any)[0].(map[string]any)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(
		t,
		"did:web:facis.example:contract:1#field-cond-1-percent",
		constraint["odrl:leftOperand"].(map[string]any)["@id"],
	)
}

func TestConvertTemplateDataToContractDataRejectsLegacyTemplate(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{"documentBlocks": []any{}})
	require.NoError(t, err)

	_, err = convertTemplateDataToContractData(&raw, "did:web:facis.example:template:1")
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}
