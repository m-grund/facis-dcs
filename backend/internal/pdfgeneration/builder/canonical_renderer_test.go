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
	require.Equal(t, "Provider country: __________", canonicalClauseText(raw))
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
