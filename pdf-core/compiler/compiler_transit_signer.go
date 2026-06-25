package compiler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type transitSigner struct {
	url       string
	namespace string
	key       string
	client    *http.Client
}

func newTransitSigner(url, namespace, key string) *transitSigner {
	return &transitSigner{
		url:       strings.TrimRight(url, "/"),
		namespace: namespace,
		key:       key,
		client:    &http.Client{},
	}
}

func (s *transitSigner) Sign(ctx context.Context, data []byte) ([]byte, error) {
	body, err := json.Marshal(map[string]string{
		"namespace": s.namespace,
		"key":       s.key,
		"data":      base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sign request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+"/v1/sign", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call transit signer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transit signer returned status %d", resp.StatusCode)
	}
	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode sign response: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode signature base64: %w", err)
	}
	return sig, nil
}
