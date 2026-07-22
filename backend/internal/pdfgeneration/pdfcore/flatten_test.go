package pdfcore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInlinePlaceholderRenderTextCopiesLabelAndValue(t *testing.T) {
	doc := []byte(`{
      "dcs:contractData": [
        {"@id": "urn:f:amount", "@type": "dcs:Placeholder", "dcs:label": "Payment Amount", "dcs:datatype": "xsd:decimal", "dcs:value": 15000}
      ],
      "dcs:documentStructure": {
        "dcs:blocks": {"@list": [
          {"@id": "c1", "@type": "dcs:Clause", "dcs:content": {"@list": ["Pay ", {"@id": "urn:f:amount"}]}}
        ]},
        "dcs:layout": []
      }
    }`)
	out, err := inlinePlaceholderRenderText(doc)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	blocks := parsed["dcs:documentStructure"].(map[string]any)["dcs:blocks"].(map[string]any)["@list"].([]any)
	content := blocks[0].(map[string]any)["dcs:content"].(map[string]any)["@list"].([]any)
	ref := content[1].(map[string]any)
	require.Equal(t, "Payment Amount", ref["dcs:label"])
	require.EqualValues(t, 15000, ref["dcs:value"])
}
