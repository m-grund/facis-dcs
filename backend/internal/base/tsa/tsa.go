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

type APIClient struct {
	url    string
	client *http.Client
}

func NewClient(url string) (*APIClient, error) {
	return &APIClient{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (c *APIClient) Timestamp(ctx context.Context, data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal data: %w", err)
	}

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
	hash := sha256.Sum256(jsonData)

	rawBytes, err := base64.StdEncoding.DecodeString(tsrBase64)
	if err != nil {
		return false, fmt.Errorf("decode TSR: %w", err)
	}

	var ts *timestamp.Timestamp
	ts, err = timestamp.ParseResponse(rawBytes)
	if err != nil {
		ts, err = timestamp.Parse(rawBytes)
		if err != nil {
			return false, fmt.Errorf("parse TSR: %w", err)
		}
	}

	if !bytes.Equal(ts.HashedMessage, hash[:]) {
		return false, fmt.Errorf("TSR hash mismatch")
	}

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
