package cryptoprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultTransitSignHandler returns an httptest.Server that mimics the Vault
// transit sign endpoint (POST /v1/{mount}/sign/{key}).
func vaultTransitSignHandler(t *testing.T, mount, key string, sigBytes []byte) *httptest.Server {
	t.Helper()
	wantPath := "/v1/" + mount + "/sign/" + key
	sig := "vault:v1:" + base64.StdEncoding.EncodeToString(sigBytes)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, wantPath, r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

		var req vaultSignRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.Input)
		assert.Equal(t, "raw", req.MarshalingAlgorithm)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSignResponse{
			Data: struct {
				Signature string `json:"signature"`
			}{Signature: sig},
		})
	}))
}

func TestCryptoProviderClient_Sign(t *testing.T) {
	wantSig := []byte{0x01, 0x02, 0x03, 0x04}
	srv := vaultTransitSignHandler(t, "transit", "dcs-signing-key", wantSig)
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", "transit", "dcs-signing-key")
	sig, err := client.Sign(context.Background(), []byte("payload to sign"))
	require.NoError(t, err)
	assert.Equal(t, wantSig, sig)
}

func TestCryptoProviderClient_Sign_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", "transit", "key")
	_, err := client.Sign(context.Background(), []byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestCryptoProviderClient_Sign_VaultErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vaultSignResponse{
			Errors: []string{"permission denied"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad-token", "transit", "key")
	_, err := client.Sign(context.Background(), []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestCryptoProviderClient_CreateCredential(t *testing.T) {
	wantSig := []byte{0xAA, 0xBB, 0xCC}
	srv := vaultTransitSignHandler(t, "transit", "dcs-signing-key", wantSig)
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", "transit", "dcs-signing-key")
	unsignedVC := json.RawMessage(`{"type":["VerifiableCredential"]}`)
	result, err := client.CreateCredential(context.Background(), unsignedVC)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &got))
	// The VC must have a proof appended.
	proof, ok := got["proof"].(map[string]interface{})
	require.True(t, ok, "expected proof field")
	assert.Equal(t, "Ed25519Signature2020", proof["type"])
	assert.NotEmpty(t, proof["jws"])
}
