package pdfcore_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

const testVersion = "1.0.1"

// testSign is the in-process dcs-c2pa stand-in: it returns a fixed 64-byte ES256
// r||s for any Sig_structure.
func testSign(_ []byte) ([]byte, error) {
	sig := make([]byte, 64)
	for i := range sig {
		sig[i] = byte(i)
	}
	return sig, nil
}

func newClient(url string) *pdfcore.Client { return pdfcore.New(url, testSign) }

func stubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// writePrepared responds like pdf-core's prepare step: a JSON envelope carrying a
// PDF and one Sig_structure to sign.
func writePrepared(w http.ResponseWriter, preparedPDF []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-PDF-Core-Version", testVersion)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"pdf_base64":          base64.StdEncoding.EncodeToString(preparedPDF),
		"c2pa_sig_structures": []string{base64.StdEncoding.EncodeToString([]byte("sig-structure"))},
	})
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

	c := newClient(srv.URL)
	v, err := c.Version(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testVersion, v)
}

// TestClientDownload verifies Download posts JSON-LD to /download (prepare), signs
// the returned Sig_structure, posts it to /c2pa/embed, and returns the embedded
// PDF plus the renderer version from the prepare header.
func TestClientDownload(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 embedded")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render":
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/ld+json", r.Header.Get("Content-Type"))
			body, _ := io.ReadAll(r.Body)
			assert.Equal(t, `{"@context":"test"}`, string(body))
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			var req struct {
				C2PASignatures []string `json:"c2pa_signatures"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Len(t, req.C2PASignatures, 1, "one signature per Sig_structure")
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(fakePDF)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	c := newClient(srv.URL)
	pdf, ver, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdate verifies Update sends a multipart prepare request then embeds.
func TestClientUpdate(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 updated")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.NotEmpty(t, r.FormValue("pdf"), "pdf field must be present")
			assert.NotEmpty(t, r.FormValue("payload"), "payload field must be present")
			assert.Empty(t, r.FormValue("vc"))
			assert.Empty(t, r.FormValue("manifest_url"))
			writePrepared(w, []byte("%PDF prepared update"))
		case "/c2pa/embed":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(fakePDF)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	c := newClient(srv.URL)
	pdf, ver, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, "")
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdateWithManifestURL verifies Update sends a "manifest_url" field.
func TestClientUpdateWithManifestURL(t *testing.T) {
	const manifestURL = "https://dcs.example/c2pa/manifest/did:example:contract-1"
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.Equal(t, manifestURL, r.FormValue("manifest_url"), "manifest_url field must match supplied URL")
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			_, _ = w.Write([]byte("%PDF embedded"))
		}
	})

	c := newClient(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, manifestURL)
	require.NoError(t, err)
}

// TestClientUpdateWithVC verifies Update sends a "vc" multipart field.
func TestClientUpdateWithVC(t *testing.T) {
	vcBytes := []byte(`{"type":["VerifiableCredential"]}`)
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.Equal(t, string(vcBytes), r.FormValue("vc"), "vc field must match supplied bytes")
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			_, _ = w.Write([]byte("%PDF embedded"))
		}
	})

	c := newClient(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), vcBytes, "")
	require.NoError(t, err)
}

// TestClientHTTPErrorPropagated verifies that a non-2xx prepare response is
// returned as an error (hard-fail — no silent fallbacks).
func TestClientHTTPErrorPropagated(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"name":"internal_error","message":"boom"}`, http.StatusInternalServerError)
	})

	c := newClient(srv.URL)
	_, _, err := c.Download(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
