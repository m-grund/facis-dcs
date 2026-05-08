package validation

import (
	"digital-contracting-service/internal/base/datatype"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func validTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()
	data, err := datatype.NewJSON(map[string]any{
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
						"schemaRef":     SchemaServiceV1,
						"semanticPath":  "service.sla.availability",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)
	return &data
}

func TestNormalizeTemplateDataAddsSchemaAndPolicyRefs(t *testing.T) {
	normalized, err := NormalizeTemplateData(validTemplateData(t))
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, SchemaTemplateDataV1, data["schemaRefs"].(map[string]any)["templateData"])
	require.NotEmpty(t, data["policyRefs"])
	require.Equal(t, "FACIS_DCS_TEMPLATE_V1", data["validation"].(map[string]any)["profile"])
}

func TestNormalizeTemplateDataRejectsUnknownConditionReference(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticConditions"] = []any{}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "unknown semantic condition")
}

func TestNormalizeContractDataRequiresSemanticValuesWhenStrict(t *testing.T) {
	templateData := validTemplateData(t)
	contractData, err := NormalizeContractData(templateData, false)
	require.NoError(t, err)

	_, err = NormalizeContractData(contractData, true)
	require.ErrorContains(t, err, "required semantic value missing")
}

func TestNormalizeContractDataAcceptsTypedSemanticValues(t *testing.T) {
	templateData := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*templateData, &decoded))
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "percent",
			"parameterValue": 99.9,
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.NoError(t, err)
}
