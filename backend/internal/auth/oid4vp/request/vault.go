package request

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

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultVerifierTransitMount    = "transit"
	defaultVerifierTransitKey      = "dcs-oid4vp-verifier-signing"
	defaultVaultTransitSignTimeout = 15 * time.Second
	vaultTransitMarshalingJWS      = "jws"
)

// VaultTransitSigner signs authorization request JWTs via the Vault transit engine.
type VaultTransitSigner struct {
	addr   string
	token  string
	mount  string
	key    string
	kid    string
	client *http.Client
}

// NewVaultTransitSigner builds a signer for the given Vault transit mount/key.
func NewVaultTransitSigner(addr, token, mount, key string) (*VaultTransitSigner, error) {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if addr == "" {
		return nil, fmt.Errorf("VAULT_ADDR is required for OID4VP request signing")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN is required for OID4VP request signing")
	}

	mount = strings.TrimSpace(mount)
	if mount == "" {
		mount = defaultVerifierTransitMount
	}

	key = strings.TrimSpace(key)
	if key == "" {
		key = defaultVerifierTransitKey
	}

	return &VaultTransitSigner{
		addr:   addr,
		token:  token,
		mount:  mount,
		key:    key,
		kid:    key,
		client: &http.Client{Timeout: defaultVaultTransitSignTimeout},
	}, nil
}

// SignAuthorizationRequestJWT returns a compact oauth-authz-req+jwt signed by Vault transit.
func (s *VaultTransitSigner) SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error) {
	if s == nil {
		return "", fmt.Errorf("vault transit signer is not configured")
	}
	return signES256JWT(oauthAuthzReqJWTType, s.kid, claims, s.signSigningInput)
}

func (s *VaultTransitSigner) signSigningInput(signingInput string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultVaultTransitSignTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/%s/sign/%s", s.addr, strings.Trim(s.mount, "/"), s.key)
	body, err := json.Marshal(map[string]string{
		"input":                base64.StdEncoding.EncodeToString([]byte(signingInput)),
		"marshaling_algorithm": vaultTransitMarshalingJWS,
	})

	if err != nil {
		return nil, fmt.Errorf("marshal vault sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build vault sign request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", s.token)

	resp, err := s.client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("vault transit sign: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read vault sign response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault transit sign returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Data struct {
			Signature string `json:"signature"`
		} `json:"data"`
	}

	err = json.Unmarshal(respBody, &parsed)

	if err != nil {
		return nil, fmt.Errorf("parse vault sign response: %w", err)
	}

	return decodeVaultTransitSignature(parsed.Data.Signature)
}

func decodeVaultTransitSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil, fmt.Errorf("vault signature is empty")
	}

	const prefix = "vault:v1:"
	if !strings.HasPrefix(value, prefix) {
		return nil, fmt.Errorf("unsupported vault signature format")
	}

	payload := strings.TrimPrefix(value, prefix)
	raw, err := base64.RawURLEncoding.DecodeString(payload)

	if err != nil {
		return nil, fmt.Errorf("decode vault jws signature: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("vault signature is empty after decode")
	}
	return raw, nil
}
