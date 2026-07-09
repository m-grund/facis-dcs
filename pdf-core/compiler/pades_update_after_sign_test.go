package compiler

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
)

// realisticSigningEvidence mimics the production evidence the /signature/apply
// flow embeds: a ContractSigningSummaryCredential carrying a verbatim SD-JWT VC
// PID presentation. It is deliberately kilobytes-large and multi-line so the
// test exercises the same evidence shape as the live ceremony rather than a
// trivial fixture.
func realisticSigningEvidence() []byte {
	var b strings.Builder
	b.WriteString(`{"@context":["https://www.w3.org/2018/credentials/v1"],`)
	b.WriteString(`"type":["VerifiableCredential","ContractSigningSummaryCredential"],`)
	b.WriteString(`"issuer":"did:web:dcs.example","issuanceDate":"2026-01-01T00:00:00Z",`)
	b.WriteString(`"credentialSubject":{"contractId":"urn:doc:pades-repro",`)
	b.WriteString(`"signerDid":"did:jwk:eyJjcnYiOiJQLTI1NiIsImt0eSI6IkVDIn0",`)
	// A verbatim SD-JWT presentation: a long JWS plus several disclosures and a
	// key-binding JWT, each a sizeable base64url run separated by '~'.
	b.WriteString(`"pidPresentation":"`)
	b.WriteString(strings.Repeat("eyJhbGciOiJFUzI1NiIsInR5cCI6InZjK3NkLWp3dCJ9.", 40))
	b.WriteString("~")
	b.WriteString(strings.Repeat("WyJzYWx0MTIzNDU2Nzg5MCIsImZhbWlseV9uYW1lIiwiTXVzdGVybWFubiJd", 12))
	b.WriteString("~")
	b.WriteString(strings.Repeat("WyJzYWx0OTg3NjU0MzIxMCIsImdpdmVuX25hbWUiLCJFcmlrYSJd", 12))
	b.WriteString("~")
	b.WriteString(strings.Repeat("eyJhbGciOiJFUzI1NiIsInR5cCI6ImtiK2p3dCJ9.", 8))
	b.WriteString(`"}}`)
	return []byte(b.String())
}

// TestPAdESFieldSurvivesLifecycleUpdateAfterSigning reproduces the production
// export path: a contract is PAdES-signed (embed-first-sign-second) and THEN a
// C2PA lifecycle update is appended on export. That update must not strip the
// signature — the AcroForm field must still carry a /V resolving to the /Type
// /Sig value dictionary, and the CMS must still verify over its /ByteRange
// (DCS-OR-C2PA-010). This is the sign-then-update ordering, distinct from the
// update-then-sign case covered elsewhere.
func TestPAdESFieldSurvivesLifecycleUpdateAfterSigning(t *testing.T) {
	ensurePAdESTestServer(t)
	ctx := context.Background()

	base, err := CompilePDF(ctx, []byte(padesTestPayload), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	evidence := realisticSigningEvidence()
	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	signed, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES: %v", err)
	}

	// Sanity: the freshly signed PDF already links the field (the fix must not
	// regress this) — so any failure below is attributable to the later update.
	preField := findAcroFormFieldByName(t, signed, "SignerOne")
	if preField.Key("V").IsNull() {
		t.Fatal("precondition: signed PDF field has no /V before the lifecycle update")
	}

	// Append a lifecycle C2PA update, exactly as the contract export flow does
	// after signing (state change → new lifecycle VC attachment).
	lifecycleVC := []byte(`{"type":["VerifiableCredential","ContractLifecycleCredential"],"status":"signed"}`)
	updated, err := UpdatePDFWithOptions(ctx, signed, []byte(padesTestPayload), lifecycleVC, "", time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdatePDFWithOptions after PAdES signing: %v", err)
	}
	if len(updated) <= len(signed) {
		t.Fatal("lifecycle update did not append to the signed PDF")
	}

	field := findAcroFormFieldByName(t, updated, "SignerOne")
	v := field.Key("V")
	if v.IsNull() {
		t.Fatal("AcroForm field \"SignerOne\" lost its /V after a post-signing lifecycle update: the signature is no longer linked to its form field")
	}
	if got := v.Key("Type").Name(); got != "Sig" {
		t.Fatalf("field /V resolves to /Type %q, want Sig", got)
	}
	if v.Key("ByteRange").IsNull() {
		t.Fatal("field /V signature dictionary has no /ByteRange after the lifecycle update")
	}

	// The signature must still verify cryptographically over its /ByteRange: the
	// appended update lives entirely after the signed range and leaves it intact.
	signedContent, der := byteRangeContentAndCMS(t, updated)
	p7, err := pkcs7.Parse(der)
	if err != nil {
		t.Fatalf("parse CMS from updated PDF: %v", err)
	}
	p7.Content = signedContent
	if err := p7.Verify(); err != nil {
		t.Fatalf("PAdES CMS no longer verifies over its /ByteRange after the lifecycle update: %v", err)
	}

	// The lifecycle VC must actually be present (the update was not a no-op).
	if vc, ok, _ := ExtractEmbeddedVC(updated); !ok || !bytes.Contains(vc, []byte("ContractLifecycleCredential")) {
		t.Fatal("lifecycle VC attachment missing from the updated PDF")
	}
}
