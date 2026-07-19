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

// C2PASignFunc signs one COSE Sig_structure with the DCS dcs-c2pa key and returns
// the 64-byte ES256 r||s. pdf-core holds no key: it prepares the to-be-signed
// Sig_structures, the DCS signs them here, and pdf-core embeds the signatures.
type C2PASignFunc func(sigStructure []byte) ([]byte, error)

// Client is an HTTP client for the pdf-core microservice.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	// sign produces the dcs-c2pa signature for a prepared Sig_structure. The DCS
	// never lets pdf-core see key material, so every C2PA signature is produced
	// here and posted back to pdf-core's /c2pa/embed.
	sign C2PASignFunc
}

// New returns a Client pointed at baseURL. sign is the in-process dcs-c2pa
// signer the two-step render flow uses; it must be non-nil.
func New(baseURL string, sign C2PASignFunc) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		sign:       sign,
	}
}

// preparedC2PA is pdf-core's prepare response: the compiled PDF with zeroed COSE
// signature slots and the Sig_structures the DCS must sign (document order).
type preparedC2PA struct {
	PDFBase64         string   `json:"pdf_base64"`
	C2PASigStructures []string `json:"c2pa_sig_structures"`
}

// embedC2PA is pdf-core's /c2pa/embed request: the prepared PDF and the ES256
// signatures for its zeroed slots, in the order prepare returned them.
type embedC2PA struct {
	PDFBase64      string   `json:"pdf_base64"`
	C2PASignatures []string `json:"c2pa_signatures"`
}

// signAndEmbed signs each prepared Sig_structure with the dcs-c2pa key and posts
// the signatures to pdf-core's stateless /c2pa/embed, returning the finished PDF.
func (c *Client) signAndEmbed(ctx context.Context, prepared preparedC2PA) ([]byte, error) {
	if c.sign == nil {
		return nil, fmt.Errorf("pdf-core client has no C2PA signer configured")
	}
	signatures := make([]string, len(prepared.C2PASigStructures))
	for i, s := range prepared.C2PASigStructures {
		sigStructure, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decode sig_structure %d: %w", i, err)
		}
		sig, err := c.sign(sigStructure)
		if err != nil {
			return nil, fmt.Errorf("sign c2pa sig_structure %d: %w", i, err)
		}
		signatures[i] = base64.StdEncoding.EncodeToString(sig)
	}
	body, err := json.Marshal(embedC2PA{PDFBase64: prepared.PDFBase64, C2PASignatures: signatures})
	if err != nil {
		return nil, fmt.Errorf("marshal c2pa embed request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/c2pa/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core c2pa embed read: %w", err)
	}
	return pdf, nil
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

// Download posts jsonld to POST /render and returns the resulting PDF bytes
// plus the renderer version from the X-PDF-Core-Version response header.
func (c *Client) Download(ctx context.Context, jsonld []byte) (pdf []byte, version string, err error) {
	jsonld, err = flattenComposedStructure(jsonld)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: flatten composed structure: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/render", bytes.NewReader(jsonld))
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ld+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	version = resp.Header.Get("X-PDF-Core-Version")
	var prepared preparedC2PA
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		return nil, "", fmt.Errorf("pdf-core download decode prepared: %w", err)
	}
	pdf, err = c.signAndEmbed(ctx, prepared)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core download: %w", err)
	}
	return pdf, version, nil
}

// Update posts a multipart request to POST /render/amendment containing existingPDF as
// "pdf", jsonld as "payload", and optionally vcBytes as "vc". When vcBytes is
// non-nil the request proceeds even if the JSON-LD payload is unchanged.
// When manifestURL is non-empty it is sent as the "manifest_url" field so
// pdf-core embeds it as the C2PA claim's remote_manifests field
// (DCS-OR-C2PA-008). Returns the updated PDF bytes and the renderer version
// header.
func (c *Client) Update(ctx context.Context, existingPDF, jsonld, vcBytes []byte, manifestURL string) (pdf []byte, version string, err error) {
	jsonld, err = flattenComposedStructure(jsonld)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: flatten composed structure: %w", err)
	}
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
		c.BaseURL+"/render/amendment", &buf)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, "", err
	}
	version = resp.Header.Get("X-PDF-Core-Version")
	var prepared preparedC2PA
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		return nil, "", fmt.Errorf("pdf-core update decode prepared: %w", err)
	}
	pdf, err = c.signAndEmbed(ctx, prepared)
	if err != nil {
		return nil, "", fmt.Errorf("pdf-core update: %w", err)
	}
	return pdf, version, nil
}

// EmbedEvidence posts pdf + evidence to POST /evidence/embed and returns the
// PDF with the evidence attached but NOT signed — the attach-only step a remote
// DSS signer performs before it produces the PAdES signature (so the /ByteRange
// covers the evidence). The default pdf-core signer embeds and signs in one
// call (Sign); this seam splits the two for the DSS backend.
func (c *Client) EmbedEvidence(ctx context.Context, pdf, evidence []byte) (embedded []byte, err error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := writeField(mw, "pdf", pdf); err != nil {
		return nil, fmt.Errorf("pdf-core embed: write pdf field: %w", err)
	}
	if err := writeField(mw, "evidence", evidence); err != nil {
		return nil, fmt.Errorf("pdf-core embed: write evidence field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("pdf-core embed: close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/evidence/embed", &buf)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	embedded, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core embed read: %w", err)
	}
	return embedded, nil
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

// ChainEntry is one manifest in a PDF's C2PA provenance chain: its JUMBF label
// and, when present, its parsed dcs.lifecycle assertion. pdf-core owns the
// JUMBF/CBOR parsing; the DCS consumes this structured form.
type ChainEntry struct {
	Label     string            `json:"label"`
	Lifecycle map[string]string `json:"lifecycle,omitempty"`
}

// ExtractManifestChain returns the parsed C2PA provenance chain embedded in a
// PDF (oldest manifest first).
func (c *Client) ExtractManifestChain(ctx context.Context, pdf []byte) ([]ChainEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/manifest/chain", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var chain []ChainEntry
	if err := json.NewDecoder(resp.Body).Decode(&chain); err != nil {
		return nil, fmt.Errorf("pdf-core manifest-chain decode: %w", err)
	}
	return chain, nil
}

// ExtractPayload returns the machine-readable JSON-LD contract payload embedded
// in a PDF. A peer that receives a contract PDF rebuilds its local copy from
// this, so the DCS never parses PDF bytes itself (ADR-13).
func (c *Client) ExtractPayload(ctx context.Context, pdf []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/payload/extract", bytes.NewReader(pdf))
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pdf-core payload-extract read: %w", err)
	}
	return payload, nil
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
