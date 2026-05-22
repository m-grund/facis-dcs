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
	require.Equal(t, SchemaJSONLDContextV1, data["schemaRefs"].(map[string]any)["jsonLdContext"])
	require.NotEmpty(t, data["policyRefs"])
	require.Equal(t, "FACIS_DCS_TEMPLATE_V1", data["validation"].(map[string]any)["profile"])
	require.Equal(t, SemanticProfileVersionV1, data["semanticProfile"].(map[string]any)["version"])
	require.IsType(t, []any{}, data["placeholderBindings"])
	require.IsType(t, []any{}, data["semanticRules"])
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

func TestNormalizeTemplateDataAddsCanonicalValueConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	normalizedCondition := result["semanticConditions"].([]any)[0].(map[string]any)
	normalizedParam := normalizedCondition["parameters"].([]any)[0].(map[string]any)
	constraint := normalizedParam["valueConstraint"].(map[string]any)
	require.Equal(t, "iso-3166-1-alpha-3", constraint["format"])
	require.Contains(t, constraint["allowedValues"], "DEU")
}

func TestNormalizeContractDataRejectsSemanticValueOutsideConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "country",
			"parameterValue": "Germany",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, "violates constraint")
}

func TestNormalizeContractDataAcceptsSemanticValueInsideConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "country",
			"parameterValue": "DEU",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.NoError(t, err)
}

func TestNormalizeTemplateDataGeneratesSemanticRuleAndPlaceholderBinding(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	param := params[0].(map[string]any)
	param["operators"] = []any{
		map[string]any{
			"operate": "greaterThanOrEqual",
			"targets": []any{"99.95"},
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	bindings := result["placeholderBindings"].([]any)
	require.Len(t, bindings, 1)
	require.Equal(t, "{{cond-1.percent}}", bindings[0].(map[string]any)["placeholder"])
	rules := result["semanticRules"].([]any)
	require.Len(t, rules, 1)
	require.Equal(t, "GreaterThanOrEqual", rules[0].(map[string]any)["operator"])
	require.Equal(t, "semanticCondition", rules[0].(map[string]any)["source"])
}

func TestNormalizeTemplateDataRejectsUnsupportedSemanticOperator(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	param := params[0].(map[string]any)
	param["operators"] = []any{
		map[string]any{
			"operate": "unsupported",
			"targets": []any{"99.95"},
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "unsupported operator")
}
