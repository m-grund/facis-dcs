package compiler

import (
	"bytes"
	"strings"
	"testing"
)

// TestNormalizePayload_SectionsAndClausesInNQuads verifies that clause text and
// section headings appear in the URDNA2015 N-Quads when the payload uses the
// dcs-pdf-core context.  Without this, UpdatePDF cannot detect changes that
// are confined to human-readable content, and /verify cannot prove that the
// embedded payload reproduces the document.
//
// The correct context must map section/clause terms to IRIs — the @vocab key
// does this by expanding all unqualified terms against the dcs-pdf-core base.
func TestNormalizePayload_SectionsAndClausesInNQuads(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcterms": "http://purl.org/dc/terms/"
		},
		"@id": "urn:doc:nquads-test",
		"dcterms:title": "NQuads Test",
		"sections": [
			{"heading": "Section Alpha", "clauses": ["unique clause sentinel"]}
		]
	}`)
	nquads, _, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	if !bytes.Contains(nquads, []byte("unique clause sentinel")) {
		t.Errorf("N-Quads do not contain clause text; sections and clauses must be mapped to IRIs via @vocab in the @context\nProduced N-Quads:\n%s", nquads)
	}
	if !bytes.Contains(nquads, []byte("Section Alpha")) {
		t.Errorf("N-Quads do not contain section heading; heading must be mapped to an IRI via @vocab\nProduced N-Quads:\n%s", nquads)
	}
}

// TestNormalizePayload_ClauseChangeProducesDifferentHash verifies that two
// payloads that differ only in clause content produce different N-Quads hashes.
// This is the property that makes UpdatePDF detect content changes.
func TestNormalizePayload_ClauseChangeProducesDifferentHash(t *testing.T) {
	base := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcterms": "http://purl.org/dc/terms/"
		},
		"@id": "urn:doc:diff-test",
		"dcterms:title": "Diff Test",
		"sections": [
			{"heading": "1. Test", "clauses": ["original clause"]}
		]
	}`)
	amended := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcterms": "http://purl.org/dc/terms/"
		},
		"@id": "urn:doc:diff-test",
		"dcterms:title": "Diff Test",
		"sections": [
			{"heading": "1. Test", "clauses": ["original clause", "added clause"]}
		]
	}`)

	nquadsBase, _, err := NormalizePayload(base)
	if err != nil {
		t.Fatalf("NormalizePayload(base): %v", err)
	}
	nquadsAmended, _, err := NormalizePayload(amended)
	if err != nil {
		t.Fatalf("NormalizePayload(amended): %v", err)
	}
	if bytes.Equal(nquadsBase, nquadsAmended) {
		t.Error("adding a clause must produce different N-Quads; payloads that differ only in clause content must not hash identically")
	}
}

// TestNormalizePayload_ExamplePayloadFullyRDFied verifies that a complete example
// payload — mirroring the structure used in feature files — produces N-Quads that
// contain every piece of semantic content: document IRI, dcterms:title, section
// heading, and both clause strings. If any field is missing from the N-Quads, a
// change to that field would be invisible to UpdatePDF and the /verify determinism
// guarantee would be broken for that field.
func TestNormalizePayload_ExamplePayloadFullyRDFied(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcterms": "http://purl.org/dc/terms/",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:rdf-completeness-test",
		"dcterms:title": "RDF Completeness Test",
		"sections": [{
			"heading": "1. Obligations",
			"clauses": [
				"The service shall produce deterministic output.",
				"All content must appear in the machine-readable RDF graph."
			]
		}]
	}`)

	nquads, _, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	nq := string(nquads)

	checks := []struct {
		desc    string
		present string
	}{
		{"document IRI", "urn:doc:rdf-completeness-test"},
		{"dcterms:title IRI", "http://purl.org/dc/terms/title"},
		{"title text", "RDF Completeness Test"},
		{"dcs-pdf-core#sections IRI", "dcs-pdf-core#sections"},
		{"dcs-pdf-core#heading IRI", "dcs-pdf-core#heading"},
		{"dcs-pdf-core#clauses IRI", "dcs-pdf-core#clauses"},
		{"section heading text", "1. Obligations"},
		{"first clause text", "The service shall produce deterministic output."},
		{"second clause text", "All content must appear in the machine-readable RDF graph."},
	}
	for _, c := range checks {
		if !strings.Contains(nq, c.present) {
			t.Errorf("N-Quads missing %s (%q):\n%s", c.desc, c.present, nq)
		}
	}
}

// TestNormalizePayload_LargeDocumentWithVocab verifies that a complex multi-section
// payload containing inline ontology-link objects, typed values, and signature fields
// can be normalized by URDNA2015 without error when @vocab is present.
//
// Regression: without @vocab the json-gold processor was lenient about value-object
// structure. With @vocab it enforces the JSON-LD spec: a value object ({@value:…})
// cannot carry extra keys beyond @value/@type/@language/@direction. Payloads that
// combined @value with arbitrary properties (e.g. schema:unitCode) were silently
// processed before but now produce "invalid value object: value object has unknown
// keys". The fix is to use valid JSON-LD for typed values (no extra keys in @value
// objects) and to declare the xsd prefix so @type: xsd:integer resolves correctly.
func TestNormalizePayload_LargeDocumentWithVocab(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"prov": "http://www.w3.org/ns/prov#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"schema": "https://schema.org/",
			"dcterms": "http://purl.org/dc/terms/",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "urn:doc:large-document",
		"@type": "prov:Bundle",
		"dcterms:title": "Master Services Agreement",
		"signatureFields": [{"name": "ClientSignature"}],
		"sections": [
			{
				"heading": "1. Scope",
				"clauses": [
					{
						"content": [
							"The provider acts as a ",
							{"@id": "prov:Activity", "schema:name": "prov:Activity"},
							" under this agreement."
						]
					},
					{
						"content": [
							"The cap is ",
							{"@value": "100", "@type": "xsd:integer"},
							" GBP unless agreed in writing."
						]
					}
				]
			}
		]
	}`)
	nquads, _, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload failed for large-document payload with @vocab: %v", err)
	}
	if len(nquads) == 0 {
		t.Fatal("NormalizePayload returned empty N-Quads for large-document payload")
	}
	if !strings.Contains(string(nquads), "1. Scope") {
		t.Error("N-Quads must contain section heading for large-document payload")
	}
}
