package compiler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransitSigner_Sign_CallsHTTPEndpoint(t *testing.T) {
	wantSig := []byte("fake-ed25519-signature-bytes")
	sigB64 := base64.StdEncoding.EncodeToString(wantSig)

	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sign" {
			t.Errorf("unexpected path %q, want /v1/sign", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"signature": sigB64})
	}))
	defer srv.Close()

	signer := newTransitSigner(srv.URL, "test-ns", "test-key")
	data := []byte("data-to-sign")
	got, err := signer.Sign(context.Background(), data)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if !bytes.Equal(got, wantSig) {
		t.Errorf("Sign() = %x, want %x", got, wantSig)
	}

	// Verify request body fields.
	if gotBody["namespace"] != "test-ns" {
		t.Errorf("request namespace = %q, want test-ns", gotBody["namespace"])
	}
	if gotBody["key"] != "test-key" {
		t.Errorf("request key = %q, want test-key", gotBody["key"])
	}
	wantDataB64 := base64.StdEncoding.EncodeToString(data)
	if gotBody["data"] != wantDataB64 {
		t.Errorf("request data = %q, want %q", gotBody["data"], wantDataB64)
	}
}

func TestTransitSigner_Sign_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	signer := newTransitSigner(srv.URL, "ns", "key")
	_, err := signer.Sign(context.Background(), []byte("data"))
	if err == nil {
		t.Fatal("Sign() should return error on non-200 status")
	}
}

func TestLoadSigningMaterialFromEnv_TransitProvider(t *testing.T) {
	_, chainPEM, _ := mustCreateTestCertChainAndKey(t)
	env := map[string]string{
		envCryptoProviderURL:       "http://crypto-provider.example.com",
		envCryptoProviderNamespace: "facis",
		envCryptoProviderKey:       "dcs-signer",
		envX5ChainPEM:              chainPEM,
	}

	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, nil)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
	ts, ok := material.signer.(*transitSigner)
	if !ok {
		t.Fatalf("signer is %T, want *transitSigner", material.signer)
	}
	if ts.url != "http://crypto-provider.example.com" {
		t.Errorf("transitSigner.url = %q, want http://crypto-provider.example.com", ts.url)
	}
	if ts.namespace != "facis" {
		t.Errorf("transitSigner.namespace = %q, want facis", ts.namespace)
	}
	if ts.key != "dcs-signer" {
		t.Errorf("transitSigner.key = %q, want dcs-signer", ts.key)
	}
}
