package pdfcore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"digital-contracting-service/internal/middleware"
)

// RendererVersion is kept in sync with pdf-core/compiler/version.go.
// Bump both together when the pdf-core renderer produces different output for
// the same JSON-LD input, so that cached PDFs are invalidated.
const RendererVersion = "1.0.1"

// Client is an HTTP client for the pdf-core microservice.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a Client pointed at baseURL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Version fetches the renderer version string from pdf-core's GET /version.
func (c *Client) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/version", nil)
	if err != nil {
		return "", fmt.Errorf("pdf-core version request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pdf-core version: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return "", err
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("pdf-core version decode: %w", err)
	}
	return body.Version, nil
}

// Download posts jsonld to POST /download and returns the resulting PDF bytes
// plus the renderer version from the X-PDF-Core-Version response header.
func (c *Client) Download(ctx context.Context, jsonld []byte) (pdf []byte, version string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/download", bytes.NewReader(jsonld))
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ld+json")
	forwardBearerToken(ctx, req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	pdf, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download read: %w", err)
	}
	return pdf, resp.Header.Get("X-PDF-Core-Version"), nil
}

// Update posts a multipart request to POST /update containing existingPDF as
// "pdf", jsonld as "payload", and optionally vcBytes as "vc". When vcBytes is
// non-nil the request proceeds even if the JSON-LD payload is unchanged.
// When manifestURL is non-empty it is sent as the "manifest_url" field so
// pdf-core embeds it as the C2PA claim's remote_manifests field
// (DCS-OR-C2PA-008 AC3). Returns the updated PDF bytes and the renderer version
// header.
func (c *Client) Update(ctx context.Context, existingPDF, jsonld, vcBytes []byte, manifestURL string) (pdf []byte, version string, err error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if err := writeField(mw, "pdf", existingPDF); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: write pdf field: %w", err)
	}
	if err := writeField(mw, "payload", jsonld); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: write payload field: %w", err)
	}
	if len(vcBytes) > 0 {
		if err := writeField(mw, "vc", vcBytes); err != nil {
			return nil, "", fmt.Errorf("pdf-core update: write vc field: %w", err)
		}
	}
	if manifestURL != "" {
		if err := writeField(mw, "manifest_url", []byte(manifestURL)); err != nil {
			return nil, "", fmt.Errorf("pdf-core update: write manifest_url field: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("pdf-core update: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/update", &buf)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	forwardBearerToken(ctx, req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	pdf, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update read: %w", err)
	}
	return pdf, resp.Header.Get("X-PDF-Core-Version"), nil
}

// Sign posts a multipart request to POST /sign containing pdf as "pdf",
// fieldName as "field_name", signatoryName as "signatory_name", and, when
// non-empty, evidence as "evidence". pdf-core embeds the evidence attachment
// (embed-first-sign-second, so it falls inside the PAdES ByteRange) and applies
// a PAdES signature in the named AcroForm field, delegating the ECDSA operation
// to the backend's /internal/pades/sign endpoint. Returns the signed PDF bytes
// and the renderer version header.
func (c *Client) Sign(ctx context.Context, pdf []byte, fieldName, signatoryName string, evidence []byte) (signed []byte, version string, err error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if err := writeField(mw, "pdf", pdf); err != nil {
		return nil, "", fmt.Errorf("pdf-core sign: write pdf field: %w", err)
	}
	if err := writeField(mw, "field_name", []byte(fieldName)); err != nil {
		return nil, "", fmt.Errorf("pdf-core sign: write field_name field: %w", err)
	}
	if err := writeField(mw, "signatory_name", []byte(signatoryName)); err != nil {
		return nil, "", fmt.Errorf("pdf-core sign: write signatory_name field: %w", err)
	}
	if len(evidence) > 0 {
		if err := writeField(mw, "evidence", evidence); err != nil {
			return nil, "", fmt.Errorf("pdf-core sign: write evidence field: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("pdf-core sign: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/sign", &buf)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core sign request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	forwardBearerToken(ctx, req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core sign: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	signed, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core sign read: %w", err)
	}
	return signed, resp.Header.Get("X-PDF-Core-Version"), nil
}

// ExtractEvidence posts pdf to POST /evidence/extract and returns the raw
// signing-evidence attachment bytes embedded by Sign, plus whether it was
// present.
func (c *Client) ExtractEvidence(ctx context.Context, pdf []byte) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/evidence/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, false, fmt.Errorf("pdf-core extract-evidence request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("pdf-core extract-evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return nil, false, nil
	}
	if err := checkStatus(resp); err != nil {
		return nil, false, err
	}
	evidence, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("pdf-core extract-evidence read: %w", err)
	}
	return evidence, true, nil
}

// VerifyResult is the structured response from pdf-core POST /verify.
type VerifyResult struct {
	// Match is true when the PDF was deterministically produced from its embedded payload.
	Match bool
	// C2PASignatureValid is true when the C2PA provenance chain is intact.
	C2PASignatureValid bool
	// VCBytes are the raw contract-lifecycle-vc.json bytes from the PDF attachment,
	// present only when the PDF contains that attachment.
	VCBytes []byte
	// VCProofValid is true when a VC attachment is present and its proof is structurally valid.
	VCProofValid bool
}

// Verify posts pdf to POST /verify and returns the structured verification result.
// Returns an error on non-2xx (including 409 content-mismatch).
func (c *Client) Verify(ctx context.Context, pdf []byte) (VerifyResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/verify", bytes.NewReader(pdf))
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	forwardBearerToken(ctx, req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return VerifyResult{}, err
	}
	var body struct {
		Match              bool   `json:"match"`
		C2PASignatureValid bool   `json:"c2pa_signature_valid"`
		VCBytes            string `json:"vc_bytes"`
		VCProofValid       bool   `json:"vc_proof_valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify decode: %w", err)
	}
	result := VerifyResult{
		Match:              body.Match,
		C2PASignatureValid: body.C2PASignatureValid,
		VCProofValid:       body.VCProofValid,
	}
	if body.VCBytes != "" {
		decoded, err := base64.StdEncoding.DecodeString(body.VCBytes)
		if err != nil {
			return VerifyResult{}, fmt.Errorf("pdf-core verify: decode vc_bytes: %w", err)
		}
		result.VCBytes = decoded
	}
	return result, nil
}

// ExtractManifest posts pdf to POST /manifest/extract and returns the raw JUMBF
// C2PA manifest store bytes embedded in the PDF (DCS-OR-C2PA-008).
func (c *Client) ExtractManifest(ctx context.Context, pdf []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/manifest/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	manifest, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest read: %w", err)
	}
	return manifest, nil
}

// forwardBearerToken copies the caller's JWT from ctx onto the outbound pdf-core
// request. pdf-core presents it as its own Authorization header when it calls the
// backend's internal C2PA signing endpoint (DCS-IR-HI-01). When ctx carries no
// token (an unauthenticated internal path) the header is left unset.
func forwardBearerToken(ctx context.Context, req *http.Request) {
	if tok := middleware.GetBearerToken(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
}

// checkStatus returns an error for non-2xx responses, including the status code
// in the message. Hard-fail: callers must not silently swallow pdf-core errors.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("pdf-core %s: status %d: %s", resp.Request.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
}

// writeField writes data as a plain multipart form field.
func writeField(mw *multipart.Writer, name string, data []byte) error {
	w, err := mw.CreateFormField(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
