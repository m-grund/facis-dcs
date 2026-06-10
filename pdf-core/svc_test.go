package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"strings"
	"testing"

	compiler "example.com/m/V2/compiler"
	dcspdfcore "example.com/m/V2/gen/dcspdfcore"
)

// minimalPayload is a valid JSON-LD payload that CompilePDF can process.
// The @vocab entry ensures all terms (sections, clauses, heading) expand to
// dcs-pdf-core IRIs and therefore appear in the URDNA2015 N-Quads used for
// determinism checks and change detection.
const minimalPayload = `{
	"@context": {
		"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
		"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "dcs-pdf-core:Document",
	"title": "Service test",
	"sections": [
		{"@type": "dcs-pdf-core:Section", "heading": "1. Test", "clauses": ["clause one"]}
	]
}`

// minimalPayloadAmended adds a second clause to minimalPayload.  With @vocab
// in place, the new clause produces additional N-Quads, making the two payloads
// semantically distinct so UpdatePDF detects the change.
const minimalPayloadAmended = `{
	"@context": {
		"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
		"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "dcs-pdf-core:Document",
	"title": "Service test",
	"sections": [
		{"@type": "dcs-pdf-core:Section", "heading": "1. Test", "clauses": ["clause one", "clause two"]}
	]
}`

const minimalPayloadFlavorPrefixed = `{
	"@context": {
		"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "dcs-pdf-core:Document",
	"dcs-pdf-core:title": "Service test",
	"dcs-pdf-core:sections": [
		{
			"@type": "dcs-pdf-core:Section",
			"dcs-pdf-core:heading": "1. Test",
			"dcs-pdf-core:clauses": ["clause one"]
		}
	]
}`

const minimalPayloadFlavorExpanded = `{
	"@context": {},
	"@id": "urn:doc:svc-test",
	"@type": "http://127.0.0.1:8080/ontology/dcs-pdf-core#Document",
	"http://127.0.0.1:8080/ontology/dcs-pdf-core#title": [{"@value": "Service test"}],
	"http://127.0.0.1:8080/ontology/dcs-pdf-core#sections": [{
		"@type": "http://127.0.0.1:8080/ontology/dcs-pdf-core#Section",
		"http://127.0.0.1:8080/ontology/dcs-pdf-core#heading": [{"@value": "1. Test"}],
		"http://127.0.0.1:8080/ontology/dcs-pdf-core#clauses": [{"@value": "clause one"}]
	}]
}`

func newSvc() dcspdfcore.Service { return &dcspdfcoreService{} }

// buildMultipartUpdate constructs an io.ReadCloser and Content-Type header for
// a multipart/form-data body containing "pdf" and "payload" fields.
func buildMultipartUpdate(t *testing.T, pdf []byte, payload string) (io.ReadCloser, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	boundary := mw.Boundary()
	fw, err := mw.CreateFormField("pdf")
	if err != nil {
		t.Fatalf("multipart CreateFormField pdf: %v", err)
	}
	if _, err := fw.Write(pdf); err != nil {
		t.Fatalf("multipart write pdf: %v", err)
	}
	fw2, err := mw.CreateFormField("payload")
	if err != nil {
		t.Fatalf("multipart CreateFormField payload: %v", err)
	}
	if _, err := fw2.Write([]byte(payload)); err != nil {
		t.Fatalf("multipart write payload: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return io.NopCloser(&buf), "multipart/form-data; boundary=" + boundary
}

// goaErrorName returns the goa error name from a service error.
func goaErrorName(err error) string {
	type namer interface{ GoaErrorName() string }
	if n, ok := err.(namer); ok {
		return n.GoaErrorName()
	}
	return ""
}

// ---- Download ---------------------------------------------------------------

func TestDownload_ValidPayload(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.DownloadPayload{ContentType: "application/ld+json"}
	body := io.NopCloser(bytes.NewBufferString(minimalPayload))
	pdf, err := svc.Download(context.Background(), p, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if !bytes.Equal(pdf[:5], []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_ApplicationJSON(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.DownloadPayload{ContentType: "application/json"}
	body := io.NopCloser(bytes.NewBufferString(minimalPayload))
	pdf, err := svc.Download(context.Background(), p, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(pdf[:5], []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_WrongContentType(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.DownloadPayload{ContentType: "text/plain"}
	body := io.NopCloser(bytes.NewBufferString("hello"))
	_, err := svc.Download(context.Background(), p, body)
	if err == nil {
		t.Fatal("expected unsupported_media_type error")
	}
	if name := goaErrorName(err); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q (err: %v)", name, err)
	}
}

func TestDownload_InvalidPayload(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.DownloadPayload{ContentType: "application/ld+json"}
	body := io.NopCloser(bytes.NewBufferString("not valid json-ld"))
	_, err := svc.Download(context.Background(), p, body)
	if err == nil {
		t.Fatal("expected bad_request error")
	}
	if name := goaErrorName(err); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q (err: %v)", name, err)
	}
}

func TestDownload_EquivalentJSONLDFlavorsProduceIdenticalPDF(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.DownloadPayload{ContentType: "application/ld+json"}

	bodies := []string{minimalPayload, minimalPayloadFlavorPrefixed, minimalPayloadFlavorExpanded}
	results := make([][]byte, 0, len(bodies))
	for i, payload := range bodies {
		pdf, err := svc.Download(context.Background(), p, io.NopCloser(bytes.NewBufferString(payload)))
		if err != nil {
			t.Fatalf("Download flavor %d failed: %v", i+1, err)
		}
		results = append(results, pdf)
	}

	for i := 1; i < len(results); i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatalf("expected identical PDF bytes for semantically equivalent JSON-LD flavors (baseline vs flavor %d)", i+1)
		}
	}
}

func TestDownload_MalformedPayloadReportsValidationDetails(t *testing.T) {
	malformed := `{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:svc-bad",
		"@type": "dcs-pdf-core:Document",
		"title": "Broken",
		"sections": [{"@type": "dcs-pdf-core:Section", "clauses": ["missing heading"]}],
		"signatureFields": [{"@type": "dcs-pdf-core:SignatureField", "label": "Signer label only"}]
	}`

	svc := newSvc()
	_, err := svc.Download(
		context.Background(),
		&dcspdfcore.DownloadPayload{ContentType: "application/ld+json"},
		io.NopCloser(bytes.NewBufferString(malformed)),
	)
	if err == nil {
		t.Fatal("expected bad_request for malformed payload")
	}
	if name := goaErrorName(err); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q (err: %v)", name, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "path=<http://127.0.0.1:8080/ontology/dcs-pdf-core#heading>") ||
		!strings.Contains(msg, "path=<http://127.0.0.1:8080/ontology/dcs-pdf-core#name>") ||
		!strings.Contains(msg, "component=<http://www.w3.org/ns/shacl#MinCountConstraintComponent>") {
		t.Fatalf("expected detailed validation report with paths, got: %s", msg)
	}
}

// ---- Verify -----------------------------------------------------------------

func TestVerify_ValidPDF(t *testing.T) {
	// Compile a PDF first, then verify it.
	svc := newSvc()
	pdf, err := svc.Download(
		context.Background(),
		&dcspdfcore.DownloadPayload{ContentType: "application/ld+json"},
		io.NopCloser(bytes.NewBufferString(minimalPayload)),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	res, err := svc.Verify(
		context.Background(),
		&dcspdfcore.VerifyPayload{ContentType: "application/pdf"},
		io.NopCloser(bytes.NewReader(pdf)),
	)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Witness-appended PDF is larger than original.
	if len(res) <= len(pdf) {
		t.Fatal("verified PDF should be larger than original")
	}
}

func TestVerify_WrongContentType(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.VerifyPayload{ContentType: "application/json"}
	body := io.NopCloser(bytes.NewBufferString("{}"))
	_, err := svc.Verify(context.Background(), p, body)
	if err == nil {
		t.Fatal("expected unsupported_media_type error")
	}
	if name := goaErrorName(err); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// ---- Update -----------------------------------------------------------------

func TestUpdate_WrongContentType(t *testing.T) {
	svc := newSvc()
	p := &dcspdfcore.UpdatePayload{ContentType: "application/json"}
	body := io.NopCloser(bytes.NewBufferString("{}"))
	_, err := svc.Update(context.Background(), p, body)
	if err == nil {
		t.Fatal("expected unsupported_media_type error")
	}
	if name := goaErrorName(err); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

func TestUpdate_MissingPDFField(t *testing.T) {
	// Build a multipart body that's missing the 'pdf' field.
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	svc := newSvc()
	p := &dcspdfcore.UpdatePayload{ContentType: "multipart/form-data; boundary=boundary"}
	body := io.NopCloser(bytes.NewReader(multipartBody))
	_, err := svc.Update(context.Background(), p, body)
	if err == nil {
		t.Fatal("expected bad_request error")
	}
	if name := goaErrorName(err); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

// ---- OntologyContext --------------------------------------------------------

func TestOntologyContext(t *testing.T) {
	svc := newSvc()
	res, err := svc.OntologyContext(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected non-empty ontology context")
	}
	// Must be valid JSON.
	if !json.Valid(res) {
		t.Fatal("ontology context is not valid JSON")
	}
}

// TestOntologyContextSubstitutesBaseURL verifies that when
// DCS_PDF_CORE_ONTOLOGY_BASE_URL is set, the context endpoint replaces the
// hardcoded default base URL with the configured one. This is required so
// that deployments behind a public hostname produce a usable JSON-LD context.
func TestOntologyContextSubstitutesBaseURL(t *testing.T) {
	t.Setenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL", "https://example.com")
	svc := newSvc()
	res, err := svc.OntologyContext(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(res, []byte("http://127.0.0.1:8080")) {
		t.Error("context response still contains default base URL after substitution")
	}
	if !bytes.Contains(res, []byte("https://example.com")) {
		t.Error("context response does not contain configured base URL")
	}
}

// ---- OntologyOwl ------------------------------------------------------------

func TestOntologyOwl(t *testing.T) {
	svc := newSvc()
	res, err := svc.OntologyOwl(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected non-empty OWL bytes")
	}
	if !json.Valid(res) {
		t.Fatal("OWL response is not valid JSON")
	}
}

// ---- Verify (amended document) ----------------------------------------------

// TestVerify_AmendedDocument proves that /verify accepts a PDF produced by
// /update and appends a verification witness, satisfying the same determinism
// guarantee as for a freshly compiled document.
func TestVerify_AmendedDocument(t *testing.T) {
	svc := newSvc()

	// Step 1: compile the original document.
	original, err := svc.Download(
		context.Background(),
		&dcspdfcore.DownloadPayload{ContentType: "application/ld+json"},
		io.NopCloser(bytes.NewBufferString(minimalPayload)),
	)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Step 2: amend the document via /update.
	body, ct := buildMultipartUpdate(t, original, minimalPayloadAmended)
	amended, err := svc.Update(
		context.Background(),
		&dcspdfcore.UpdatePayload{ContentType: ct},
		body,
	)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !bytes.HasPrefix(amended, original) {
		t.Fatal("amended PDF must preserve original bytes as prefix (C2PA invariant)")
	}

	// Step 3: verify the amended document — the embedded new payload must
	// deterministically reproduce the amended PDF.
	result, err := svc.Verify(
		context.Background(),
		&dcspdfcore.VerifyPayload{ContentType: "application/pdf"},
		io.NopCloser(bytes.NewReader(amended)),
	)
	if err != nil {
		t.Fatalf("Verify of amended document: %v", err)
	}
	if len(result) <= len(amended) {
		t.Fatal("verified amended PDF must be larger than input (witness appended)")
	}
}

// TestVerify_AmendedDocumentRejectsCorruption proves that /verify rejects an
// amended PDF whose incremental section has been tampered with.
func TestVerify_AmendedDocumentRejectsCorruption(t *testing.T) {
	svc := newSvc()

	original, err := svc.Download(
		context.Background(),
		&dcspdfcore.DownloadPayload{ContentType: "application/ld+json"},
		io.NopCloser(bytes.NewBufferString(minimalPayload)),
	)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	body, ct := buildMultipartUpdate(t, original, minimalPayloadAmended)
	amended, err := svc.Update(
		context.Background(),
		&dcspdfcore.UpdatePayload{ContentType: ct},
		body,
	)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Corrupt a byte in the incremental section (after the original prefix).
	corrupted := append([]byte(nil), amended...)
	corrupted[len(original)+50] ^= 0xFF

	_, err = svc.Verify(
		context.Background(),
		&dcspdfcore.VerifyPayload{ContentType: "application/pdf"},
		io.NopCloser(bytes.NewReader(corrupted)),
	)
	if err == nil {
		t.Fatal("expected Verify to reject a corrupted amended document")
	}
}

// ---- Claim ------------------------------------------------------------------

// buildClaimMultipart is like buildMultipartUpdate but names the payload field
// "payload" and expects the pdf field to hold a stripped PDF.
// It reuses buildMultipartUpdate since the field names are identical.
func buildClaimMultipart(t *testing.T, pdf []byte, payload string) (io.ReadCloser, string) {
	t.Helper()
	return buildMultipartUpdate(t, pdf, payload)
}

// minimalPayloadStripped returns the bytes of a PDF compiled from minimalPayload
// with the embedded JSON-LD content zeroed out (simulating a PDF that was
// distributed after its file attachment was stripped by a mail client or DMS).
func compileAndStripPayload(t *testing.T) (fullPDF, strippedPDF []byte) {
	t.Helper()
	svc := newSvc()
	full, err := svc.Download(
		context.Background(),
		&dcspdfcore.DownloadPayload{ContentType: "application/ld+json"},
		io.NopCloser(bytes.NewBufferString(minimalPayload)),
	)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	stripped, err := compiler.StripEmbeddedJSONLD(full)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}
	return full, stripped
}

// TestClaim_MatchingPayload verifies that /claim accepts a stripped PDF paired
// with the correct JSON-LD and returns the canonical PDF (with embedded JSON-LD
// and verification witness).
func TestClaim_MatchingPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	svc := newSvc()
	body, ct := buildClaimMultipart(t, stripped, minimalPayload)
	result, err := svc.Claim(
		context.Background(),
		&dcspdfcore.ClaimPayload{ContentType: ct},
		body,
	)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !bytes.HasPrefix(result, []byte("%PDF-")) {
		t.Fatal("result must be a PDF")
	}
	// The canonical PDF (with embedded JSON-LD) must have been returned; a
	// verification witness must have been appended, making it larger than a
	// freshly compiled PDF.
	baseline, _ := compiler.CompilePDF([]byte(minimalPayload))
	if len(result) <= len(baseline) {
		t.Fatal("result must include a witness appendix and be larger than a bare compilation")
	}
}

// TestClaim_WrongContentType verifies that a non-multipart Content-Type is
// rejected with unsupported_media_type.
func TestClaim_WrongContentType(t *testing.T) {
	svc := newSvc()
	_, err := svc.Claim(
		context.Background(),
		&dcspdfcore.ClaimPayload{ContentType: "application/json"},
		io.NopCloser(bytes.NewBufferString("{}")),
	)
	if err == nil {
		t.Fatal("expected unsupported_media_type")
	}
	if name := goaErrorName(err); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// TestClaim_MismatchedPayload verifies that /claim rejects a payload whose
// compiled output does not match the submitted PDF's page content.
func TestClaim_MismatchedPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	svc := newSvc()
	body, ct := buildClaimMultipart(t, stripped, minimalPayloadAmended)
	_, err := svc.Claim(
		context.Background(),
		&dcspdfcore.ClaimPayload{ContentType: ct},
		body,
	)
	if err == nil {
		t.Fatal("expected conflict error for mismatched payload")
	}
	if name := goaErrorName(err); name != "conflict" {
		t.Fatalf("expected conflict, got %q", name)
	}
}

// TestClaim_MissingPDFField verifies that /claim rejects a request without the
// pdf field.
func TestClaim_MissingPDFField(t *testing.T) {
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	svc := newSvc()
	_, err := svc.Claim(
		context.Background(),
		&dcspdfcore.ClaimPayload{ContentType: "multipart/form-data; boundary=boundary"},
		io.NopCloser(bytes.NewReader(multipartBody)),
	)
	if err == nil {
		t.Fatal("expected bad_request")
	}
	if name := goaErrorName(err); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}
