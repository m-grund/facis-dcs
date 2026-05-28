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

func TestCryptoProviderClient_Sign(t *testing.T) {
	wantSig := []byte{0x01, 0x02, 0x03, 0x04}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/sign", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-ns", r.Header.Get("x-namespace"))

		// Decode request body.
		var req signRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-ns", req.Namespace)
		assert.Equal(t, "test-key", req.Key)
		assert.NotEmpty(t, req.Data)

		// Respond with fixed signature.
		resp := signResult{Signature: base64.StdEncoding.EncodeToString(wantSig)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-ns", "test-key")
	sig, err := client.Sign(context.Background(), []byte("payload to sign"))
	require.NoError(t, err)
	assert.Equal(t, wantSig, sig)
}

func TestCryptoProviderClient_Sign_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "ns", "key")
	_, err := client.Sign(context.Background(), []byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestCryptoProviderClient_CreateCredential(t *testing.T) {
	signedVC := json.RawMessage(`{"@context":["https://www.w3.org/2018/credentials/v1"],"type":["VerifiableCredential"],"proof":{}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/credential", r.URL.Path)
		assert.Equal(t, "ldp_vc", r.Header.Get("x-format"))

		var req createCredentialRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.Credential)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(signedVC)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "ns", "key")
	unsignedVC := json.RawMessage(`{"type":["VerifiableCredential"]}`)
	result, err := client.CreateCredential(context.Background(), unsignedVC)
	require.NoError(t, err)
	assert.Equal(t, []byte(signedVC), []byte(result))
}
