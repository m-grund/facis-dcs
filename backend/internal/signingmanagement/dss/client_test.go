package dss

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidatePDFParsesIndication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/rest/validation/validateSignature" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		signed, _ := body["signedDocument"].(map[string]any)
		if signed["bytes"] == "" || signed["name"] != "contract.pdf" {
			t.Fatalf("expected the signed document in the request, got: %v", body)
		}
		// A trimmed WSReportsDTO in the DSS demo webapp's shape.
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {"Indication": "INDETERMINATE", "SubIndication": "NO_CERTIFICATE_CHAIN_FOUND"}}
				]
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF-1.7 fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if report.Indication != "INDETERMINATE" || report.SubIndication != "NO_CERTIFICATE_CHAIN_FOUND" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidatePDFHardFailsWhenUnreachable(t *testing.T) {
	// A configured DSS that cannot be reached is an error, never a silent skip.
	if _, err := New("http://127.0.0.1:1").ValidatePDF(context.Background(), []byte("x"), "x.pdf"); err == nil {
		t.Fatal("expected an error for an unreachable DSS")
	}
}

func TestValidatePDFRejectsReportWithoutIndication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"simpleReport": {}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL).ValidatePDF(context.Background(), []byte("x"), "x.pdf"); err == nil {
		t.Fatal("expected an error for a response without an Indication")
	}
}
