package compiler

import (
	"context"
	"testing"
	"time"
)

// TestPAdESFieldCarriesSignatureValueAfterLifecycleUpdate reproduces the real
// production flow: a contract PDF accumulates C2PA lifecycle updates (submitted,
// approved) via UpdatePDFWithVC before it is ever signed. Each such update
// supersedes the AcroForm (obj 14) and re-emits the signature widget objects at
// higher object numbers. Only then does the /signature/apply path embed the
// signing evidence and apply the PAdES signature. The signature must still be
// linked to the (now superseded) AcroForm field via /V.
func TestPAdESFieldCarriesSignatureValueAfterLifecycleUpdate(t *testing.T) {
	ensurePAdESTestServer(t)
	ctx := context.Background()

	base, err := CompilePDF(ctx, []byte(padesTestPayload), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	lifecycleVC := []byte(`{"type":["VerifiableCredential","ContractLifecycleCredential"],"status":"approved"}`)
	approved, err := UpdatePDFWithVC(ctx, base, []byte(padesTestPayload), lifecycleVC, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdatePDFWithVC (approve): %v", err)
	}

	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	embedded, err := EmbedSigningEvidence(approved, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}

	signed, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES: %v", err)
	}

	field := findAcroFormFieldByName(t, signed, "SignerOne")
	v := field.Key("V")
	if v.IsNull() {
		t.Fatal("AcroForm field \"SignerOne\" has no /V after a lifecycle update precedes signing: the signature is not linked to its form field")
	}
	if got := v.Key("Type").Name(); got != "Sig" {
		t.Fatalf("field /V resolves to /Type %q, want Sig", got)
	}
	if v.Key("ByteRange").IsNull() {
		t.Fatal("field /V signature dictionary has no /ByteRange")
	}
}
