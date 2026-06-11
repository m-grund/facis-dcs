package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	compiler "example.com/m/V2/compiler"
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

// buildMultipartBody constructs an io.Reader and Content-Type header for a
// multipart/form-data body containing "pdf" and "payload" fields.
func buildMultipartBody(t *testing.T, pdf []byte, payload string) (io.Reader, string) {
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
	return &buf, "multipart/form-data; boundary=" + boundary
}

// buildMultipartBodyWithVC is like buildMultipartBody but also includes a "vc" field.
func buildMultipartBodyWithVC(t *testing.T, pdf []byte, payload string, vc []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	boundary := mw.Boundary()
	for name, data := range map[string][]byte{"pdf": pdf, "payload": []byte(payload), "vc": vc} {
		fw, err := mw.CreateFormField(name)
		if err != nil {
			t.Fatalf("multipart CreateFormField %s: %v", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("multipart write %s: %v", name, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return &buf, "multipart/form-data; boundary=" + boundary
}

// errorName parses the "name" field from a JSON error response body.
func errorName(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse error body: %v (body: %s)", err, body)
	}
	return v.Name
}

// doRequest performs a request against newServer() and returns the recorder.
func doRequest(method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	newServer().ServeHTTP(rec, req)
	return rec
}

// compilePDF is a test helper that compiles minimalPayload and returns the PDF bytes.
func compilePDF(t *testing.T) []byte {
	t.Helper()
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString(minimalPayload), "application/ld+json")
	if rec.Code != http.StatusOK {
		t.Fatalf("compile: status %d, body: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// ---- Download ---------------------------------------------------------------

func TestDownload_ValidPayload(t *testing.T) {
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString(minimalPayload), "application/ld+json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	pdf := rec.Body.Bytes()
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_ApplicationJSON(t *testing.T) {
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString(minimalPayload), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString("hello"), "text/plain")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

func TestDownload_InvalidPayload(t *testing.T) {
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString("not valid json-ld"), "application/ld+json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

func TestDownload_EquivalentJSONLDFlavorsProduceIdenticalPDF(t *testing.T) {
	bodies := []string{minimalPayload, minimalPayloadFlavorPrefixed, minimalPayloadFlavorExpanded}
	results := make([][]byte, 0, len(bodies))
	for i, payload := range bodies {
		rec := doRequest(http.MethodPost, "/download",
			bytes.NewBufferString(payload), "application/ld+json")
		if rec.Code != http.StatusOK {
			t.Fatalf("Download flavor %d failed: status %d", i+1, rec.Code)
		}
		results = append(results, rec.Body.Bytes())
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
	rec := doRequest(http.MethodPost, "/download",
		bytes.NewBufferString(malformed), "application/ld+json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
	var v struct{ Message string `json:"message"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	msg := v.Message
	if !strings.Contains(msg, "path=<http://127.0.0.1:8080/ontology/dcs-pdf-core#heading>") ||
		!strings.Contains(msg, "path=<http://127.0.0.1:8080/ontology/dcs-pdf-core#name>") ||
		!strings.Contains(msg, "component=<http://www.w3.org/ns/shacl#MinCountConstraintComponent>") {
		t.Fatalf("expected detailed validation report with paths, got: %s", msg)
	}
}

// ---- Verify -----------------------------------------------------------------

func TestVerify_ValidPDF(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for a valid compiled PDF")
	}
}

func TestVerify_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// ---- Update -----------------------------------------------------------------

func TestUpdate_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/update",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

func TestUpdate_MissingPDFField(t *testing.T) {
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	rec := doRequest(http.MethodPost, "/update",
		bytes.NewReader(multipartBody), "multipart/form-data; boundary=boundary")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

// ---- OntologyContext --------------------------------------------------------

func TestOntologyContext(t *testing.T) {
	rec := doRequest(http.MethodGet, "/ontology/dcs-pdf-core", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	res := rec.Body.Bytes()
	if len(res) == 0 {
		t.Fatal("expected non-empty ontology context")
	}
	if !json.Valid(res) {
		t.Fatal("ontology context is not valid JSON")
	}
}

// TestVersionEndpointReturnsVersion verifies that GET /version returns a JSON
// object containing the renderer version string.
func TestVersionEndpointReturnsVersion(t *testing.T) {
	rec := doRequest(http.MethodGet, "/version", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json content type, got %q", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	v, ok := body["version"]
	if !ok || v == "" {
		t.Fatalf("response body must contain non-empty \"version\" field, got %v", body)
	}
}

// TestDownloadResponseCarriesVersionHeader verifies that POST /download
// includes an X-PDF-Core-Version header in the response.
func TestDownloadResponseCarriesVersionHeader(t *testing.T) {
	rec := doRequest(http.MethodPost, "/download",
		strings.NewReader(minimalPayload), "application/ld+json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if v := rec.Header().Get("X-PDF-Core-Version"); v == "" {
		t.Error("POST /download response must include X-PDF-Core-Version header")
	}
}

// TestUpdateResponseCarriesVersionHeader verifies that POST /update
// includes an X-PDF-Core-Version header in the response.
func TestUpdateResponseCarriesVersionHeader(t *testing.T) {
	// First compile a base PDF.
	baseRec := doRequest(http.MethodPost, "/download",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: expected 200, got %d", baseRec.Code)
	}
	basePDF := baseRec.Body.Bytes()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayloadAmended))
	mw.Close()

	rec := doRequest(http.MethodPost, "/update", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if v := rec.Header().Get("X-PDF-Core-Version"); v == "" {
		t.Error("POST /update response must include X-PDF-Core-Version header")
	}
}

// TestUpdateWithVCEmbedsAttachment verifies that POST /update with an optional
// "vc" multipart field embeds the VC bytes as "contract-lifecycle-vc.json" in
// the returned PDF.
func TestUpdateWithVCEmbedsAttachment(t *testing.T) {
	baseRec := doRequest(http.MethodPost, "/download",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: %d", baseRec.Code)
	}
	basePDF := baseRec.Body.Bytes()

	vcBytes := []byte(`{"type":["VerifiableCredential"],"id":"urn:dcs:vc:test"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayloadAmended))
	vcPart, _ := mw.CreateFormField("vc")
	_, _ = vcPart.Write(vcBytes)
	mw.Close()

	rec := doRequest(http.MethodPost, "/update", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("contract-lifecycle-vc.json")) {
		t.Error("response PDF must contain the VC attachment name")
	}
}

// TestUpdateWithVCUnchangedPayloadProceeds verifies that POST /update with a
// "vc" field succeeds even when the payload is identical to the current
// embedded one, because the VC attachment is itself a provenance event.
func TestUpdateWithVCUnchangedPayloadProceeds(t *testing.T) {
	baseRec := doRequest(http.MethodPost, "/download",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: %d", baseRec.Code)
	}
	basePDF := baseRec.Body.Bytes()

	vcBytes := []byte(`{"type":["VerifiableCredential"],"id":"urn:dcs:vc:genesis"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayload)) // same payload as base
	vcPart, _ := mw.CreateFormField("vc")
	_, _ = vcPart.Write(vcBytes)
	mw.Close()

	rec := doRequest(http.MethodPost, "/update", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (VC-only update), got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("contract-lifecycle-vc.json")) {
		t.Error("response PDF must contain the VC attachment name")
	}
}

// TestOntologyContextSubstitutesBaseURL verifies that when
// DCS_PDF_CORE_ONTOLOGY_BASE_URL is set, the context endpoint replaces the
// hardcoded default base URL with the configured one.
func TestOntologyContextSubstitutesBaseURL(t *testing.T) {
	t.Setenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL", "https://example.com")
	rec := doRequest(http.MethodGet, "/ontology/dcs-pdf-core", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	res := rec.Body.Bytes()
	if bytes.Contains(res, []byte("http://127.0.0.1:8080")) {
		t.Error("context response still contains default base URL after substitution")
	}
	if !bytes.Contains(res, []byte("https://example.com")) {
		t.Error("context response does not contain configured base URL")
	}
}

// ---- OntologyOwl ------------------------------------------------------------

func TestOntologyOwl(t *testing.T) {
	rec := doRequest(http.MethodGet, "/ontology/dcs-pdf-core.owl", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	res := rec.Body.Bytes()
	if len(res) == 0 {
		t.Fatal("expected non-empty OWL bytes")
	}
	if !json.Valid(res) {
		t.Fatal("OWL response is not valid JSON")
	}
}

// ---- Verify (amended document) ----------------------------------------------

// TestVerify_AmendedDocument proves that /verify accepts a PDF produced by
// /update and appends a verification witness.
func TestVerify_AmendedDocument(t *testing.T) {
	original := compilePDF(t)

	body, ct := buildMultipartBody(t, original, minimalPayloadAmended)
	recUpdate := doRequest(http.MethodPost, "/update", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d: %s", recUpdate.Code, recUpdate.Body.String())
	}
	amended := recUpdate.Body.Bytes()
	if !bytes.HasPrefix(amended, original) {
		t.Fatal("amended PDF must preserve original bytes as prefix (C2PA invariant)")
	}

	recVerify := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(amended), "application/pdf")
	if recVerify.Code != http.StatusOK {
		t.Fatalf("verify amended: status %d: %s", recVerify.Code, recVerify.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(recVerify.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for valid amended PDF")
	}
}

// TestVerify_AmendedDocumentRejectsCorruption proves that /verify rejects an
// amended PDF whose incremental section has been tampered with.
func TestVerify_AmendedDocumentRejectsCorruption(t *testing.T) {
	original := compilePDF(t)

	body, ct := buildMultipartBody(t, original, minimalPayloadAmended)
	recUpdate := doRequest(http.MethodPost, "/update", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d", recUpdate.Code)
	}
	amended := recUpdate.Body.Bytes()

	corrupted := append([]byte(nil), amended...)
	corrupted[len(original)+50] ^= 0xFF

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(corrupted), "application/pdf")
	if rec.Code == http.StatusOK {
		t.Fatal("expected Verify to reject a corrupted amended document")
	}
}

// ---- Claim ------------------------------------------------------------------

// compileAndStripPayload compiles minimalPayload and strips the embedded JSON-LD.
func compileAndStripPayload(t *testing.T) (fullPDF, strippedPDF []byte) {
	t.Helper()
	full := compilePDF(t)
	stripped, err := compiler.StripEmbeddedJSONLD(full)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}
	return full, stripped
}

// TestClaim_MatchingPayload verifies that /claim accepts a stripped PDF paired
// with the correct JSON-LD.
func TestClaim_MatchingPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	body, ct := buildMultipartBody(t, stripped, minimalPayload)
	rec := doRequest(http.MethodPost, "/claim", body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: status %d: %s", rec.Code, rec.Body.String())
	}
	result := rec.Body.Bytes()
	if !bytes.HasPrefix(result, []byte("%PDF-")) {
		t.Fatal("result must be a PDF")
	}
	baseline, _ := compiler.CompilePDF([]byte(minimalPayload))
	if len(result) <= len(baseline) {
		t.Fatal("result must include a witness appendix and be larger than a bare compilation")
	}
}

// TestClaim_WrongContentType verifies that a non-multipart Content-Type is
// rejected with 415.
func TestClaim_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/claim",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// TestClaim_MismatchedPayload verifies that /claim rejects a payload whose
// compiled output does not match the submitted PDF's page content.
func TestClaim_MismatchedPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	body, ct := buildMultipartBody(t, stripped, minimalPayloadAmended)
	rec := doRequest(http.MethodPost, "/claim", body, ct)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "conflict" {
		t.Fatalf("expected conflict, got %q", name)
	}
}

// TestClaim_MissingPDFField verifies that /claim rejects a request without
// the pdf field.
func TestClaim_MissingPDFField(t *testing.T) {
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	rec := doRequest(http.MethodPost, "/claim",
		bytes.NewReader(multipartBody), "multipart/form-data; boundary=boundary")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

// ---- Verify JSON response ---------------------------------------------------

// verifyResult matches the JSON body returned by POST /verify.
type verifyResult struct {
	Match              bool   `json:"match"`
	C2PASignatureValid bool   `json:"c2pa_signature_valid"`
	VCBytes            string `json:"vc_bytes,omitempty"` // base64-encoded VC JSON
	VCProofValid       bool   `json:"vc_proof_valid"`
}

// TestVerify_ReturnsJSON verifies that POST /verify returns application/json
// with a match=true body for a valid compiled PDF.
func TestVerify_ReturnsJSON(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json response, got %q", ct)
	}

	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for a valid compiled PDF")
	}
	if !result.C2PASignatureValid {
		t.Error("expected c2pa_signature_valid=true for a valid compiled PDF")
	}
}

// TestVerify_JSONIncludesVCBytesWhenPresent verifies that when a PDF contains
// a contract-lifecycle-vc.json attachment /verify returns its base64 bytes in
// the vc_bytes field so the backend can check the status list without parsing
// PDF bytes itself.
func TestVerify_JSONIncludesVCBytesWhenPresent(t *testing.T) {
	original := compilePDF(t)
	vcJSON := []byte(`{"type":["VerifiableCredential"],"credentialSubject":{"status":"active"}}`)

	body, ct := buildMultipartBodyWithVC(t, original, minimalPayloadAmended, vcJSON)
	recUpdate := doRequest(http.MethodPost, "/update", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d: %s", recUpdate.Code, recUpdate.Body.String())
	}
	withVC := recUpdate.Body.Bytes()

	recVerify := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(withVC), "application/pdf")
	if recVerify.Code != http.StatusOK {
		t.Fatalf("verify with VC: status %d: %s", recVerify.Code, recVerify.Body.String())
	}

	var result verifyResult
	if err := json.NewDecoder(recVerify.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if result.VCBytes == "" {
		t.Error("expected vc_bytes to be non-empty when PDF contains contract-lifecycle-vc.json")
	}
}

// TestVerify_JSONMatchFalseOnMismatch verifies that tampered content returns
// match=false and status 409 instead of 200.
func TestVerify_JSONMatchFalseOnMismatch(t *testing.T) {
	pdf := compilePDF(t)
	// Flip a byte inside the page content stream to break the match.
	corrupted := append([]byte(nil), pdf...)
	// Find "clause" text in the content stream and flip a byte.
	idx := bytes.Index(corrupted, []byte("("))
	if idx > 0 {
		corrupted[idx+1] ^= 0x01
	}

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(corrupted), "application/pdf")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for tampered PDF, got %d", rec.Code)
	}
}
