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
	defer resp.Body.Close()
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

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	defer resp.Body.Close()
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
// "pdf", jsonld as "payload", optionally vcBytes as "vc", and optionally
// manifestURL as "manifest_url" (DCS-OR-C2PA-008). When vcBytes is non-nil the
// request proceeds even if the JSON-LD payload is unchanged.
// Returns the updated PDF bytes and the renderer version header.
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

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	pdf, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update read: %w", err)
	}
	return pdf, resp.Header.Get("X-PDF-Core-Version"), nil
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

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("pdf-core verify: %w", err)
	}
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	manifest, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core extract-manifest read: %w", err)
	}
	return manifest, nil
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
