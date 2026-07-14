package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const pAdESVerifyE2EPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:pades-verify-e2e",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "PAdES Verify E2E"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:pades-verify-e2e#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:pades-verify-e2e#s1", "children": ["urn:doc:pades-verify-e2e#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:pades-verify-e2e#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:pades-verify-e2e#c1", "content": ["Clause."]}
		]
	},
	"signatureFields": [
		{"@type": "SignatureField", "@id": "urn:doc:pades-verify-e2e#SignerOne", "signatoryName": "SignerOne"}
	]
}`

var e2ePAdESServerOnce sync.Once

// ensureE2EPAdESSigningServer wires DCS_PDF_CORE_PADES_SIGNING_ENDPOINT/
// _X5CHAIN_PEM_FILE to a local test signer, mirroring compiler package's own
// test harness (compiler.SignPAdES resolves its signing material once per
// process, so this must run before the first /sign call in this binary).
func ensureE2EPAdESSigningServer(t *testing.T) {
	t.Helper()
	e2ePAdESServerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dcs-pades-e2e-test")
		if err != nil {
			t.Fatal(err)
		}
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		caTmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(2),
			Subject:               pkix.Name{CommonName: "DCS PAdES e2e test CA"},
			BasicConstraintsValid: true,
			IsCA:                  true,
			KeyUsage:              x509.KeyUsageCertSign,
		}
		caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		caCert, _ := x509.ParseCertificate(caDER)
		leafTmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "DCS PAdES e2e test leaf"},
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageDigitalSignature,
		}
		leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, leafKey.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		chainPath := filepath.Join(dir, "pades-x5chain.pem")
		pemBytes := append(certPEM(leafDER), certPEM(caDER)...)
		if err := os.WriteFile(chainPath, pemBytes, 0o644); err != nil {
			t.Fatal(err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Digest string `json:"digest"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			digest, err := base64.StdEncoding.DecodeString(req.Digest)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			der, err := ecdsa.SignASN1(rand.Reader, leafKey, digest)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"signature": base64.StdEncoding.EncodeToString(der),
			})
		}))
		os.Setenv("DCS_PDF_CORE_PADES_SIGNING_ENDPOINT", srv.URL)
		os.Setenv("DCS_PDF_CORE_PADES_X5CHAIN_PEM_FILE", chainPath)
	})
}

// TestSignThenUpdateThenVerify_MatchesTrue reproduces AC4's real-stack failure
// end-to-end through the actual HTTP handlers: compile -> embed evidence ->
// PAdES-sign -> a C2PA /update on top of the signed PDF (the "revoked as a
// post-sign C2PA update" step) -> /verify, and asserts match=true. This is the
// exact sequence the backend's exportcontract.go/verifycontract.go drive via
// pdf-core's public HTTP contract.
func TestSignThenUpdateThenVerify_MatchesTrue(t *testing.T) {
	ensureE2EPAdESSigningServer(t)

	compiled := compilePDF(t)

	// Embed evidence + sign, mirroring the backend's embed-first-sign-second
	// ContractSigner (POST /sign, multipart pdf+field_name+signatory_name+evidence).
	var signBuf bytes.Buffer
	sw := multipart.NewWriter(&signBuf)
	pdfPart, _ := sw.CreateFormField("pdf")
	pdfPart.Write(compiled)
	fieldPart, _ := sw.CreateFormField("field_name")
	fieldPart.Write([]byte("SignerOne"))
	namePart, _ := sw.CreateFormField("signatory_name")
	namePart.Write([]byte("SignerOne"))
	evidencePart, _ := sw.CreateFormField("evidence")
	evidencePart.Write([]byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"]}`))
	sw.Close()

	signRec := doRequest(http.MethodPost, "/sign", &signBuf, "multipart/form-data; boundary="+sw.Boundary())
	if signRec.Code != http.StatusOK {
		t.Fatalf("/sign: status %d: %s", signRec.Code, signRec.Body.String())
	}
	signedPDF := signRec.Body.Bytes()

	// A C2PA /update on top of the signed PDF, exactly what a post-sign lifecycle
	// transition (e.g. revoke -> suspended) triggers via appendAndCache.
	updatedPDF := postPDFUpdate(t, signedPDF, "suspended")

	verifyRec := doRequest(http.MethodPost, "/verify", bytes.NewReader(updatedPDF), "application/pdf")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("/verify: status %d: %s", verifyRec.Code, verifyRec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(verifyRec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Fatalf("expected match=true for sign -> update -> verify, got %+v", result)
	}
}

// postPDFUpdate POSTs pdf to /update with pAdESVerifyE2EPayload as the (unchanged)
// content and a lifecycle VC tagging status, returning the updated PDF bytes.
func postPDFUpdate(t *testing.T, pdf []byte, status string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	pdfPart, _ := w.CreateFormField("pdf")
	pdfPart.Write(pdf)
	payloadPart, _ := w.CreateFormField("payload")
	payloadPart.Write([]byte(pAdESVerifyE2EPayload))
	vcPart, _ := w.CreateFormField("vc")
	vcPart.Write([]byte(`{"type":["VerifiableCredential","ContractLifecycleCredential"],"credentialSubject":{"status":"` + status + `"}}`))
	w.Close()

	rec := doRequest(http.MethodPost, "/update", &buf, "multipart/form-data; boundary="+w.Boundary())
	if rec.Code != http.StatusOK {
		t.Fatalf("/update (status=%s): status %d: %s", status, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// TestMultiHopThenSignThenUpdateThenVerify_MatchesTrue reproduces the real
// contract lifecycle depth AC4 exercises: several C2PA lifecycle updates
// happen (draft -> submitted -> approved, each its own export-triggered
// /update) BEFORE the PAdES signature is ever applied, and one more update
// happens AFTER signing (e.g. revoke -> suspended). VerifyIncrementalUpdate
// must walk the *entire* incremental-update chain, not assume there is
// exactly one update total.
func TestMultiHopThenSignThenUpdateThenVerify_MatchesTrue(t *testing.T) {
	ensureE2EPAdESSigningServer(t)

	compiled := compilePDF(t)
	afterSubmitted := postPDFUpdate(t, compiled, "pending")
	afterApproved := postPDFUpdate(t, afterSubmitted, "active")

	var signBuf bytes.Buffer
	sw := multipart.NewWriter(&signBuf)
	pdfPart, _ := sw.CreateFormField("pdf")
	pdfPart.Write(afterApproved)
	fieldPart, _ := sw.CreateFormField("field_name")
	fieldPart.Write([]byte("SignerOne"))
	namePart, _ := sw.CreateFormField("signatory_name")
	namePart.Write([]byte("SignerOne"))
	evidencePart, _ := sw.CreateFormField("evidence")
	evidencePart.Write([]byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"]}`))
	sw.Close()

	signRec := doRequest(http.MethodPost, "/sign", &signBuf, "multipart/form-data; boundary="+sw.Boundary())
	if signRec.Code != http.StatusOK {
		t.Fatalf("/sign: status %d: %s", signRec.Code, signRec.Body.String())
	}
	signedPDF := signRec.Body.Bytes()

	afterRevoked := postPDFUpdate(t, signedPDF, "suspended")

	verifyRec := doRequest(http.MethodPost, "/verify", bytes.NewReader(afterRevoked), "application/pdf")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("/verify: status %d: %s", verifyRec.Code, verifyRec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(verifyRec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Fatalf("expected match=true for the full multi-hop update->sign->update chain, got %+v", result)
	}
}
