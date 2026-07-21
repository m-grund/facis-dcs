package jsonld

import (
	"encoding/json"
	"testing"
)

// canonical form as pdf-core embeds it: bare @vocab terms + pdf-core context URL.
const pdfCoreCanonical = `{
  "@context": "https://pdf-core.example/ontology/dcs-pdf-core",
  "@id": "contract-123",
  "@type": "Contract",
  "parentContract": {"@id": "https://dcs.example/contracts/parent-9"},
  "derivedFromTemplate": {"@id": "https://dcs.example/templates/tmpl-7", "version": 3},
  "metadata": {"title": "Service Agreement"},
  "policies": "https://dcs.example/policies/p1"
}`

func TestCompactToFacisRestoresDcsPrefixesOffline(t *testing.T) {
	out, err := CompactToFacis([]byte(pdfCoreCanonical))
	if err != nil {
		t.Fatalf("CompactToFacis: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := doc["dcs:parentContract"]; !ok {
		t.Errorf("expected dcs:parentContract, got keys: %v", keys(doc))
	}
	if _, ok := doc["dcs:policies"]; !ok {
		t.Errorf("expected dcs:policies, got keys: %v", keys(doc))
	}
	if _, ok := doc["dcs:metadata"]; !ok {
		t.Errorf("expected dcs:metadata, got keys: %v", keys(doc))
	}
	if _, ok := doc["derivedFromTemplate"]; !ok {
		t.Errorf("expected derivedFromTemplate alias, got keys: %v", keys(doc))
	}
	if doc["@context"] != "https://pdf-core.example/ontology/dcs-pdf-core" {
		t.Errorf("original @context not preserved: %v", doc["@context"])
	}
	t.Logf("compacted: %s", string(out))
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
