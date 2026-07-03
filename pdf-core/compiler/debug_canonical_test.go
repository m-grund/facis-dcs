package compiler

import (
	"encoding/json"
	"testing"
)

func TestDebugFrontendPayload(t *testing.T) {
	// Exact structure the frontend sends: dcs: prefix, @id on every node, no @vocab
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "did:web:localhost:template:5e88dd71",
		"@type": "dcs:ContractTemplate",
		"dcs:contractData": [],
		"dcs:metadata": {
			"@id": "did:web:localhost:template:5e88dd71#metadata",
			"@type": "dcs:TemplateMetadata",
			"dcs:description": "asdasdas",
			"dcs:templateType": "dcs:SubContract",
			"dcs:title": "asdasdas"
		},
		"dcs:documentStructure": {
			"@id": "did:web:localhost:template:5e88dd71#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": {
				"@list": [
					{
						"@id": "did:web:localhost:template:5e88dd71#block-c1",
						"@type": "dcs:Clause",
						"dcs:content": {"@list": ["clause text"]},
						"dcs:title": "c"
					},
					{
						"@id": "did:web:localhost:template:5e88dd71#block-s1",
						"@type": "dcs:Section"
					}
				]
			},
			"dcs:layout": [
				{
					"@id": "did:web:localhost:template:5e88dd71#root",
					"@type": "dcs:LayoutNode",
					"dcs:children": {"@list": [
						{"@id": "did:web:localhost:template:5e88dd71#block-s1"}
					]},
					"dcs:isRoot": true
				},
				{
					"@id": "did:web:localhost:template:5e88dd71#block-s1",
					"@type": "dcs:LayoutNode",
					"dcs:children": {"@list": [
						{"@id": "did:web:localhost:template:5e88dd71#block-c1"}
					]}
				}
			]
		},
		"dcs:policies": [],
		"http://localhost:8080/ontology/dcs-pdf-core#title": "asdasdas"
	}`)

	canonical, err := CanonicalizePayload(payload)
	if err != nil {
		t.Fatalf("CanonicalizePayload error: %v", err)
	}
	
	// Pretty-print for inspection
	var pretty map[string]any
	if err := json.Unmarshal(canonical, &pretty); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	b, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("canonical JSON:\n%s", b)

	// Now check what the model extraction produces
	var tmpl ContractTemplate
	if err := json.Unmarshal(canonical, &tmpl); err != nil {
		t.Logf("json.Unmarshal error: %v", err)
	}
	t.Logf("tmpl.Metadata = %+v", tmpl.Metadata)
	if tmpl.Metadata != nil {
		t.Logf("tmpl.Metadata.Title = %q", tmpl.Metadata.Title)
	}
}
