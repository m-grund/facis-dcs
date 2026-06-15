package tsa

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_Timestamp_Success(t *testing.T) {
	type input struct {
		Field string `json:"field"`
	}
	testData := input{Field: "test-value"}

	wantBody := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}

	jsonData, err := json.Marshal(testData)
	require.NoError(t, err)
	hash := sha256.Sum256(jsonData)
	wantHashHex := hex.EncodeToString(hash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/"+wantHashHex, r.URL.Path)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wantBody)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL + "/")
	require.NoError(t, err)

	result, err := client.Timestamp(context.Background(), testData)
	require.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString(wantBody), result)
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
	assert.Contains(t, err.Error(), "500")
}

func TestAPIClient_Timestamp_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client, err := NewClient(srv.URL + "/")
	require.NoError(t, err)

	_, err = client.Timestamp(context.Background(), map[string]string{"key": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do request")
}
