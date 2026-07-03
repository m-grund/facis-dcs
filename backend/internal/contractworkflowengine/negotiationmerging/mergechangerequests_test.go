package negotiationmerging

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeContractDataChangeAcceptsCanonicalContractData(t *testing.T) {
	stored := map[string]any{
		"semanticConditionValues": []any{
			map[string]any{
				"blockId":        "block-1",
				"conditionId":    "provider",
				"parameterName":  "country",
				"parameterValue": "DEU",
			},
		},
	}
	canonicalChange := json.RawMessage(`{
		"@type": "dcs:Contract",
		"dcs:documentStructure": {
			"dcs:blocks": { "@list": [] },
			"dcs:layout": []
		},
		"dcs:contractData": [],
		"dcs:policies": [],
		"semanticConditionValues": []
	}`)

	merged, err := mergeContractDataChange(stored, canonicalChange)

	require.NoError(t, err)
	require.Equal(t, "dcs:Contract", merged["@type"])
	require.Contains(t, merged, "dcs:documentStructure")
	require.Empty(t, merged["semanticConditionValues"])
}

func TestMergeContractDataChangeAppliesLegacySemanticValuePatch(t *testing.T) {
	stored := map[string]any{
		"semanticConditionValues": []any{
			map[string]any{
				"blockId":        "block-1",
				"conditionId":    "provider",
				"parameterName":  "country",
				"parameterValue": "DEU",
			},
		},
	}
	partialChange := json.RawMessage(`{
		"semanticConditionValues": [
			{
				"blockId": "block-1",
				"conditionId": "provider",
				"parameterName": "country",
				"parameterValue": "AUT"
			}
		]
	}`)

	merged, err := mergeContractDataChange(stored, partialChange)

	require.NoError(t, err)
	values, ok := merged["semanticConditionValues"].([]SemanticConditionValue)
	require.True(t, ok)
	require.Len(t, values, 1)
	require.JSONEq(t, `"AUT"`, string(values[0].ParameterValue))
}
