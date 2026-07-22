package compiler

import (
	"bytes"
	"context"
	"testing"
)

// The REAL filled-contract shape: contractData carries a RequirementField with a
// parameterValue (the negotiated filling); a clause Placeholder bindsTo that
// field's @id; children are @id-reference objects; an ODRL policy sits alongside
// and MUST be ignored by the render (attachment-only) but preserved verbatim.
const filledContractPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Filled Contract"},
  "dcs:contractData":{"@list":[
    {"@type":"dcs:DataRequirement","dcs:fields":{"@list":[
      {"@id":"urn:c#field-amount","@type":"dcs:RequirementField","dcs:parameterName":"amount","dcs:parameterValue":"15000"}
    ]}}
  ]},
  "dcs:policies":{"@type":"odrl:Set","odrl:permission":[{"@type":"odrl:Permission","odrl:action":{"@id":"odrl:use"}}]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["Amount ",{"@type":"dcs:Placeholder","dcs:bindsTo":{"@id":"urn:c#field-amount"}}," EUR"]}
    ]}
  }
}`

func renderedText(t *testing.T, pdf []byte) []byte {
	t.Helper()
	streams, err := extractPageContentStreams(pdf)
	if err != nil {
		t.Fatalf("extract page content: %v", err)
	}
	var all []byte
	for _, s := range streams {
		all = append(all, s...)
	}
	return all
}

func TestContractRendersFilledPlaceholderValue(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("filled contract must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if !bytes.Contains(text, []byte("15000")) {
		t.Fatalf("filled placeholder value 15000 not rendered (got the empty slot instead)")
	}
	if bytes.Contains(text, []byte("odrl")) || bytes.Contains(text, []byte("Permission")) {
		t.Fatalf("ODRL policy leaked into the visible render")
	}
	embedded, err := ExtractEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("extract embedded: %v", err)
	}
	if !bytes.Contains(embedded, []byte("odrl:Set")) {
		t.Fatalf("ODRL policy must be preserved verbatim in the attachment")
	}
}
