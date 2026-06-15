package tsa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/digitorus/timestamp"
)

type APIClient struct {
	url    string
	client *http.Client
}

type Receipt struct {
	Token          string    `json:"token"`
	TokenEncoding  string    `json:"token_encoding"`
	HashAlgorithm  string    `json:"hash_algorithm"`
	MessageImprint string    `json:"message_imprint"`
	GeneratedAt    time.Time `json:"generated_at"`
	Policy         string    `json:"policy,omitempty"`
	SerialNumber   string    `json:"serial_number,omitempty"`
}

func NewClient(url string) (*APIClient, error) {
	return &APIClient{
		url: strings.TrimSpace(url),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (c *APIClient) Enabled() bool {
	return c != nil && c.url != ""
}

func (c *APIClient) Timestamp(ctx context.Context, data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal data: %w", err)
	}

	receipt, err := c.TimestampBytes(ctx, jsonData)
	if err != nil {
		return "", err
	}
	return receipt.Token, nil
}

func (c *APIClient) TimestampBytes(ctx context.Context, data []byte) (*Receipt, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("TSA URL is empty")
	}
	token, ts, err := requestTimestampToken(ctx, c.client, c.url, data)
	if err != nil {
		return nil, err
	}
	return receiptFromTimestamp(token, ts), nil
}

func RequestTimestamp(ctx context.Context, tsaURL string, data []byte) ([]byte, error) {
	token, _, err := requestTimestampToken(ctx, &http.Client{Timeout: 10 * time.Second}, tsaURL, data)
	return token, err
}

func VerifyReceipt(receipt Receipt, data []byte) (*timestamp.Timestamp, error) {
	if strings.TrimSpace(receipt.Token) == "" {
		return nil, fmt.Errorf("TSA token is empty")
	}
	if receipt.TokenEncoding != "" && receipt.TokenEncoding != "base64" {
		return nil, fmt.Errorf("unsupported TSA token encoding %q", receipt.TokenEncoding)
	}
	token, err := base64.StdEncoding.DecodeString(receipt.Token)
	if err != nil {
		return nil, fmt.Errorf("decode TSA token: %w", err)
	}
	ts, err := timestamp.Parse(token)
	if err != nil {
		return nil, fmt.Errorf("parse TSA token: %w", err)
	}
	if err := verifyTimestampForData(ts, data); err != nil {
		return nil, err
	}
	if receipt.MessageImprint != "" && receipt.MessageImprint != hex.EncodeToString(ts.HashedMessage) {
		return nil, fmt.Errorf("TSA receipt message imprint mismatch")
	}
	return ts, nil
}

func requestTimestampToken(ctx context.Context, httpClient *http.Client, tsaURL string, data []byte) ([]byte, *timestamp.Timestamp, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	hash := sha256.Sum256(data)
	hashString := hex.EncodeToString(hash[:])

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, timestampURL(tsaURL, hashString), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create TSA HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "text/plain")
	httpReq.Header.Set("Accept", "application/timestamp-reply")

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("call TSA endpoint: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close body")
		}
	}(httpResp.Body)

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read TSA response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected TSA status %d: %s", httpResp.StatusCode, string(body))
	}

	ts, err := timestamp.Parse(body)
	if err != nil {
		tsResp, respErr := timestamp.ParseResponse(body)
		if respErr != nil {
			return nil, nil, fmt.Errorf("parse TSA token: %w", err)
		}
		ts = tsResp
	}
	if err := verifyTimestampForData(ts, data); err != nil {
		return nil, nil, err
	}
	if len(ts.RawToken) == 0 {
		return nil, nil, fmt.Errorf("TSA response token is empty")
	}

	return ts.RawToken, ts, nil
}

func timestampURL(baseURL, hash string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + hash
	}
	return baseURL + "/" + hash
}

func verifyTimestampForData(ts *timestamp.Timestamp, data []byte) error {
	if ts == nil {
		return fmt.Errorf("TSA timestamp is empty")
	}
	expectedHash := sha256.Sum256(data)
	if !bytes.Equal(ts.HashedMessage, expectedHash[:]) {
		return fmt.Errorf("TSA hashed message mismatch")
	}
	return nil
}

func receiptFromTimestamp(token []byte, ts *timestamp.Timestamp) *Receipt {
	receipt := &Receipt{
		Token:          base64.StdEncoding.EncodeToString(token),
		TokenEncoding:  "base64",
		HashAlgorithm:  "SHA-256",
		MessageImprint: hex.EncodeToString(ts.HashedMessage),
		GeneratedAt:    ts.Time.UTC(),
	}
	if len(ts.Policy) > 0 {
		receipt.Policy = ts.Policy.String()
	}
	if ts.SerialNumber != nil {
		receipt.SerialNumber = ts.SerialNumber.String()
	}
	return receipt
}
