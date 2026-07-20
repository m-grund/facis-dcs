package pdfcore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenComposedStructureLeavesSimpleDocumentsUnchanged(t *testing.T) {
	simple := []byte(`{"@type":"dcs:Contract","dcs:documentStructure":{"dcs:blocks":{"@list":[{"@type":"dcs:Clause","@id":"c1"}]},"dcs:layout":[{"@id":"r","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"c1"}]}}]}}`)
	out, err := flattenComposedStructure(simple)
	require.NoError(t, err)
	require.JSONEq(t, string(simple), string(out))
}

func TestFlattenComposedStructureInlinesSubTemplate(t *testing.T) {
	doc := map[string]any{
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"dcs:subTemplates": []any{
				map[string]any{
					"@id":         "sub-did",
					"dcs:version": float64(1),
					"dcs:template": map[string]any{
						"dcs:documentStructure": map[string]any{
							"dcs:blocks": map[string]any{"@list": []any{
								map[string]any{"@type": "dcs:Clause", "@id": "sub-clause", "dcs:title": "Payment terms"},
							}},
							"dcs:layout": []any{
								map[string]any{"@id": "sub-root", "dcs:isRoot": true, "dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "sub-clause"}}}},
							},
						},
					},
				},
			},
		},
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{"@type": "dcs:ApprovedTemplate", "@id": "at1", "dcs:templateDid": "sub-did", "dcs:version": float64(1)},
			}},
			"dcs:layout": []any{
				map[string]any{"@id": "root", "dcs:isRoot": true, "dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "at1"}}}},
				map[string]any{"@id": "at1", "dcs:children": map[string]any{"@list": []any{}}},
			},
		},
	}
	payload, _ := json.Marshal(doc)

	out, err := flattenComposedStructure(payload)
	require.NoError(t, err)

	var flat map[string]any
	require.NoError(t, json.Unmarshal(out, &flat))
	structure := flat["dcs:documentStructure"].(map[string]any)
	blocks, _ := listValue(structure["dcs:blocks"])

	types := map[string]bool{}
	for _, raw := range blocks {
		b := raw.(map[string]any)
		types[b["@type"].(string)] = true
	}
	// No ApprovedTemplate survives; the sub-template's clause is inlined and
	// the composition block became a Section container.
	require.False(t, types["dcs:ApprovedTemplate"], "ApprovedTemplate must be flattened away")
	require.True(t, types["dcs:Section"], "the composition block becomes a Section")
	require.True(t, types["dcs:Clause"], "the sub-template clause is inlined")

	// The inlined clause id is namespaced under the host block.
	var inlinedID string
	for _, raw := range blocks {
		b := raw.(map[string]any)
		if b["@type"] == "dcs:Clause" {
			inlinedID = b["@id"].(string)
		}
	}
	require.Equal(t, "at1::sub-clause", inlinedID)
}

func TestFlattenComposedStructureFailsOnMissingSnapshot(t *testing.T) {
	doc := []byte(`{"dcs:metadata":{"dcs:subTemplates":{"@list":[]}},"dcs:documentStructure":{"dcs:blocks":{"@list":[{"@type":"dcs:ApprovedTemplate","@id":"at1","dcs:templateDid":"absent","dcs:version":1}]},"dcs:layout":[{"@id":"root","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"at1"}]}}]}}`)
	_, err := flattenComposedStructure(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not present in dcs:subTemplates")
}
