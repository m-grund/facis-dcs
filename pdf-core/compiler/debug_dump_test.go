package compiler

import (
	"encoding/json"
	"testing"
)

func TestDebugDumpCanonicalUserPayload(t *testing.T) {
	raw := []byte(`{
  "@context": {
    "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
    "odrl": "http://www.w3.org/ns/odrl/2/",
    "xsd": "http://www.w3.org/2001/XMLSchema#"
  },
  "@id": "did:web:localhost:template:demo",
  "@type": "dcs:ContractTemplate",
  "dcs:documentStructure": {
    "@id": "did:web:localhost:template:demo#document-structure",
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {
      "@list": [
        {"@id": "did:web:localhost:template:demo#s1", "@type": "dcs:Section", "dcs:title": "ABCDEFG"},
        {"@id": "did:web:localhost:template:demo#t1", "@type": "dcs:TextBlock", "dcs:text": "asdsadsaddasd"},
        {"@id": "did:web:localhost:template:demo#c1", "@type": "dcs:Clause", "dcs:content": {"@list": ["Hello world"]}, "dcs:title": "stuff"}
      ]
    },
    "dcs:layout": [
      {"@id": "did:web:localhost:template:demo#root", "@type": "dcs:LayoutNode", "dcs:isRoot": true,
        "dcs:children": {"@list": [
          {"@id": "did:web:localhost:template:demo#s1"},
          {"@id": "did:web:localhost:template:demo#c1"}
        ]}},
      {"@id": "did:web:localhost:template:demo#s1", "@type": "dcs:LayoutNode",
        "dcs:children": {"@list": [
          {"@id": "did:web:localhost:template:demo#t1"}
        ]}}
    ]
  },
  "dcs:metadata": {"@id": "did:web:localhost:template:demo#metadata", "@type": "dcs:TemplateMetadata", "dcs:title": "fdsfdsf"}
}`)

	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	var pretty any
	_ = json.Unmarshal(canonical, &pretty)
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("CANONICAL:\n%s", out)

	var tmpl ContractTemplate
	if err := json.Unmarshal(canonical, &tmpl); err != nil {
		t.Fatalf("unmarshal into ContractTemplate: %v", err)
	}
	if tmpl.DocumentStructure == nil {
		t.Fatalf("DocumentStructure nil")
	}
	for i, ln := range tmpl.DocumentStructure.Layout {
		t.Logf("layout[%d] id=%q isRoot=%v children=%v", i, ln.ID, ln.IsRoot, ln.Children)
	}
	for i, b := range tmpl.DocumentStructure.Blocks {
		t.Logf("block[%d] id=%q type=%q title=%q text=%q", i, b.ID, b.Type, b.Title, b.Text)
	}
}
