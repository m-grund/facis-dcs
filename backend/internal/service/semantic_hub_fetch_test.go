package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchSchemaFromURLFollowsRedirects(t *testing.T) {
	const body = "@prefix ex: <http://example.org/> . ex:s a ex:Shape ."
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer final.Close()
	// A w3id/purl-style redirect to the real document.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	got, err := fetchSchemaFromURL(context.Background(), redirector.URL)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestFetchSchemaFromURLRejectsNonHTTPScheme(t *testing.T) {
	_, err := fetchSchemaFromURL(context.Background(), "file:///etc/passwd")
	require.ErrorContains(t, err, "http or https")
}

func TestFetchSchemaFromURLRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := fetchSchemaFromURL(context.Background(), srv.URL)
	require.ErrorContains(t, err, "HTTP 404")
}
