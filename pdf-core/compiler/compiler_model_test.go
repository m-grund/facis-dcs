package compiler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCompilePDF_NoOntologyHTTPFetch verifies that compilation makes no outbound
// HTTP calls to ontology endpoints. If ontologyTerms ever fetches again, the
// test server will receive a request and the test will fail.
func TestCompilePDF_NoOntologyHTTPFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request to ontology server: %s %s", r.Method, r.URL)
		http.Error(w, "not expected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "` + srv.URL + `/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:no-fetch-test",
		"title": "No Fetch Test",
		"sections": [{"heading": "1. Terms", "clauses": ["No ontology fetch should occur."]}]
	}`)

	_, err := CompilePDF(context.Background(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
}

// TestCompilePDF_DcsCoreIRITitleExtracted verifies that a document whose title
// is expressed via the dcs-pdf-core:title IRI is correctly extracted and rendered
// as the document heading.
//
// Failure mode: if initOntologyIRI is not called or modelTitleIRIs is stale,
// the title is invisible and the PDF compilation returns an error.
func TestCompilePDF_DcsCoreIRITitleExtracted(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:title-iri-test",
		"title": "IRI Title Test Document",
		"sections": [{"heading": "1. Terms", "clauses": ["A clause."]}]
	}`)

	pdf, err := CompilePDF(context.Background(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(IRI Title Test Document) Tj")) {
		t.Errorf("dcs-pdf-core:title must be RENDERED in the page content stream; " +
			"model extraction must use IRI-based lookup, not verbatim JSON key matching.")
	}
}

// TestCompilePDF_PrefixedTermsExtracted verifies that section properties declared
// with the explicit dcs-pdf-core: prefix (rather than relying on @vocab shorthand)
// are correctly recognised and rendered by the IRI-based model extractor.
func TestCompilePDF_PrefixedTermsExtracted(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:prefix-test",
		"dcs-pdf-core:title": "Prefix Test Document",
		"dcs-pdf-core:sections": [{
			"dcs-pdf-core:heading": "1. Obligations",
			"dcs-pdf-core:clauses": ["All terms shall use prefixed IRIs."]
		}]
	}`)

	pdf, err := CompilePDF(context.Background(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(Prefix Test Document) Tj")) {
		t.Errorf("dcs-pdf-core:title must be rendered when using explicit prefix (not via @vocab)")
	}
	if !bytes.Contains(pdf, []byte("(1. Obligations) Tj")) {
		t.Errorf("dcs-pdf-core:heading must be rendered; sections must be found by IRI, not verbatim key")
	}
}

// TestCompilePDF_TitleFieldRendered verifies that the document "title"
// field (dcsCoreIRI + "title") is used as the PDF heading.
func TestCompilePDF_TitleFieldRendered(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:title-test",
		"title": "My Contract Template",
		"sections": [{"heading": "1. Terms", "clauses": ["A clause."]}]
	}`)

	pdf, err := CompilePDF(context.Background(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(My Contract Template) Tj")) {
		t.Errorf("dcs-pdf-core:title must be rendered as the document heading")
	}
}

// TestCompilePDF_MissingTitleReturnsError verifies that CompilePDF returns an
// error when the payload contains no dcs-pdf-core:title. The title IRI is
// mandatory; a missing title is a malformed document, not a fallback situation.
func TestCompilePDF_MissingTitleReturnsError(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:no-title-test",
		"sections": [{"heading": "1. Terms", "clauses": ["A clause."]}]
	}`)

	_, err := CompilePDF(context.Background(), payload, time.Now())
	if err == nil {
		t.Error("CompilePDF must return an error when dcs-pdf-core:title is absent")
	}
}

// TestCompilePDF_NonStablePrefixRenderedCompact verifies that ontology-link terms
// from a namespace that is NOT in the internal stable context (e.g. odrl) are still
// rendered with their compact prefix form in the PDF body text when the service
// path (CanonicalizePayload → CompilePDF) is used.
//
// Failure mode before fix: CanonicalizePayload compacts with a fixed stableCtx that
// lacks the odrl prefix, so odrl:Policy becomes "http://www.w3.org/ns/odrl/2/Policy"
// in the canonical payload. CompilePDF then reads back that canonical payload whose
// @context no longer contains the odrl binding, so compactIRI cannot shorten it and
// the full IRI is rendered in the PDF.
func TestCompilePDF_NonStablePrefixRenderedCompact(t *testing.T) {
	raw := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"odrl": "http://www.w3.org/ns/odrl/2/"
		},
		"@id": "urn:doc:odrl-compact-test",
		"title": "ODRL Compact Test",
		"sections": [{"heading": "1. Test", "clauses": [{"content": [
			"Subject to the applicable ",
			{"@id": "odrl:Policy"},
			"."
		]}]}]
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	pdf, err := CompilePDF(context.Background(), canonical, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(odrl:Policy) Tj")) {
		t.Errorf("odrl:Policy must be rendered as compact name after CanonicalizePayload+CompilePDF; full IRI was rendered instead.\ncanonical context: %s", canonical[:min(len(canonical), 300)])
	}
}

