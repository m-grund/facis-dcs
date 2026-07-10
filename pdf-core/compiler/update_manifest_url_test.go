package compiler

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestUpdatePDFWithManifestURLEmbedsXMPProvenance verifies that
// UpdatePDFWithOptions references the remote manifest URL (DCS-OR-C2PA-008
// AC3) via the C2PA-normative XMP dcterms:provenance link, NOT via a
// non-standard "remote_manifests" claim field. c2pa-rs 0.85.1 (c2patool
// 0.26.61) hard-rejects an unrecognized "remote_manifests" V2 claim field
// ("claim could not be converted from CBOR"), which broke c2patool/veraPDF
// validation for every DCS-produced PDF that ever had a manifest URL — i.e.
// almost all of them, since every lifecycle update passes one. The XMP link
// is the mechanism real C2PA-conformant tools actually expect for remote
// manifest discovery.
func TestUpdatePDFWithManifestURLEmbedsXMPProvenance(t *testing.T) {
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

	if !bytes.Contains(updated, []byte("dcterms:provenance")) {
		t.Error("updated PDF's XMP metadata does not contain a dcterms:provenance property")
	}
	if !bytes.Contains(updated, []byte(manifestURL)) {
		t.Errorf("updated PDF does not contain the remote manifest URL %q anywhere", manifestURL)
	}

	store, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if bytes.Contains(store, []byte("remote_manifests")) {
		t.Error("manifest store must no longer contain the non-standard remote_manifests claim field")
	}

	if err := VerifyIncrementalUpdate(context.Background(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate must still pass with the XMP provenance link embedded: %v", err)
	}
}

// TestUpdatePDFWithoutManifestURLHasNoProvenanceLink verifies the default path
// (no manifest URL) emits no dcterms:provenance XMP property and no
// remote_manifests claim field — matching pdf-core/features/manifest_url.feature's
// "absent" scenario.
func TestUpdatePDFWithoutManifestURLHasNoProvenanceLink(t *testing.T) {
	original, err := CompilePDF(context.Background(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}
	updated, err := UpdatePDF(context.Background(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	if bytes.Contains(updated, []byte("dcterms:provenance")) {
		t.Error("updated PDF must not contain a dcterms:provenance property when no manifest URL is supplied")
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
