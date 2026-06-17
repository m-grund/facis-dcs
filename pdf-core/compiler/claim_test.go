package compiler

import (
	"context"
	"testing"
	"time"
)

// claimBase is a self-contained JSON-LD payload used across claim tests.
const claimBase = `{
  "@context": {"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"},
  "@id": "urn:doc:claim-test",
  "title": "Claim Test",
  "clauses": ["Original clause for claim verification."]
}`

// claimAlternate has different clause text so MatchPageContent must reject it.
const claimAlternate = `{
  "@context": {"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"},
  "@id": "urn:doc:claim-test",
  "title": "Claim Test",
  "clauses": ["Completely different content that renders differently."]
}`

// TestStripEmbeddedJSONLD_RemovesPayload verifies that StripEmbeddedJSONLD
// zeroes the stream content of the embedded JSON-LD object without changing
// the surrounding byte structure (so all object offsets remain valid).
func TestStripEmbeddedJSONLD_RemovesPayload(t *testing.T) {
	pdf, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	stripped, err := StripEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}

	// Length must be unchanged (stream is zero-padded, not truncated).
	if len(stripped) != len(pdf) {
		t.Fatalf("stripped PDF length %d != original %d; xref offsets would break", len(stripped), len(pdf))
	}

	// The embedded JSON-LD stream must now contain only null bytes — the clause
	// text may still appear in the page content stream (rendered PDF), but the
	// machine-readable attachment must be zeroed.
	extracted, err := ExtractEmbeddedJSONLD(stripped)
	if err != nil {
		t.Fatalf("ExtractEmbeddedJSONLD after strip: %v", err)
	}
	for _, b := range extracted {
		if b != 0 {
			t.Errorf("stripped embedded JSON-LD stream contains non-zero byte; content was not fully zeroed")
			break
		}
	}
}

// TestStripEmbeddedJSONLD_IsReversible verifies the round-trip property: a PDF
// stripped of its embedded JSON-LD, when submitted to MatchPageContent against
// a fresh compilation from the same payload, must pass.
func TestStripEmbeddedJSONLD_IsReversible(t *testing.T) {
	pdf, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	stripped, err := StripEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}

	canonical, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF canonical: %v", err)
	}
	if err := MatchPageContent(stripped, canonical); err != nil {
		t.Errorf("MatchPageContent after strip: %v", err)
	}
}

// TestMatchPageContent_SamePayloadPasses verifies that two PDFs compiled from
// the same payload have identical page content streams.
func TestMatchPageContent_SamePayloadPasses(t *testing.T) {
	pdfA, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF A: %v", err)
	}
	pdfB, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF B: %v", err)
	}
	if err := MatchPageContent(pdfA, pdfB); err != nil {
		t.Errorf("identical payloads must produce matching page content: %v", err)
	}
}

// TestMatchPageContent_DifferentPayloadFails verifies that two PDFs compiled
// from different payloads are rejected by MatchPageContent.
func TestMatchPageContent_DifferentPayloadFails(t *testing.T) {
	pdfA, err := CompilePDF(context.Background(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF A: %v", err)
	}
	pdfB, err := CompilePDF(context.Background(), []byte(claimAlternate), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF B: %v", err)
	}
	if err := MatchPageContent(pdfA, pdfB); err == nil {
		t.Error("different payloads must produce a MatchPageContent error")
	}
}
