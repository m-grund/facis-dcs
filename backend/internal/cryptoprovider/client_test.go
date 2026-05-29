package cryptoprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testNamespace = "transit"
	testKey       = "dcs-signing-key"
)

func TestCryptoProviderClient_Sign(t *testing.T) {
	wantSig := []byte{0x01, 0x02, 0x03, 0x04}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/sign", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, testNamespace, r.Header.Get("x-namespace"))

		var req signRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, testNamespace, req.Namespace)
		assert.Equal(t, testKey, req.Key)
		assert.Equal(t, "", req.Group)
		assert.NotEmpty(t, req.Data)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(signResult{
			Signature: base64.StdEncoding.EncodeToString(wantSig),
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, testNamespace, testKey)
	sig, err := client.Sign(context.Background(), []byte("payload to sign"))
	require.NoError(t, err)
	assert.Equal(t, wantSig, sig)
}

func TestCryptoProviderClient_Sign_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, testNamespace, testKey)
	_, err := client.Sign(context.Background(), []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestCryptoProviderClient_CreateCredential(t *testing.T) {
	t.Setenv("CRYPTO_PROVIDER_VC_KEY", "dcs-vc-signing-key")

	wantSig := []byte{0x01, 0x02, 0x03, 0x04}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/sign", r.URL.Path)
		assert.Equal(t, testNamespace, r.Header.Get("x-namespace"))

		var req signRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, testNamespace, req.Namespace)
		assert.Equal(t, "dcs-vc-signing-key", req.Key)
		assert.Equal(t, "", req.Group)
		assert.NotEmpty(t, req.Data)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(signResult{Signature: base64.StdEncoding.EncodeToString(wantSig)})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, testNamespace, testKey)
	unsignedVC := json.RawMessage(`{
		"@context": ["https://www.w3.org/2018/credentials/v1", "https://w3id.org/security/suites/ed25519-2020/v1"],
		"type": ["VerifiableCredential"],
		"issuer": "did:web:example.org:issuer",
		"credentialSubject": {
			"id": "did:web:example.org:subject",
			"name": "Example"
		}
	}`)
	result, err := client.CreateCredential(context.Background(), unsignedVC)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &got))
	proof, ok := got["proof"].(map[string]interface{})
	require.True(t, ok, "expected proof field in response")
	assert.Equal(t, "Ed25519Signature2020", proof["type"])
	assert.Equal(t, "assertionMethod", proof["proofPurpose"])
	assert.True(t, strings.HasPrefix(proof["proofValue"].(string), "z"))
}
