package contracttemplate

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

func TestConvertTemplateDataToContractDataKeepsJSONLDSemantics(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-1"}},
		},
		"documentBlocks": []any{
			map[string]any{"blockId": "clause-1", "type": "CLAUSE", "text": "Availability {{cond-1.percent}}", "conditionIds": []any{"cond-1"}},
		},
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "cond-1",
				"conditionName": "Availability",
				"schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "percent",
						"type":          "decimal",
						"schemaRef":     validation.SchemaServiceV1,
						"semanticPath":  "service.sla.availability",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
			map[string]any{
				"conditionId":   "customer",
				"conditionName": "Customer",
				"schemaVersion": "v1",
				"entityType":    "CompanyParty",
				"entityRole":    "customer",
				"parameters": []any{
					map[string]any{
						"parameterName": "legalName",
						"type":          "string",
						"schemaRef":     validation.SchemaPartyV1,
						"semanticPath":  "company.legalName",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)

	converted, err := convertTemplateDataToContractData(&raw, "did:web:facis.example:template:1")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*converted, &data))
	require.Equal(t, validation.SchemaJSONLDContextV1, data["@context"])
	require.Equal(t, "Contract", data["@type"])
	require.Equal(t, "did:web:facis.example:template:1", data["derivedFromTemplate"])
	require.Equal(t, "did:web:facis.example:template:1", data["sourceTemplate"].(map[string]any)["did"])
	conditions := data["semanticConditions"].([]any)
	customer := conditions[1].(map[string]any)
	require.Equal(t, validation.SchemaPartyV1, customer["parameters"].([]any)[0].(map[string]any)["schemaRef"])
	require.Equal(t, "https://w3id.org/facis/dcs/taxonomy/v1#role-customer", customer["entityRole"])
}
