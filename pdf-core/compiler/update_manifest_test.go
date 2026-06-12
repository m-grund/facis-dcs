package compiler

import (
	"bytes"
	"strings"
	"testing"
)

// TestExtractManifestStore_SucceedsOnCompiledPDF verifies that a freshly
// compiled PDF contains an extractable C2PA manifest store.
func TestExtractManifestStore_SucceedsOnCompiledPDF(t *testing.T) {
	pdf, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	manifest, err := ExtractManifestStore(pdf)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if len(manifest) == 0 {
		t.Fatal("ExtractManifestStore returned empty bytes")
	}
	// JUMBF manifest store starts with a BMFF box whose type is "jumb".
	if !bytes.Contains(manifest, []byte("jumb")) {
		t.Error("manifest store bytes do not contain JUMBF box marker 'jumb'")
	}
}

// TestExtractManifestStore_SucceedsOnUpdatedPDF verifies that an
// incrementally-updated PDF also has an extractable manifest store.
func TestExtractManifestStore_SucceedsOnUpdatedPDF(t *testing.T) {
	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	updated, err := UpdatePDF(original, []byte(minimalPayloadAmended))
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	manifest, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore on updated PDF: %v", err)
	}
	if len(manifest) == 0 {
		t.Fatal("ExtractManifestStore returned empty bytes for updated PDF")
	}
}

// TestUpdatePDFWithRemoteManifest_URLPresentInManifest verifies that the
// remote manifest URL supplied to UpdatePDFWithRemoteManifest is retrievable
// via ExtractManifestStore + ExtractRemoteManifestURL.
func TestUpdatePDFWithRemoteManifest_URLPresentInManifest(t *testing.T) {
	const wantURL = "https://api.example.com/contracts/did:example:abc123/c2pa-manifest"

	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	updated, err := UpdatePDFWithRemoteManifest(original, []byte(minimalPayloadAmended), nil, wantURL)
	if err != nil {
		t.Fatalf("UpdatePDFWithRemoteManifest: %v", err)
	}

	manifest, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}

	got := ExtractRemoteManifestURL(manifest)
	if got != wantURL {
		t.Errorf("remote manifest URL: got %q, want %q", got, wantURL)
	}
}

// TestUpdatePDFWithRemoteManifest_EmptyURL_NoRemoteRef verifies that an
// empty manifest URL produces no remote_manifests entry in the manifest.
func TestUpdatePDFWithRemoteManifest_EmptyURL_NoRemoteRef(t *testing.T) {
	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	updated, err := UpdatePDFWithRemoteManifest(original, []byte(minimalPayloadAmended), nil, "")
	if err != nil {
		t.Fatalf("UpdatePDFWithRemoteManifest: %v", err)
	}

	manifest, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}

	if got := ExtractRemoteManifestURL(manifest); got != "" {
		t.Errorf("expected no remote manifest URL, got %q", got)
	}
	if bytes.Contains(manifest, []byte("remote_manifests")) {
		t.Error("manifest must not contain 'remote_manifests' key when URL is empty")
	}
}

// TestUpdatePDFWithRemoteManifest_WithVC verifies that the VC and the remote
// manifest URL can be provided together.
func TestUpdatePDFWithRemoteManifest_WithVC(t *testing.T) {
	const wantURL = "https://api.example.com/contracts/did:example:vc/c2pa-manifest"
	vcBytes := []byte(`{"type":"VerifiableCredential","proof":{"type":"Ed25519Signature2020"}}`)

	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	updated, err := UpdatePDFWithRemoteManifest(original, []byte(minimalPayloadBase), vcBytes, wantURL)
	if err != nil {
		t.Fatalf("UpdatePDFWithRemoteManifest: %v", err)
	}

	manifest, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if got := ExtractRemoteManifestURL(manifest); got != wantURL {
		t.Errorf("remote manifest URL: got %q, want %q", got, wantURL)
	}
	if _, ok, _ := ExtractEmbeddedVC(updated); !ok {
		t.Error("VC attachment must be present when vcBytes supplied")
	}
}

// TestVerifyIncrementalUpdate_WithRemoteManifestURL verifies that
// VerifyIncrementalUpdate still passes for PDFs produced with a remote
// manifest URL — the URL is re-used during deterministic re-application.
func TestVerifyIncrementalUpdate_WithRemoteManifestURL(t *testing.T) {
	const url = "https://api.example.com/contracts/did:example:verify/c2pa-manifest"

	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	updated, err := UpdatePDFWithRemoteManifest(original, []byte(minimalPayloadAmended), nil, url)
	if err != nil {
		t.Fatalf("UpdatePDFWithRemoteManifest: %v", err)
	}

	if err := VerifyIncrementalUpdate(updated); err != nil {
		t.Errorf("VerifyIncrementalUpdate: %v", err)
	}
}

// TestExtractRemoteManifestURL_AbsentReturnsEmpty verifies that the
// extraction helper returns "" when the manifest has no remote ref.
func TestExtractRemoteManifestURL_AbsentReturnsEmpty(t *testing.T) {
	pdf, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	manifest, err := ExtractManifestStore(pdf)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if got := ExtractRemoteManifestURL(manifest); got != "" {
		t.Errorf("expected empty URL from compiled PDF manifest, got %q", got)
	}
}

// TestExtractRemoteManifestURL_RoundTrip verifies URL round-trip for a range
// of URL lengths, ensuring the CBOR length encoding handles both the short
// (≤23 byte) and the uint8-prefix (24–255 byte) encoding ranges.
func TestExtractRemoteManifestURL_RoundTrip(t *testing.T) {
	cases := []string{
		"https://x.io/a",                                                  // short (<24 bytes)
		"https://api.example.com/contracts/did:example:abc/c2pa-manifest", // typical (>23 bytes)
		"https://" + strings.Repeat("a", 200) + ".example.com/manifest",  // long (>255 bytes would need 2-byte length)
	}
	original, err := CompilePDF([]byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	for _, url := range cases {
		t.Run(url[:min(len(url), 30)], func(t *testing.T) {
			updated, err := UpdatePDFWithRemoteManifest(original, []byte(minimalPayloadAmended), nil, url)
			if err != nil {
				t.Fatalf("UpdatePDFWithRemoteManifest: %v", err)
			}
			manifest, err := ExtractManifestStore(updated)
			if err != nil {
				t.Fatalf("ExtractManifestStore: %v", err)
			}
			if got := ExtractRemoteManifestURL(manifest); got != url {
				t.Errorf("URL round-trip failed: got %q, want %q", got, url)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
