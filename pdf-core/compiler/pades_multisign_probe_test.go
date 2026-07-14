package compiler

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// padesTwoFieldPayload mirrors padesTestPayload with a second signature field.
const padesTwoFieldPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:pades-multisign",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "PAdES MultiSign Probe"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:pades-multisign#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:pades-multisign#s1", "children": ["urn:doc:pades-multisign#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:pades-multisign#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:pades-multisign#c1", "content": ["Clause."]}
		]
	},
	"signatureFields": [
		{"@type": "SignatureField", "@id": "urn:doc:pades-multisign#SignerOne", "signatoryName": "SignerOne"},
		{"@type": "SignatureField", "@id": "urn:doc:pades-multisign#SignerTwo", "signatoryName": "SignerTwo"}
	]
}`

// TestPAdESSecondSignatureProbe empirically probes whether the current
// signing stack can append a SECOND approval signature to an already
// PAdES-signed PDF (multi-signer, DCS-FR-SM-07/17). Documents the mechanics
// for the multi-signer design decision; not wired to any product path yet.
func TestPAdESSecondSignatureProbe(t *testing.T) {
	ensurePAdESTestServer(t)

	ctx := context.Background()
	compiledAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base, err := CompilePDF(ctx, []byte(padesTwoFieldPayload), compiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	evidence1 := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.one~a~b"}`)
	embedded, err := EmbedSigningEvidence(base, evidence1)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence(1): %v", err)
	}
	signed1, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES(1): %v", err)
	}

	signed2, err := SignPAdES(ctx, signed1, "SignerTwo", "SignerTwo")
	if err != nil {
		t.Fatalf("SignPAdES(2) on already-signed PDF: %v", err)
	}

	if !bytes.HasPrefix(signed2, signed1) {
		t.Fatal("second signature is not an incremental update: signed1 bytes were rewritten")
	}
	if n := strings.Count(string(signed2), "/SubFilter /ETSI.CAdES.detached"); n != 2 {
		t.Fatalf("expected 2 PAdES signatures, found %d", n)
	}
}
