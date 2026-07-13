package sign

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetTSA_RequestsGETWithHashInPath verifies that GetTSA speaks the
// protocol the deployed ORCE TSA flow actually implements
// (deployment/helm/charts/orce/flows/tsa_orce_flow.json): a bodyless GET at
// {TSA.URL}/{sha256-hex(sign_content)}, not a POST of an ASN.1
// TimeStampReq to the bare TSA URL.
func TestGetTSA_RequestsGETWithHashInPath(t *testing.T) {
	signContent := []byte("dcs-pdf-core test sign content")
	wantHash := sha256.Sum256(signContent)
	wantHashHex := hex.EncodeToString(wantHash[:])
	wantPath := "/" + wantHashHex

	var gotMethod, gotPath string
	var gotBodyLen int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBodyLen = len(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-tsr-bytes"))
	}))
	defer server.Close()

	ctx := &SignContext{SignData: SignData{TSA: TSA{URL: server.URL}}}
	responseBody, err := ctx.GetTSA(signContent)
	if err != nil {
		t.Fatalf("GetTSA: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("GetTSA used HTTP method %q, want GET", gotMethod)
	}
	if gotPath != wantPath {
		t.Errorf("GetTSA requested path %q, want %q (sha256 hex of sign_content)", gotPath, wantPath)
	}
	if gotBodyLen != 0 {
		t.Errorf("GetTSA sent a %d-byte request body, want a bodyless GET", gotBodyLen)
	}
	if string(responseBody) != "fake-tsr-bytes" {
		t.Errorf("GetTSA returned %q, want the raw TSA response body unmodified", responseBody)
	}
}

// TestGetTSA_NonOKStatusIsAnError verifies that a non-2xx response from the
// TSA endpoint is surfaced as an error rather than silently returning the
// (error) response body as if it were a valid TSR.
func TestGetTSA_NonOKStatusIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream TSA unavailable"))
	}))
	defer server.Close()

	ctx := &SignContext{SignData: SignData{TSA: TSA{URL: server.URL}}}
	_, err := ctx.GetTSA([]byte("anything"))
	if err == nil {
		t.Fatal("GetTSA returned no error for a non-2xx TSA response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("GetTSA error %q does not mention the non-OK status code", err)
	}
}
