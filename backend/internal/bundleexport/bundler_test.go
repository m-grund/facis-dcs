package bundleexport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"testing"
)

func TestZipWithManifestHashesMatchPackagedBytes(t *testing.T) {
	files := bundleFiles{
		"contract.jsonld":                 []byte(`{"@id":"did:example:c"}`),
		"contract.pdf":                    []byte("%PDF-1.7 fake"),
		"manifest-store.c2pa":             []byte("c2pa-bytes"),
		"credentials/manifest-chain.json": []byte(`[]`),
		"signatures.json":                 []byte(`[]`),
	}
	components := []componentInfo{{DID: "did:example:c", State: "SIGNED", Role: "root"}}

	zipBytes, err := zipWithManifest(files, "contract", "did:example:c", components)
	if err != nil {
		t.Fatalf("zipWithManifest: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	contents := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		contents[f.Name] = data
	}

	// bundle-manifest.json must exist and be excluded from its own entries.
	manifestRaw, ok := contents["bundle-manifest.json"]
	if !ok {
		t.Fatalf("bundle-manifest.json missing from zip")
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Entries) != len(files) {
		t.Fatalf("expected %d entries, got %d", len(files), len(manifest.Entries))
	}

	for _, e := range manifest.Entries {
		if e.Path == "bundle-manifest.json" {
			t.Fatalf("manifest must not list itself")
		}
		packed, ok := contents[e.Path]
		if !ok {
			t.Fatalf("manifest entry %s not present in zip", e.Path)
		}
		sum := sha256.Sum256(packed)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			t.Fatalf("sha256 mismatch for %s", e.Path)
		}
	}

	// Required contract members are present.
	for _, want := range []string{"contract.jsonld", "contract.pdf", "manifest-store.c2pa", "signatures.json"} {
		if _, ok := contents[want]; !ok {
			t.Fatalf("missing required member %s", want)
		}
	}
	if _, ok := contents["credentials/manifest-chain.json"]; !ok {
		t.Fatalf("missing credentials/ member")
	}
}
