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

// bearerTokenCtxKey carries the JWT the DCS backend forwarded with the render
// request. pdf-core presents it when it calls the backend's internal C2PA
// signing endpoint (DCS-IR-HI-01): pdf-core holds no key material and delegates
// every signature to the backend's PKCS#11 token.
type bearerTokenCtxKey struct{}

// WithBearerToken returns a context carrying the caller's JWT for the C2PA
// signing callback.
func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenCtxKey{}, token)
}

func bearerTokenFromContext(ctx context.Context) string {
	if tok, ok := ctx.Value(bearerTokenCtxKey{}).(string); ok {
		return tok
	}
	return ""
}

// httpCallbackSigner signs COSE Sig_structure bytes by delegating to the DCS
// backend's authenticated internal endpoint (POST /internal/c2pa/sign). The
// endpoint signs with the PKCS#11 dcs-c2pa key and returns the raw 64-byte r||s
// ES256 signature that COSE_Sign1 embeds directly.
type httpCallbackSigner struct {
	endpoint string
	client   *http.Client
}

func newHTTPCallbackSigner(endpoint string) *httpCallbackSigner {
	return &httpCallbackSigner{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{},
	}
}

func (s *httpCallbackSigner) Sign(ctx context.Context, data []byte) ([]byte, error) {
	body, err := json.Marshal(map[string]string{
		"sig_structure": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal c2pa sign request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create c2pa sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The backend endpoint enforces JWTAuth; forward the caller's token when the
	// render request carried one. When absent the endpoint answers 401 and Sign
	// fails with a clear status error below.
	if token := bearerTokenFromContext(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call c2pa signing endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("c2pa signing endpoint returned status %d", resp.StatusCode)
	}
	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode c2pa sign response: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode c2pa signature base64: %w", err)
	}
	if len(sig) != 64 {
		return nil, fmt.Errorf("c2pa signing endpoint returned %d-byte signature, want 64 (ES256 r||s)", len(sig))
	}
	return sig, nil
}
