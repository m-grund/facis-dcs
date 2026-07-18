package dss

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Remote AdES signing over the EU DSS REST "one-document" services (DCS-IR-SI-10,
// DCS-FR-SM-02/-16): DSS is the Signature Creation Application (the QTSP/
// remote-signer stand-in). Signing is the two-call CSC/rQES shape — getDataToSign
// computes the data-to-be-signed, an external key produces the signature value,
// signDocument embeds it — so the private key never has to live inside DSS: in
// the demonstrator the backend PKCS#11 token signs the DTBS; the prod switch is
// pointing SignatureValue production at a remote QTSP unlocked by the wallet.
//
// Signature formats follow the CSC signature_format codes DSS accepts as
// SignatureLevel: PAdES for the PDF (visible AcroForm field placed by pdf-core),
// JAdES for the machine-readable JSON-LD (DCS-FR-SM-02).

// Format is an AdES signature format DSS produces.
type Format string

const (
	// FormatPAdES signs a PDF (ETSI EN 319 142).
	FormatPAdES Format = "PAdES"
	// FormatJAdES signs JSON (ETSI TS 119 182 — JSON Advanced Electronic Signatures).
	FormatJAdES Format = "JAdES"
)

// SignParams carries the AdES parameters a signing operation needs. The signing
// certificate and its chain are base64 DER; SignatureLevel is a DSS enum such as
// "PAdES_BASELINE_B"/"PAdES_BASELINE_T"/"JAdES_BASELINE_B".
type SignParams struct {
	Format              Format
	SignatureLevel      string
	SigningCertificate  string   // base64 DER
	CertificateChain    []string // base64 DER, leaf-to-root (excluding the leaf)
	DigestAlgorithm     string   // e.g. "SHA256"
	SignatureFieldID    string   // PAdES: the existing AcroForm field pdf-core placed
	JAdESSignedEnvelope string   // JAdES: e.g. "ENVELOPING"
}

// HashSigner produces a signature value over the DSS data-to-be-signed. In the
// demonstrator this is the backend PKCS#11 token; in production it is the wallet-
// unlocked remote QTSP. It returns the raw signature value and the DSS signature
// algorithm name (e.g. "ECDSA_SHA256").
type HashSigner func(ctx context.Context, dataToSign []byte) (signatureValue []byte, algorithm string, err error)

// Sign performs the rQES two-call flow and returns the signed document bytes.
func (c *Client) Sign(ctx context.Context, document []byte, name string, params SignParams, sign HashSigner) ([]byte, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("dss: no base URL configured")
	}
	dtbs, err := c.getDataToSign(ctx, document, name, params)
	if err != nil {
		return nil, err
	}
	signatureValue, algorithm, err := sign(ctx, dtbs)
	if err != nil {
		return nil, fmt.Errorf("dss: sign data-to-be-signed: %w", err)
	}
	return c.signDocument(ctx, document, name, params, signatureValue, algorithm)
}

func (c *Client) getDataToSign(ctx context.Context, document []byte, name string, params SignParams) ([]byte, error) {
	body := map[string]any{
		"parameters":     signatureParameters(params),
		"toSignDocument": namedDocument(document, name),
	}
	var out struct {
		Bytes string `json:"bytes"`
	}
	if err := c.postJSON(ctx, "/services/rest/signature/one-document/getDataToSign", body, &out); err != nil {
		return nil, fmt.Errorf("dss: getDataToSign: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(out.Bytes)
	if err != nil {
		return nil, fmt.Errorf("dss: decode data-to-be-signed: %w", err)
	}
	return raw, nil
}

func (c *Client) signDocument(ctx context.Context, document []byte, name string, params SignParams, signatureValue []byte, algorithm string) ([]byte, error) {
	body := map[string]any{
		"parameters":     signatureParameters(params),
		"toSignDocument": namedDocument(document, name),
		"signatureValue": map[string]any{
			"algorithm": algorithm,
			"value":     base64.StdEncoding.EncodeToString(signatureValue),
		},
	}
	var out struct {
		Bytes string `json:"bytes"`
	}
	if err := c.postJSON(ctx, "/services/rest/signature/one-document/signDocument", body, &out); err != nil {
		return nil, fmt.Errorf("dss: signDocument: %w", err)
	}
	signed, err := base64.StdEncoding.DecodeString(out.Bytes)
	if err != nil {
		return nil, fmt.Errorf("dss: decode signed document: %w", err)
	}
	return signed, nil
}

func signatureParameters(params SignParams) map[string]any {
	digest := params.DigestAlgorithm
	if digest == "" {
		digest = "SHA256"
	}
	out := map[string]any{
		"signingCertificate": map[string]any{"encodedCertificate": params.SigningCertificate},
		"signatureLevel":     params.SignatureLevel,
		"digestAlgorithm":    digest,
	}
	if len(params.CertificateChain) > 0 {
		chain := make([]any, 0, len(params.CertificateChain))
		for _, cert := range params.CertificateChain {
			chain = append(chain, map[string]any{"encodedCertificate": cert})
		}
		out["certificateChain"] = chain
	}
	switch params.Format {
	case FormatPAdES:
		out["signaturePackaging"] = "ENVELOPED"
		// Place the visible signature in the existing AcroForm field pdf-core
		// laid out (imageParameters/fieldParameters, DSS 6.x RemoteSignature-
		// Parameters). DCS-FR-SM: PAdES signatures MUST be visible.
		if params.SignatureFieldID != "" {
			out["imageParameters"] = map[string]any{
				"fieldParameters": map[string]any{"fieldId": params.SignatureFieldID},
			}
		}
	case FormatJAdES:
		packaging := params.JAdESSignedEnvelope
		if packaging == "" {
			packaging = "ENVELOPING"
		}
		out["signaturePackaging"] = packaging
		out["jwsSerializationType"] = "COMPACT_SERIALIZATION"
	}
	return out
}

func namedDocument(document []byte, name string) map[string]any {
	return map[string]any{
		"bytes": base64.StdEncoding.EncodeToString(document),
		"name":  name,
	}
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
