// Package cryptoprovider provides a client for the Eclipse XFSC
// Crypto Provider Service (DCS-IR-SI-12).
//
// The service exposes an HTTP API that delegates all cryptographic operations
// to a HashiCorp Vault transit backend via a gRPC sidecar
// (crypto-provider-hashicorp-vault-plugin). DCS never holds private keys
// (DCS-IR-HI-01); all signing is delegated to this service.
//
// Required env vars:
//
//	CRYPTO_PROVIDER_URL       – HTTP base URL of the crypto-provider-service
//	CRYPTO_PROVIDER_NAMESPACE – Vault transit mount path (default: "transit")
//	CRYPTO_PROVIDER_KEY       – signing key name (default: "dcs-signing-key")
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

// Client calls the XFSC Crypto Provider Service HTTP API.
type Client struct {
	url        string
	namespace  string
	key        string
	httpClient *http.Client
}

// NewClient creates a Client.
//   - url       – HTTP base URL of the crypto-provider-service (CRYPTO_PROVIDER_URL)
//   - namespace – Vault transit mount path (CRYPTO_PROVIDER_NAMESPACE)
//   - key       – signing key name (CRYPTO_PROVIDER_KEY)
func NewClient(url, namespace, key string) *Client {
	if namespace == "" {
		namespace = "transit"
	}
	if key == "" {
		key = "dcs-signing-key"
	}
	return &Client{
		url:        strings.TrimRight(url, "/"),
		namespace:  namespace,
		key:        key,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// signRequest is the body for POST /v1/sign.
type signRequest struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Data      string `json:"data"`  // base64-encoded
	Group     string `json:"group"` // key group; empty string for default
}

// signResult is the response from POST /v1/sign.
type signResult struct {
	Signature string `json:"signature"` // base64-encoded
}

// Sign sends data to the crypto-provider-service for signing and returns the
// raw signature bytes. Used for COSE_Sign1 signatures in C2PA manifests
// (DCS-OR-C2PA-001).
func (c *Client) Sign(ctx context.Context, data []byte) ([]byte, error) {
	body, err := json.Marshal(signRequest{
		Namespace: c.namespace,
		Key:       c.key,
		Data:      base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/sign", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-namespace", c.namespace)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call /v1/sign: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sign response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sign returned %d: %s", resp.StatusCode, respBody)
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

// credentialProofRequest is the body for POST /v1/credential/proof.
type credentialProofRequest struct {
	Namespace     string      `json:"namespace"`
	Key           string      `json:"key"`
	Group         string      `json:"group"`
	Credential    interface{} `json:"credential"`    // existing unsigned VC as JSON
	Format        string      `json:"format"`        // "ldp_vc"
	SignatureType string      `json:"signatureType"` // "ed25519signature2020"
}

// CreateCredential adds an Ed25519Signature2020 LD proof to the unsigned VC
// by calling POST /v1/credential/proof on the crypto-provider-service.
// The service handles URDNA2015 canonicalization and proof construction
// internally (DCS-OR-C2PA-004, DCS-FR-SM-08).
func (c *Client) CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	// Unmarshal so we can pass the VC as a structured object.
	var vcDoc interface{}
	if err := json.Unmarshal(unsignedVC, &vcDoc); err != nil {
		return nil, fmt.Errorf("unmarshal unsigned VC: %w", err)
	}

	body, err := json.Marshal(credentialProofRequest{
		Namespace:     c.namespace,
		Key:           c.key,
		Credential:    vcDoc,
		Format:        "ldp_vc",
		SignatureType: "ed25519signature2020",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal credential proof request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/credential/proof", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create credential proof request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-namespace", c.namespace)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call /v1/credential/proof: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read credential proof response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("credential/proof returned %d: %s", resp.StatusCode, respBody)
	}

	return json.RawMessage(respBody), nil
}
