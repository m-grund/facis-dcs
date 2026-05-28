// Package cryptoprovider provides a client for the XFSC Crypto Provider Service (DCS-IR-SI-12).
// The service handles VC/VP signing, COSE_Sign1, and DID operations without the DCS process
// holding any private keys (keys are in HSM per DCS-IR-HI-01).
//
// API reference: https://github.com/eclipse-xfsc/crypto-provider-service
package cryptoprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client calls the XFSC Crypto Provider Service REST API.
type Client struct {
	baseURL    string
	namespace  string
	key        string
	httpClient *http.Client
}

// NewClient creates a Client. baseURL is typically from the CRYPTO_PROVIDER_URL env var.
// namespace and key identify the signing key within the Vault transit engine.
func NewClient(baseURL, namespace, key string) *Client {
	return &Client{
		baseURL:   baseURL,
		namespace: namespace,
		key:       key,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// signRequest is the payload for POST /v1/sign.
type signRequest struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Data      string `json:"data"` // base64-encoded bytes to sign
}

// signResult is the response from POST /v1/sign.
type signResult struct {
	Signature string `json:"signature"` // base64-encoded signature bytes
}

// Sign sends data to the Crypto Provider Service for signing and returns the raw signature bytes.
// The service signs with the key identified by the client's namespace/key fields.
// This is used to produce the COSE_Sign1 signature for C2PA manifests (DCS-OR-C2PA-001).
func (c *Client) Sign(ctx context.Context, data []byte) ([]byte, error) {
	payload := signRequest{
		Namespace: c.namespace,
		Key:       c.key,
		Data:      base64.StdEncoding.EncodeToString(data),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/sign", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-namespace", c.namespace)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call sign endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sign response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sign endpoint returned %d: %s", resp.StatusCode, respBody)
	}

	var result signResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode sign response: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode signature bytes: %w", err)
	}
	return sig, nil
}

// createCredentialRequest is the payload for POST /v1/credential.
type createCredentialRequest struct {
	Credential json.RawMessage `json:"credential"`
}

// CreateCredential submits an unsigned VC JSON to the Crypto Provider Service, which adds a
// linked-data proof and returns the signed VC. Used for C2PA VC binding (DCS-OR-C2PA-004)
// and signing summary VCs (DCS-FR-SM-08).
func (c *Client) CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	payload := createCredentialRequest{Credential: unsignedVC}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal credential request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/credential", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create credential request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-namespace", c.namespace)
	req.Header.Set("x-format", "ldp_vc")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call create-credential endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read credential response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create-credential endpoint returned %d: %s", resp.StatusCode, respBody)
	}

	return json.RawMessage(respBody), nil
}
