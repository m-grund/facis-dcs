package pdfcore_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

const testVersion = "1.0.1"

func stubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// TestClientVersion verifies that Version calls GET /version and parses the
// version string from the JSON response body.
func TestClientVersion(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/version", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"`+testVersion+`"}`)
	})

	c := pdfcore.New(srv.URL)
	v, err := c.Version(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testVersion, v)
}

// TestClientDownload verifies that Download posts JSON-LD as application/ld+json
// and returns the PDF bytes together with the renderer version from the header.
func TestClientDownload(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 fake")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/download", r.URL.Path)
		assert.Equal(t, "application/ld+json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, `{"@context":"test"}`, string(body))
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("X-PDF-Core-Version", testVersion)
		_, _ = w.Write(fakePDF)
	})

	c := pdfcore.New(srv.URL)
	pdf, ver, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdate verifies that Update sends a multipart request with "pdf"
// and "payload" parts and returns the PDF bytes with renderer version.
func TestClientUpdate(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 updated")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/update", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(8<<20))
		assert.NotEmpty(t, r.FormValue("pdf"), "pdf field must be present")
		assert.NotEmpty(t, r.FormValue("payload"), "payload field must be present")
		// no vc field in this call
		assert.Empty(t, r.FormValue("vc"))
		// no manifest_url in this call
		assert.Empty(t, r.FormValue("manifest_url"))
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("X-PDF-Core-Version", testVersion)
		_, _ = w.Write(fakePDF)
	})

	c := pdfcore.New(srv.URL)
	pdf, ver, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, "")
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdateWithManifestURL verifies that Update sends a "manifest_url"
// multipart field when a remote manifest URL is supplied (DCS-OR-C2PA-008 AC3).
func TestClientUpdateWithManifestURL(t *testing.T) {
	const manifestURL = "https://dcs.example/c2pa/manifest/did:example:contract-1"
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(8<<20))
		assert.Equal(t, manifestURL, r.FormValue("manifest_url"), "manifest_url field must match supplied URL")
		w.Header().Set("X-PDF-Core-Version", testVersion)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF updated+manifesturl"))
	})

	c := pdfcore.New(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, manifestURL)
	require.NoError(t, err)
}

// TestClientUpdateWithVC verifies that Update sends a "vc" multipart field
// when vcBytes is non-nil.
func TestClientUpdateWithVC(t *testing.T) {
	vcBytes := []byte(`{"type":["VerifiableCredential"]}`)
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(8<<20))
		assert.Equal(t, string(vcBytes), r.FormValue("vc"), "vc field must match supplied bytes")
		w.Header().Set("X-PDF-Core-Version", testVersion)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF updated+vc"))
	})

	c := pdfcore.New(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), vcBytes, "")
	require.NoError(t, err)
}

// TestClientHTTPErrorPropagated verifies that a non-2xx response from pdf-core
// is returned as an error (hard-fail — no silent fallbacks).
func TestClientHTTPErrorPropagated(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"name":"internal_error","message":"boom"}`, http.StatusInternalServerError)
	})

	c := pdfcore.New(srv.URL)
	_, _, err := c.Download(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
