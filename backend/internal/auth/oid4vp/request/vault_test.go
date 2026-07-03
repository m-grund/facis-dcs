package request

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signWithECDSA signs data using an ECDSA private key in JWS format.
func signWithECDSA(privKey *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	if err != nil {
		return nil, err
	}
	// JWS format: r and s concatenated (each 32 bytes for P-256)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	rPadded := make([]byte, 32)
	sPadded := make([]byte, 32)
	copy(rPadded[32-len(rBytes):], rBytes)
	copy(sPadded[32-len(sBytes):], sBytes)
	return append(rPadded, sPadded...), nil
}

// fakePublicKeyCache is an in-memory cache implementation for testing.
type fakePublicKeyCache struct {
	data  map[string]string
	getEC map[string]error // Allows injecting errors on specific keys
}

func newFakeCache() *fakePublicKeyCache {
	return &fakePublicKeyCache{
		data:  make(map[string]string),
		getEC: make(map[string]error),
	}
}

func (c *fakePublicKeyCache) Get(ctx context.Context, key string) (string, bool, error) {
	if err, ok := c.getEC[key]; ok {
		return "", false, err
	}
	v, found := c.data[key]
	return v, found, nil
}

func (c *fakePublicKeyCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	c.data[key] = value
	return nil
}

func TestVaultTransitSignerWithCache(t *testing.T) {
	// Generate a test P-256 key pair.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	// Encode as PEM.
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Count calls to the Vault keys endpoint.
	keysFetchCount := 0

	// Create a test Vault server.
	vaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/test-key" && r.Method == http.MethodGet {
			keysFetchCount++
			// Return the public key in Vault format.
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"latest_version": 1,
					"keys": map[string]interface{}{
						"1": map[string]interface{}{
							"public_key": string(pubKeyPEM),
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if r.URL.Path == "/v1/transit/sign/test-key" && r.Method == http.MethodPost {
			// Decode request.
			var req struct {
				Input string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			// Decode input from base64.
			inputBytes, err := base64.StdEncoding.DecodeString(req.Input)
			if err != nil {
				inputBytes = []byte(req.Input)
			}

			// Sign with the private key.
			sig, err := signWithECDSA(privKey, inputBytes)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// Format as Vault signature.
			vaultSig := "vault:v1:" + base64.RawURLEncoding.EncodeToString(sig)

			response := map[string]interface{}{
				"data": map[string]interface{}{
					"signature": vaultSig,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer vaultServer.Close()

	// Create signer with cache.
	signer, err := NewVaultTransitSigner(vaultServer.URL, "test-token", "transit", "test-key")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	cache := newFakeCache()
	signer.SetPublicKeyCache(cache, 10*time.Minute)

	// First call: should fetch from Vault and store in cache.
	claims1 := jwt.MapClaims{
		"iss": "https://verifier",
		"sub": "wallet",
		"aud": "https://wallet",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token1, err := signer.SignAuthorizationRequestJWT(claims1)
	if err != nil {
		t.Fatalf("first sign: %v", err)
	}
	if token1 == "" {
		t.Fatal("first sign returned empty token")
	}
	if keysFetchCount != 1 {
		t.Fatalf("expected 1 key fetch, got %d", keysFetchCount)
	}

	// Verify the JWK was cached.
	cacheKey := "oid4vp:verifier:jar-signing-jwk:transit:test-key"
	cachedValue, found, _ := cache.Get(context.Background(), cacheKey)
	if !found {
		t.Fatal("JWK not found in cache after first sign")
	}
	if cachedValue == "" {
		t.Fatal("cached JWK is empty")
	}

	// Second call: should use cache, not fetch from Vault.
	claims2 := jwt.MapClaims{
		"iss": "https://verifier",
		"sub": "wallet2",
		"aud": "https://wallet2",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token2, err := signer.SignAuthorizationRequestJWT(claims2)
	if err != nil {
		t.Fatalf("second sign: %v", err)
	}
	if token2 == "" {
		t.Fatal("second sign returned empty token")
	}
	if keysFetchCount != 1 {
		t.Fatalf("expected 1 key fetch after second sign, got %d", keysFetchCount)
	}

	// The cache is a required dependency: a cache error fails signing instead of
	// falling back to Vault.
	signer2, err := NewVaultTransitSigner(vaultServer.URL, "test-token", "transit", "test-key")
	if err != nil {
		t.Fatalf("create second signer: %v", err)
	}

	errCache := newFakeCache()
	errCache.getEC["oid4vp:verifier:jar-signing-jwk:transit:test-key"] = fmt.Errorf("cache error")
	signer2.SetPublicKeyCache(errCache, 10*time.Minute)

	keysFetchCount = 0 // Reset counter
	claims3 := jwt.MapClaims{
		"iss": "https://verifier",
		"sub": "wallet3",
		"aud": "https://wallet3",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	_, err = signer2.SignAuthorizationRequestJWT(claims3)
	if err == nil {
		t.Fatal("expected signing to fail on cache error")
	}
	if keysFetchCount != 0 {
		t.Fatalf("expected no key fetch on cache error, got %d", keysFetchCount)
	}
}

func TestECPEMToJWKCoordinateSize(t *testing.T) {
	// Generate a P-256 key.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	// Encode as PEM.
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Convert to JWK.
	jwk, err := ecPEMToJWK(string(pubKeyPEM))
	if err != nil {
		t.Fatalf("ecPEMToJWK: %v", err)
	}

	// Check that the JWK has the expected format.
	if jwk["kty"] != "EC" {
		t.Fatalf("expected kty=EC, got %q", jwk["kty"])
	}
	if jwk["crv"] != "P-256" {
		t.Fatalf("expected crv=P-256, got %q", jwk["crv"])
	}

	// For P-256, x and y should be 43 characters (32 bytes base64url-encoded).
	expectedLen := 43
	if len(jwk["x"]) != expectedLen {
		t.Fatalf("expected x length %d, got %d: %q", expectedLen, len(jwk["x"]), jwk["x"])
	}
	if len(jwk["y"]) != expectedLen {
		t.Fatalf("expected y length %d, got %d: %q", expectedLen, len(jwk["y"]), jwk["y"])
	}
}
