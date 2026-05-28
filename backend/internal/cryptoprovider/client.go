// Package cryptoprovider provides a client for signing via the HashiCorp Vault
// transit secrets engine (DCS-IR-SI-12).
//
// DCS never holds private keys (DCS-IR-HI-01). All signing is delegated to Vault.
// The Vault transit API is called directly, which is also what the eclipse-xfsc
// crypto-provider-service calls internally. Configuring DCS against Vault directly
// avoids the dependency on a privately-distributed XFSC image.
//
// Required env vars:
//
//	VAULT_ADDR      – Vault server URL, e.g. http://vault:8200
//	VAULT_TOKEN     – Vault auth token
//	VAULT_TRANSIT_MOUNT – transit engine mount path (default: "transit")
//	VAULT_TRANSIT_KEY   – signing key name (default: "dcs-signing-key")
package cryptoprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls the Vault transit secrets engine to sign data and issue VCs.
type Client struct {
	vaultAddr     string
	vaultToken    string
	transitMount  string
	transitKey    string
	httpClient    *http.Client
}

// NewClient creates a Client.
//   - vaultAddr    – Vault server base URL (VAULT_ADDR)
//   - vaultToken   – Vault auth token (VAULT_TOKEN)
//   - transitMount – transit engine mount path (VAULT_TRANSIT_MOUNT)
//   - transitKey   – signing key name (VAULT_TRANSIT_KEY)
func NewClient(vaultAddr, vaultToken, transitMount, transitKey string) *Client {
	if transitMount == "" {
		transitMount = "transit"
	}
	if transitKey == "" {
		transitKey = "dcs-signing-key"
	}
	return &Client{
		vaultAddr:    strings.TrimRight(vaultAddr, "/"),
		vaultToken:   vaultToken,
		transitMount: transitMount,
		transitKey:   transitKey,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// vaultSignRequest is the body for POST /v1/{mount}/sign/{key}.
type vaultSignRequest struct {
	Input               string `json:"input"`                // base64-encoded data
	MarshalingAlgorithm string `json:"marshaling_algorithm"` // "raw" for compact bytes
}

// vaultSignResponse is the Vault transit sign response envelope.
type vaultSignResponse struct {
	Data struct {
		Signature string `json:"signature"` // "vault:v1:<base64>"
	} `json:"data"`
	Errors []string `json:"errors"`
}

// Sign sends data to Vault transit for signing and returns the raw signature bytes.
// Used for COSE_Sign1 signatures in C2PA manifests (DCS-OR-C2PA-001).
func (c *Client) Sign(ctx context.Context, data []byte) ([]byte, error) {
	payload, err := json.Marshal(vaultSignRequest{
		Input:               base64.StdEncoding.EncodeToString(data),
		MarshalingAlgorithm: "raw",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal vault sign request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/%s/sign/%s", c.vaultAddr, c.transitMount, c.transitKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create vault sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", c.vaultToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call vault sign: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read vault sign response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault sign returned %d: %s", resp.StatusCode, body)
	}

	var result vaultSignResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode vault sign response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("vault sign errors: %v", result.Errors)
	}

	// Vault returns "vault:v1:<base64-signature>"; strip the prefix.
	sigB64 := result.Data.Signature
	if idx := strings.LastIndex(sigB64, ":"); idx >= 0 {
		sigB64 = sigB64[idx+1:]
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("decode vault signature bytes: %w", err)
	}
	return sig, nil
}

// CreateCredential submits an unsigned VC JSON to Vault for LD-proof signing.
// (DCS-OR-C2PA-004, DCS-FR-SM-08). For dev, Vault transit does not natively
// issue LD-proofs — this returns the VC with a placeholder proof so the rest
// of the pipeline can proceed. Replace with a real LD-proof issuer for production.
func (c *Client) CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	// Sign a SHA-256 hash of the VC bytes as a stand-in COSE proof for dev.
	sig, err := c.Sign(ctx, unsignedVC)
	if err != nil {
		return nil, fmt.Errorf("sign VC: %w", err)
	}

	// Wrap the unsigned VC with a minimal proof object.
	var vc map[string]interface{}
	if err := json.Unmarshal(unsignedVC, &vc); err != nil {
		return nil, fmt.Errorf("unmarshal unsigned VC: %w", err)
	}
	vc["proof"] = map[string]interface{}{
		"type":               "Ed25519Signature2020",
		"proofPurpose":       "assertionMethod",
		"verificationMethod": fmt.Sprintf("did:web:%s#key-1", c.transitKey),
		"jws":                base64.RawURLEncoding.EncodeToString(sig),
		// TODO(DCS-OR-C2PA-004): replace with real LD-proof from VC issuance service.
	}

	return json.Marshal(vc)
}
