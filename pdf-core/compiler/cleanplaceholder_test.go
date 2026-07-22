package compiler

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// cleanPlaceholderPayload is the ADR-15 clean form as it reaches pdf-core after
// the DCS runs inlinePlaceholderRenderText: contractData is a flat list of typed
// dcs:Placeholder nodes; each clause references a placeholder by a bare {"@id"}
// node onto which the DCS has copied the placeholder's dcs:label and (once
// filled) its dcs:value. The renderer must show the filled value, never the @id.
const cleanPlaceholderPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Clean Placeholder Contract"},
  "dcs:contractData":{"@list":[
    {"@id":"urn:c#f-amount","@type":"dcs:Placeholder","dcs:label":"Amount","dcs:datatype":"xsd:decimal","dcs:value":15000},
    {"@id":"urn:c#f-term","@type":"dcs:Placeholder","dcs:label":"Term","dcs:datatype":"xsd:integer","dcs:value":36}
  ]},
  "dcs:policies":{"@type":"odrl:Set","odrl:permission":[{"@type":"odrl:Permission","odrl:action":{"@id":"odrl:use"}}]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["The amount is ",{"@id":"urn:c#f-amount","dcs:label":"Amount","dcs:value":15000}," EUR per ",{"@id":"urn:c#f-term","dcs:label":"Term","dcs:value":36}," months."]}
    ]}
  }
}`

// cleanUnfilledPayload is the template form: the placeholder carries a label but
// no value (unfilled). The renderer must show the empty slot, never the @id.
const cleanUnfilledPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Clean Placeholder Template"},
  "dcs:contractData":{"@list":[
    {"@id":"urn:c#f-amount","@type":"dcs:Placeholder","dcs:label":"Amount","dcs:datatype":"xsd:decimal"}
  ]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["The amount is ",{"@id":"urn:c#f-amount","dcs:label":"Amount"}," EUR."]}
    ]}
  }
}`

func TestCleanPlaceholderRendersFilledValue(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(cleanPlaceholderPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("clean placeholder contract must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if !bytes.Contains(text, []byte("15000")) {
		t.Fatalf("filled placeholder value 15000 not rendered:\n%s", text)
	}
	if !bytes.Contains(text, []byte("36")) {
		t.Fatalf("filled placeholder value 36 not rendered:\n%s", text)
	}
	if bytes.Contains(text, []byte("urn:c#f-amount")) {
		t.Fatalf("placeholder @id leaked into the visible render instead of its value:\n%s", text)
	}
}

func TestCleanPlaceholderUnfilledRendersEmptySlot(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(cleanUnfilledPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("clean placeholder template must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if bytes.Contains(text, []byte("urn:c#f-amount")) {
		t.Fatalf("unfilled placeholder @id leaked into the visible render:\n%s", text)
	}
	if !bytes.Contains(text, []byte("_____")) {
		t.Fatalf("unfilled placeholder did not render the empty slot:\n%s", text)
	}
}

// TestCleanPlaceholderUpdateMatchesFreshCompile is the determinism contract for
// placeholder-bearing documents: an amended contract regenerated via UpdatePDF
// must have byte-identical page content to a fresh CompilePDF of the same
// payload (otherwise a peer's /verify rejects with a content mismatch).
func TestCleanPlaceholderUpdateMatchesFreshCompile(t *testing.T) {
	ctx := context.Background()
	base, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(cleanPlaceholderPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(cleanPlaceholderPayload, "15000", "12000", -1)
	updated, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), base, []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	reference, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("reference compile: %v", err)
	}
	if err := MatchPageContent(updated, reference); err != nil {
		t.Fatalf("clean placeholder updatePDF page content diverges from fresh CompilePDF: %v", err)
	}
}

// cleanPlaceholderVerifyBase mirrors the minimal update-test payload conventions
// (bare @vocab terms, top-level @id) but carries a clean ADR-15 placeholder in
// both contractData and the clause content, so the byte-prefix /verify contract
// is exercised with a filled placeholder present.
const cleanPlaceholderVerifyBase = `{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:clean-ph",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Clean PH Verify"},
  "contractData": [
    {"@id":"urn:doc:clean-ph#f-amount","@type":"Placeholder","label":"Amount","datatype":"xsd:decimal","value":15000}
  ],
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:clean-ph#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:clean-ph#s1", "children": ["urn:doc:clean-ph#c1"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:clean-ph#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:clean-ph#c1", "content": ["The amount is ", {"@id":"urn:doc:clean-ph#f-amount","label":"Amount","value":15000}, " EUR."]}
    ]
  }
}`

// TestCleanContractDataPassesSHACL proves the /render SHACL gate accepts the
// clean ADR-15 format: a flat dcs:contractData list of typed dcs:Placeholder
// nodes (this is the exact shape that used to 400 with a ClassConstraintComponent
// violation because the LinkML shape declared contractData items as
// dcs:DataRequirement). dcs:value is numeric, which must validate against the
// open Placeholder shape.
func TestCleanContractDataPassesSHACL(t *testing.T) {
	loadSHACLForTest(t)
	clean := []byte(`{
		"@context": {"dcs":"https://w3id.org/facis/dcs/ontology/v1#","xsd":"http://www.w3.org/2001/XMLSchema#"},
		"@id":"urn:doc:clean-shacl",
		"@type":"dcs:ContractTemplate",
		"dcs:metadata":{"@id":"urn:doc:clean-shacl#m","@type":"dcs:TemplateMetadata","dcs:title":"Clean SHACL"},
		"dcs:contractData":[
			{"@id":"urn:doc:clean-shacl#f-amount","@type":"dcs:Placeholder","dcs:label":"Payment Amount","dcs:datatype":"xsd:decimal","dcs:shape":{"@id":"urn:shape:PaymentClauseShape"},"dcs:required":true,"dcs:value":15000}
		],
		"dcs:documentStructure":{"@id":"urn:doc:clean-shacl#ds","@type":"dcs:DocumentStructure",
			"dcs:blocks":{"@list":[{"@id":"urn:doc:clean-shacl#c1","@type":"dcs:Clause","dcs:content":{"@list":["Amount ",{"@id":"urn:doc:clean-shacl#f-amount"}]}}]},
			"dcs:layout":[{"@id":"urn:doc:clean-shacl#root","@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:doc:clean-shacl#c1"}]}}]
		}
	}`)
	if err := ValidatePayloadSHACL(clean); err != nil {
		t.Fatalf("clean dcs:Placeholder contractData must pass SHACL, got: %v", err)
	}
}

// TestOldContractDataRejectedBySHACL proves the shape genuinely moved off
// dcs:DataRequirement: the pre-ADR-15 layered form is now a class-constraint
// violation on dcs:contractData.
func TestOldContractDataRejectedBySHACL(t *testing.T) {
	loadSHACLForTest(t)
	old := []byte(`{
		"@context": {"dcs":"https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":"urn:doc:old-shacl",
		"@type":"dcs:ContractTemplate",
		"dcs:metadata":{"@id":"urn:doc:old-shacl#m","@type":"dcs:TemplateMetadata","dcs:title":"Old SHACL"},
		"dcs:contractData":[
			{"@id":"urn:doc:old-shacl#req","@type":"dcs:DataRequirement","dcs:conditionId":"c1","dcs:fields":[
				{"@id":"urn:doc:old-shacl#f","@type":"dcs:RequirementField","dcs:parameterName":"amount"}
			]}
		]
	}`)
	if err := ValidatePayloadSHACL(old); err == nil {
		t.Fatal("old dcs:DataRequirement contractData must now be rejected (the shape requires dcs:Placeholder)")
	}
}

// TestCleanPlaceholderRecompileMatchesPrefix is the /verify contract: the
// original compile of a placeholder document must reproduce byte-for-byte from
// its own embedded payload across an amendment chain.
func TestCleanPlaceholderRecompileMatchesPrefix(t *testing.T) {
	base, err := CompilePDF(testSigningContext(), []byte(cleanPlaceholderVerifyBase), time.Now())
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(cleanPlaceholderVerifyBase, "15000", "12000", -1)
	updated, err := UpdatePDF(testSigningContext(), base, []byte(amended), time.Now())
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	if err := VerifyIncrementalUpdate(testSigningContext(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate rejected an honestly amended placeholder PDF: %v", err)
	}
	// The recompiled visible content must carry the filled value, not the @id.
	if !bytes.Contains(renderedText(t, updated), []byte("12000")) {
		t.Fatalf("amended placeholder value 12000 not rendered")
	}
}
