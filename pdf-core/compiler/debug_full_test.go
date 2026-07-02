package compiler

import (
	"strings"
	"testing"
)

func TestDebugFullUserPayload(t *testing.T) {
	raw := []byte(userPayloadJSON)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	doc, err := extractDocumentModelFromCanonical(canonical, strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	var dump func(s sectionData, depth int)
	dump = func(s sectionData, depth int) {
		t.Logf("%sSECTION heading=%q clauses=%d subs=%d", strings.Repeat("  ", depth), s.Heading, len(s.Clauses), len(s.Subsections))
		for ci, c := range s.Clauses {
			var b strings.Builder
			for _, seg := range c.Segments {
				b.WriteString(seg.Text)
				b.WriteString(seg.Value)
			}
			t.Logf("%s  clause[%d]=%q", strings.Repeat("  ", depth), ci, b.String())
		}
		for _, sub := range s.Subsections {
			dump(sub, depth+1)
		}
	}
	for _, s := range doc.Sections {
		dump(s, 0)
	}
}

const userPayloadJSON = `{
  "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/", "xsd": "http://www.w3.org/2001/XMLSchema#"},
  "@id": "did:web:localhost:template:fb",
  "@type": "dcs:ContractTemplate",
  "dcs:documentStructure": {
    "@id": "did:web:localhost:template:fb#ds",
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {"@list": [
      {"@id": "did:web:localhost:template:fb#c631", "@type": "dcs:Clause", "dcs:content": {"@list": ["Dispute Resolution Method: ", {"@type": "dcs:Placeholder"}]}, "dcs:title": "Legal Terms"},
      {"@id": "did:web:localhost:template:fb#0fe4", "@type": "dcs:Section", "dcs:title": "ABCDEFG"},
      {"@id": "did:web:localhost:template:fb#c006", "@type": "dcs:TextBlock", "dcs:text": "asdsadsaddasd"},
      {"@id": "did:web:localhost:template:fb#f969", "@type": "dcs:Section", "dcs:title": "dasdasdsa"},
      {"@id": "did:web:localhost:template:fb#0a85", "@type": "dcs:TextBlock", "dcs:text": "aaaaa"},
      {"@id": "did:web:localhost:template:fb#6ea7", "@type": "dcs:Clause", "dcs:content": {"@list": ["asdasd stuff"]}, "dcs:title": "stuff"},
      {"@id": "did:web:localhost:template:fb#e03f", "@type": "dcs:TextBlock", "dcs:text": "fdsfdsf"},
      {"@id": "did:web:localhost:template:fb#603f", "@type": "dcs:TextBlock", "dcs:text": "dasdsad"},
      {"@id": "did:web:localhost:template:fb#7e2a", "@type": "dcs:Clause", "dcs:content": {"@list": ["SLA Availability: "]}, "dcs:title": "Service Level Objective"},
      {"@id": "did:web:localhost:template:fb#1a91", "@type": "dcs:Section", "dcs:title": "sec 2"},
      {"@id": "did:web:localhost:template:fb#125a", "@type": "dcs:TextBlock", "dcs:text": "blablo"},
      {"@id": "did:web:localhost:template:fb#9325", "@type": "dcs:TextBlock", "dcs:text": "pups"}
    ]},
    "dcs:layout": [
      {"@id": "did:web:localhost:template:fb#rt", "@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": {"@list": [
        {"@id": "did:web:localhost:template:fb#0fe4"},
        {"@id": "did:web:localhost:template:fb#c631"},
        {"@id": "did:web:localhost:template:fb#1a91"}
      ]}},
      {"@id": "did:web:localhost:template:fb#0fe4", "@type": "dcs:LayoutNode", "dcs:children": {"@list": [
        {"@id": "did:web:localhost:template:fb#f969"},
        {"@id": "did:web:localhost:template:fb#c006"}
      ]}},
      {"@id": "did:web:localhost:template:fb#f969", "@type": "dcs:LayoutNode", "dcs:children": {"@list": [
        {"@id": "did:web:localhost:template:fb#7e2a"},
        {"@id": "did:web:localhost:template:fb#603f"},
        {"@id": "did:web:localhost:template:fb#9325"},
        {"@id": "did:web:localhost:template:fb#0a85"},
        {"@id": "did:web:localhost:template:fb#6ea7"},
        {"@id": "did:web:localhost:template:fb#e03f"}
      ]}},
      {"@id": "did:web:localhost:template:fb#1a91", "@type": "dcs:LayoutNode", "dcs:children": {"@list": [
        {"@id": "did:web:localhost:template:fb#125a"}
      ]}}
    ]
  },
  "dcs:metadata": {"@id": "did:web:localhost:template:fb#md", "@type": "dcs:TemplateMetadata", "dcs:title": "fdsfdsf"}
}`
