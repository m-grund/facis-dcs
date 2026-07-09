package compiler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustTestX5ChainPEM returns a self-signed P-256 leaf certificate as an x5chain
// PEM, standing in for the dev CA leaf whose public key matches the dcs-c2pa
// token key in production.
func mustTestX5ChainPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS-PDF-CORE test signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(certPEM(der))
}

func TestLoadSigningMaterialFromEnv_EndpointAndInlineX5Chain(t *testing.T) {
	chainPEM := mustTestX5ChainPEM(t)
	env := map[string]string{
		envSigningEndpoint: "http://backend:8991/api/internal/c2pa/sign",
		envX5ChainPEM:      chainPEM,
	}
	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
	if _, ok := material.signer.(*httpCallbackSigner); !ok {
		t.Fatalf("signer type = %T, want *httpCallbackSigner", material.signer)
	}
}

func TestLoadSigningMaterialFromEnv_EndpointAndFileX5Chain(t *testing.T) {
	chainPEM := mustTestX5ChainPEM(t)
	dir := t.TempDir()
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(chainPath, []byte(chainPEM), 0o644); err != nil {
		t.Fatalf("write chain: %v", err)
	}
	env := map[string]string{
		envSigningEndpoint: "http://backend:8991/api/internal/c2pa/sign",
		envX5ChainPEMFile:  chainPath,
	}
	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
}

func TestLoadSigningMaterialFromEnv_MissingEndpoint(t *testing.T) {
	env := map[string]string{
		envX5ChainPEM: mustTestX5ChainPEM(t),
	}
	if _, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile); err == nil {
		t.Fatalf("expected error when %s is missing", envSigningEndpoint)
	}
}

func TestLoadSigningMaterialFromEnv_MissingX5Chain(t *testing.T) {
	env := map[string]string{
		envSigningEndpoint: "http://backend:8991/api/internal/c2pa/sign",
	}
	if _, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile); err == nil {
		t.Fatalf("expected error when x5chain is missing")
	}
}

// TestHTTPCallbackSigner_ForwardsTokenAndReturnsSignature proves the signer
// presents the context bearer token as an Authorization header and returns the
// 64-byte signature the endpoint produced.
func TestHTTPCallbackSigner_ForwardsTokenAndReturnsSignature(t *testing.T) {
	var gotAuth string
	var gotSigStructure string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			SigStructure string `json:"sig_structure"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotSigStructure = req.SigStructure
		sig := make([]byte, 64)
		for i := range sig {
			sig[i] = byte(i)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"signature": base64.StdEncoding.EncodeToString(sig)})
	}))
	defer srv.Close()

	signer := newHTTPCallbackSigner(srv.URL)
	ctx := WithBearerToken(context.Background(), "test-jwt-token")
	sig, err := signer.Sign(ctx, []byte("sig-structure-bytes"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("signature length = %d, want 64", len(sig))
	}
	if gotAuth != "Bearer test-jwt-token" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer test-jwt-token")
	}
	if want := base64.StdEncoding.EncodeToString([]byte("sig-structure-bytes")); gotSigStructure != want {
		t.Fatalf("sig_structure = %q, want %q", gotSigStructure, want)
	}
}

// TestHTTPCallbackSigner_RejectsWrongLength ensures a non-64-byte response is
// rejected: COSE ES256 requires exactly r||s (64 bytes).
func TestHTTPCallbackSigner_RejectsWrongLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"signature": base64.StdEncoding.EncodeToString([]byte("short"))})
	}))
	defer srv.Close()

	signer := newHTTPCallbackSigner(srv.URL)
	if _, err := signer.Sign(context.Background(), []byte("x")); err == nil {
		t.Fatalf("expected error for non-64-byte signature")
	}
}
