package request

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	addr     string
	token    string
	mount    string
	key      string
	kid      string
	client   *http.Client
	cache    PublicKeyCache
	cacheTTL time.Duration
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
	jwk, err := s.getOrFetchTransitPublicJWK()
	if err != nil {
		return "", err
	}
	return signES256JWT(oauthAuthzReqJWTType, s.kid, claims, jwk, s.signSigningInput)
}

func (s *VaultTransitSigner) getOrFetchTransitPublicJWK() (map[string]string, error) {
	cacheKey := fmt.Sprintf("oid4vp:verifier:jar-signing-jwk:%s:%s", s.mount, s.key)
	ctx := context.Background()

	cached, found, err := s.cache.Get(ctx, cacheKey)
	if err != nil {
		return nil, fmt.Errorf("public key cache get: %w", err)
	}
	if found {
		var jwk map[string]string
		if err := json.Unmarshal([]byte(cached), &jwk); err != nil {
			return nil, fmt.Errorf("parse cached public key jwk: %w", err)
		}
		return jwk, nil
	}

	jwk, err := s.fetchTransitPublicJWK()
	if err != nil {
		return nil, err
	}

	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		return nil, fmt.Errorf("marshal public key jwk: %w", err)
	}
	if err := s.cache.Set(ctx, cacheKey, string(jwkJSON), s.cacheTTL); err != nil {
		return nil, fmt.Errorf("public key cache set: %w", err)
	}

	return jwk, nil
}

// SetPublicKeyCache sets the shared public key cache and TTL (default 10 minutes if TTL is <= 0).
// The cache is a required dependency of the signer: cmd/dcs wires it at startup and refuses
// to start without it.
func (s *VaultTransitSigner) SetPublicKeyCache(cache PublicKeyCache, ttl time.Duration) {
	s.cache = cache
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	s.cacheTTL = ttl
}

func (s *VaultTransitSigner) fetchTransitPublicJWK() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultVaultTransitSignTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/%s/keys/%s", s.addr, strings.Trim(s.mount, "/"), s.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build vault key request: %w", err)
	}
	req.Header.Set("X-Vault-Token", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault transit key read: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read vault key response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault transit key read returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Data struct {
			LatestVersion int `json:"latest_version"`
			Keys          map[string]struct {
				PublicKey string `json:"public_key"`
			} `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse vault key response: %w", err)
	}

	pubPEM, err := latestVaultPublicKey(parsed.Data.LatestVersion, parsed.Data.Keys)
	if err != nil {
		return nil, err
	}
	return ecPEMToJWK(pubPEM)
}

func latestVaultPublicKey(latestVersion int, keys map[string]struct {
	PublicKey string `json:"public_key"`
}) (string, error) {
	if len(keys) == 0 {
		return "", fmt.Errorf("vault transit key response missing versions")
	}

	if latestVersion > 0 {
		entry, ok := keys[strconv.Itoa(latestVersion)]
		if ok && strings.TrimSpace(entry.PublicKey) != "" {
			return entry.PublicKey, nil
		}
	}

	bestVersion := -1
	bestKey := ""
	for rawVersion, entry := range keys {
		version, err := strconv.Atoi(rawVersion)
		if err != nil {
			continue
		}
		if version > bestVersion && strings.TrimSpace(entry.PublicKey) != "" {
			bestVersion = version
			bestKey = entry.PublicKey
		}
	}

	if bestVersion < 0 {
		return "", fmt.Errorf("vault transit key response has no usable public_key")
	}
	return bestKey, nil
}

func ecPEMToJWK(publicKeyPEM string) (map[string]string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(publicKeyPEM)))
	if block == nil {
		return nil, fmt.Errorf("vault public key is not valid PEM")
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse vault public key: %w", err)
	}

	pk, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("vault public key is not ECDSA")
	}
	if pk.Curve != elliptic.P256() {
		return nil, fmt.Errorf("vault public key curve is not P-256")
	}

	coordSize := (pk.Curve.Params().BitSize + 7) / 8
	return map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"x":   coordinateBase64URL(pk.X, coordSize),
		"y":   coordinateBase64URL(pk.Y, coordSize),
	}, nil
}

func coordinateBase64URL(value interface{ Bytes() []byte }, size int) string {
	raw := value.Bytes()
	if len(raw) < size {
		padded := make([]byte, size)
		copy(padded[size-len(raw):], raw)
		raw = padded
	}
	return base64.RawURLEncoding.EncodeToString(raw)
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
