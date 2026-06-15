package tsa

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_Timestamp_Success(t *testing.T) {
	type input struct {
		Field string `json:"field"`
	}
	testData := input{Field: "test-value"}

	jsonData, err := json.Marshal(testData)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/timestamp-reply", r.Header.Get("Accept"))
		expectedHash := sha256.Sum256(jsonData)
		assert.Equal(t, hex.EncodeToString(expectedHash[:]), strings.TrimPrefix(r.URL.Path, "/"))

		cert, key := mustTSACert(t)
		ts := &timestamp.Timestamp{
			HashAlgorithm: crypto.SHA256,
			HashedMessage: expectedHash[:],
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(1),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
		}
		resp, err := ts.CreateResponseWithOpts(cert, key, crypto.SHA256)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Timestamp(context.Background(), testData)
	require.NoError(t, err)
	token, err := base64.StdEncoding.DecodeString(result)
	require.NoError(t, err)
	ts, err := timestamp.Parse(token)
	require.NoError(t, err)
	assert.NotEmpty(t, ts.HashedMessage)
	receipt, err := client.TimestampBytes(context.Background(), jsonData)
	require.NoError(t, err)
	assert.Equal(t, "base64", receipt.TokenEncoding)
	assert.NotEmpty(t, receipt.MessageImprint)
}

func TestAPIClient_Timestamp_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL + "/")
	require.NoError(t, err)

	_, err = client.Timestamp(context.Background(), map[string]string{"key": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected TSA status")
}

func TestAPIClient_Timestamp_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)

	_, err = client.Timestamp(context.Background(), map[string]string{"key": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call TSA endpoint")
}

func mustTSACert(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "test-tsa"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}
