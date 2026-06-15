// Package tsa provides a client for obtaining RFC 3161 timestamps and a
// verifier that checks timestamp responses (TSRs) against a trusted TSA
// certificate.
//
// This package does not communicate with an external TSA directly. All
// timestamp requests are forwarded to ORCE, which handles the actual RFC 3161
// flow with the upstream TSA provider. This package only verifies the returned
// TSR using the embedded CA certificate.
//
// # Switching TSA providers
//
// To use a different TSA, two changes are required:
//
//  1. Update the TSA flow in ORCE to point to the new provider.
//
//  2. Replace certs/tsa.crt with the CA certificate (PEM) of the new provider.
//     The file is embedded at compile time (see [tsaCertPEM]), so a rebuild of
//     this service is required after the certificate is replaced.
package tsa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
)

//go:embed certs/tsa.crt
var tsaCertPEM []byte

var embeddedTSACert = func() *x509.Certificate {
	block, _ := pem.Decode(tsaCertPEM)
	if block == nil {
		panic("tsa: failed to decode certs/tsa.crt")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("tsa: failed to parse certs/tsa.crt: " + err.Error())
	}
	return cert
}()

// APIClient sends timestamp requests to an RFC 3161 TSA HTTP endpoint.
type APIClient struct {
	// url is the base URL of the TSA endpoint. The hex-encoded hash of the
	// payload is appended to this URL for each request.
	url    string
	client *http.Client
}

// NewClient creates a new [APIClient] for the given TSA endpoint URL.
// url must end with a path separator so that the hash can be appended directly
func NewClient(url string) (*APIClient, error) {
	return &APIClient{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Timestamp JSON-marshals data, computes its SHA-256 hash, and requests a
// timestamp token from the TSA. The returned string is the raw TSR
// (timestamp response) encoded in base64. Store this value alongside the
// original data and pass it to [Verify] later to prove the data existed at
// the returned point in time.
func (c *APIClient) Timestamp(ctx context.Context, data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal data: %w", err)
	}

	// Compute SHA-256 of the JSON representation and append it to the base URL.
	hash := sha256.Sum256(jsonData)
	hashString := hex.EncodeToString(hash[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+hashString, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	// Return the raw TSR bytes as base64 so they can be stored as a plain string.
	return base64.StdEncoding.EncodeToString(body), nil
}

// Verify checks that a base64-encoded TSR covers data and was signed by the
// embedded TSA certificate (certs/tsa.crt). It JSON-marshals data, computes
// its SHA-256 hash, and compares it against the hash inside the TSR.
// Returns (true, nil) on success, (false, err) on any failure.
func Verify(tsrBase64 string, data any) (bool, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false, fmt.Errorf("marshal data: %w", err)
	}
	// Recompute the same hash that was sent to the TSA during Timestamp().
	hash := sha256.Sum256(jsonData)

	rawBytes, err := base64.StdEncoding.DecodeString(tsrBase64)
	if err != nil {
		return false, fmt.Errorf("decode TSR: %w", err)
	}

	// Try parsing as a full HTTP timestamp response first; fall back to the
	// bare DER-encoded TimeStampToken format used by some providers.
	var ts *timestamp.Timestamp
	ts, err = timestamp.ParseResponse(rawBytes)
	if err != nil {
		ts, err = timestamp.Parse(rawBytes)
		if err != nil {
			return false, fmt.Errorf("parse TSR: %w", err)
		}
	}

	// Ensure the TSR was issued for exactly the data we supplied.
	if !bytes.Equal(ts.HashedMessage, hash[:]) {
		return false, fmt.Errorf("TSR hash mismatch")
	}

	// Verify the TSA's cryptographic signature using the embedded certificate.
	// embeddedTSACert was parsed from certs/tsa.crt at package init time.
	pool := x509.NewCertPool()
	pool.AddCert(embeddedTSACert)
	p7, err := pkcs7.Parse(ts.RawToken)
	if err != nil {
		return false, fmt.Errorf("parse TSR token: %w", err)
	}
	if err := p7.VerifyWithChain(pool); err != nil {
		return false, fmt.Errorf("TSA certificate verification: %w", err)
	}

	return true, nil
}
