package compiler

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestUpdatePDFWithManifestURLEmbedsRemoteManifests verifies that
// UpdatePDFWithOptions embeds the remote_manifests claim field
// (DCS-OR-C2PA-008 AC3) when a manifest URL is supplied, and — crucially — that
// the resulting PDF still passes the deterministic incremental-update
// verification (VerifyIncrementalUpdate re-renders the amendment with the same
// remote_manifests recovered from the stored claim, so the byte-for-byte
// determinism check holds).
func TestUpdatePDFWithManifestURLEmbedsRemoteManifests(t *testing.T) {
	const manifestURL = "http://localhost:8991/api/c2pa/manifest/did:example:contract-42"

	original, err := CompilePDF(context.Background(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}

	updated, err := UpdatePDFWithOptions(context.Background(), original,
		[]byte(minimalPayloadAmended), nil, manifestURL, time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithOptions: %v", err)
	}

	store, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if !bytes.Contains(store, []byte("remote_manifests")) {
		t.Error("manifest store does not contain the remote_manifests claim field")
	}
	if !bytes.Contains(store, []byte(manifestURL)) {
		t.Errorf("manifest store does not contain the remote manifest URL %q", manifestURL)
	}

	// The remote_manifests entry must NOT break the deterministic verify.
	if err := VerifyIncrementalUpdate(context.Background(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate must still pass with remote_manifests embedded: %v", err)
	}
}

// TestUpdatePDFWithoutManifestURLHasNoRemoteManifests verifies the default path
// (no manifest URL) emits no remote_manifests field — matching
// pdf-core/features/manifest_url.feature's "absent" scenario.
func TestUpdatePDFWithoutManifestURLHasNoRemoteManifests(t *testing.T) {
	original, err := CompilePDF(context.Background(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}
	updated, err := UpdatePDF(context.Background(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	store, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if bytes.Contains(store, []byte("remote_manifests")) {
		t.Error("manifest store must not contain remote_manifests when no manifest URL is supplied")
	}
	if err := VerifyIncrementalUpdate(context.Background(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate (no manifest url): %v", err)
	}
}
