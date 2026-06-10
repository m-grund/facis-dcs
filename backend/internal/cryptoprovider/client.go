// Package cryptoprovider provides a client for the Eclipse XFSC
// Crypto Provider Service (DCS-IR-SI-12).
//
// The service exposes an HTTP API that delegates signing to a HashiCorp Vault
// transit backend via a gRPC sidecar (crypto-provider-hashicorp-vault-plugin).
// DCS never holds private keys (DCS-IR-HI-01); all signing is delegated to this
// service.
//
// Required env vars:
//
//	CRYPTO_PROVIDER_URL       - HTTP base URL of the crypto-provider-service
//	CRYPTO_PROVIDER_NAMESPACE - Vault transit mount path
//	CRYPTO_PROVIDER_KEY       - signing key name for C2PA
//	CRYPTO_PROVIDER_VC_KEY    - signing key name for VC proof signing
package cryptoprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/piprate/json-gold/ld"
)

// Client calls the XFSC Crypto Provider Service HTTP API.
type Client struct {
	url        string
	namespace  string
	key        string
	vcKey      string
	certChain  [][]byte
	httpClient *http.Client
}

// NewClient creates a Client.
//   - url       - HTTP base URL of the crypto-provider-service (CRYPTO_PROVIDER_URL)
//   - namespace - Vault transit mount path (CRYPTO_PROVIDER_NAMESPACE)
//   - key       - signing key name (CRYPTO_PROVIDER_KEY)
func NewClient(url, namespace, key string) *Client {
	vcKey := strings.TrimSpace(os.Getenv("CRYPTO_PROVIDER_VC_KEY"))
	return &Client{
		url:        strings.TrimRight(url, "/"),
		namespace:  strings.TrimSpace(namespace),
		key:        strings.TrimSpace(key),
		vcKey:      vcKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// SetCertificateChainFromPEMFile loads a PEM file containing one or more X.509 certificates.
func (c *Client) SetCertificateChainFromPEMFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read certificate chain file: %w", err)
	}
	return c.SetCertificateChainFromPEM(data)
}

// SetCertificateChainFromPEM parses a PEM bundle and stores the DER certificates for COSE x5chain.
func (c *Client) SetCertificateChainFromPEM(pemData []byte) error {
	var chain [][]byte
	remaining := pemData
	for {
		var block *pem.Block
		block, remaining = pem.Decode(remaining)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("parse certificate chain entry: %w", err)
		}
		chain = append(chain, cert.Raw)
	}
	if len(chain) == 0 {
		return fmt.Errorf("no certificates found in PEM chain")
	}
	c.certChain = chain
	return nil
}

// CertificateChain returns the configured certificate chain for C2PA COSE headers.
func (c *Client) CertificateChain(context.Context) ([][]byte, error) {
	if len(c.certChain) == 0 {
		return nil, fmt.Errorf("certificate chain not configured")
	}
	chain := make([][]byte, len(c.certChain))
	for i := range c.certChain {
		chain[i] = append([]byte(nil), c.certChain[i]...)
	}
	return chain, nil
}

// signRequest is the body for POST /v1/sign.
type signRequest struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Group     string `json:"group"`
	Data      string `json:"data"`
}

// signResult is the response from POST /v1/sign.
type signResult struct {
	Signature string `json:"signature"`
}

// Sign sends data to the crypto-provider-service for signing and returns the
// raw signature bytes. Used for COSE_Sign1 signatures in C2PA manifests
// (DCS-OR-C2PA-001).
func (c *Client) Sign(ctx context.Context, data []byte) ([]byte, error) {
	return c.signWithKey(ctx, c.key, data)
}

// CreateCredential adds an Ed25519Signature2020 proof to the unsigned VC.
// The proof is built locally from canonical JSON-LD and signed through the
// dedicated VC transit key via the crypto-provider service's /v1/sign API.
func (c *Client) CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	var vcDoc interface{}
	if err := json.Unmarshal(unsignedVC, &vcDoc); err != nil {
		return nil, fmt.Errorf("unmarshal unsigned VC: %w", err)
	}

	vcMap, ok := vcDoc.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unsigned VC must be a JSON object")
	}

	proof := map[string]interface{}{
		"@context":           []interface{}{"https://w3id.org/security/suites/ed25519-2020/v1"},
		"type":               "Ed25519Signature2020",
		"created":            time.Now().UTC().Format(time.RFC3339),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": "",
	}

	verificationMethod, err := verificationMethodFor(vcMap, c.vcKey)
	if err != nil {
		return nil, err
	}
	proof["verificationMethod"] = verificationMethod

	proofNQ, err := normalizeJSONLD(proof)
	if err != nil {
		return nil, fmt.Errorf("normalize proof options: %w", err)
	}
	docNQ, err := normalizeJSONLD(vcMap)
	if err != nil {
		return nil, fmt.Errorf("normalize VC document: %w", err)
	}

	proofHash := sha256.Sum256([]byte(proofNQ))
	docHash := sha256.Sum256([]byte(docNQ))
	toSign := append(append([]byte{}, proofHash[:]...), docHash[:]...)

	sig, err := c.signWithKey(ctx, c.vcKey, toSign)
	if err != nil {
		return nil, fmt.Errorf("sign proof payload: %w", err)
	}

	proof["proofValue"] = "z" + base58.Encode(sig)
	vcMap["proof"] = proof

	out, err := json.Marshal(vcMap)
	if err != nil {
		return nil, fmt.Errorf("marshal VC with local proof: %w", err)
	}
	return json.RawMessage(out), nil
}

func (c *Client) signWithKey(ctx context.Context, key string, data []byte) ([]byte, error) {
	if c.url == "" {
		return nil, fmt.Errorf("crypto provider URL is required")
	}
	if c.namespace == "" {
		return nil, fmt.Errorf("crypto provider namespace is required")
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("crypto provider signing key is required")
	}

	body, err := json.Marshal(signRequest{
		Namespace: c.namespace,
		Key:       key,
		Group:     "",
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
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

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

func normalizeJSONLD(doc interface{}) (string, error) {
	proc := ld.NewJsonLdProcessor()
	opts := ld.NewJsonLdOptions("")
	opts.Algorithm = ld.AlgorithmURDNA2015
	opts.Format = "application/n-quads"
	norm, err := proc.Normalize(doc, opts)
	if err != nil {
		return "", err
	}
	return norm.(string), nil
}

func verificationMethodFor(vc map[string]interface{}, vcKey string) (string, error) {
	if strings.TrimSpace(vcKey) == "" {
		return "", fmt.Errorf("CRYPTO_PROVIDER_VC_KEY is required")
	}
	issuer, ok := vc["issuer"].(string)
	if ok && strings.TrimSpace(issuer) != "" {
		return strings.TrimSpace(issuer) + "#" + strings.TrimSpace(vcKey), nil
	}
	return "", fmt.Errorf("VC issuer is required to derive verificationMethod")
}
