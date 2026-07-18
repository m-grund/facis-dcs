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

func TestValidatePDFExtractsSignerIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {
						"Indication": "TOTAL-PASSED",
						"SignatureFormat": "PAdES-BASELINE-B",
						"SignedBy": "CN=DCS Signatory johndoe,O=Test",
						"SigningTime": "2026-07-18T10:00:00Z"
					}}
				]
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected TOTAL-PASSED, got %q", report.Indication)
	}
	if report.SignedBy != "CN=DCS Signatory johndoe,O=Test" {
		t.Fatalf("unexpected SignedBy: %q", report.SignedBy)
	}
	if report.SignatureFormat != "PAdES-BASELINE-B" || report.SigningTime == "" {
		t.Fatalf("unexpected format/time: %+v", report)
	}
}

func TestAssertValidAES(t *testing.T) {
	// A cryptographically sound signature with a signing certificate is a valid
	// AES. Identifying the signatory is the ceremony PID's job, not a certificate
	// subject match (eIDAS Art. 26 mandates no PID-to-cert binding).
	passed := &Report{Indication: "TOTAL-PASSED", SignedBy: "CN=Jane Doe, SURNAME=Doe, GIVENNAME=Jane"}
	if err := passed.AssertValidAES(); err != nil {
		t.Fatalf("expected a valid AES to be accepted: %v", err)
	}

	// AES: a non-qualified CA yields INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND
	// (a trust gap, not a crypto failure) and MUST still be accepted — qualified
	// trust is a QES property, not required for AES.
	nonQualified := &Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "CN=Jane Doe"}
	if err := nonQualified.AssertValidAES(); err != nil {
		t.Fatalf("expected a cryptographically-sound AES over a non-qualified CA to be accepted: %v", err)
	}

	failed := &Report{Indication: "TOTAL-FAILED", SubIndication: "HASH_FAILURE", SignedBy: "CN=x"}
	if err := failed.AssertValidAES(); err == nil {
		t.Fatal("expected a failed indication to be rejected")
	}

	// A crypto failure is rejected even when the top indication is INDETERMINATE.
	cryptoBroken := &Report{Indication: "INDETERMINATE", SubIndication: "SIG_CRYPTO_FAILURE", SignedBy: "CN=x"}
	if err := cryptoBroken.AssertValidAES(); err == nil {
		t.Fatal("expected a crypto failure to be rejected")
	}

	noCert := &Report{Indication: "TOTAL-PASSED"}
	if err := noCert.AssertValidAES(); err == nil {
		t.Fatal("expected rejection when no signing certificate is present")
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
