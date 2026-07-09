package compiler

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
)

const padesTestPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:pades-repro",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "PAdES Repro"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:pades-repro#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:pades-repro#s1", "children": ["urn:doc:pades-repro#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:pades-repro#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:pades-repro#c1", "content": ["Clause."]}
		]
	},
	"signatureFields": [
		{"@type": "SignatureField", "@id": "urn:doc:pades-repro#SignerOne", "signatoryName": "SignerOne"}
	]
}`

// startSharedPAdESSigningServer generates a P-256 leaf under a self-signed CA,
// writes the two-certificate x5chain PEM, and starts an endpoint mirroring the
// backend's POST /internal/pades/sign (ASN.1 DER ECDSA over the posted digest).
// The server outlives any single test because SignPAdES caches its resolved
// signing material for the process.
func startSharedPAdESSigningServer(dir string) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "DCS PAdES test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
	if err != nil {
		panic(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS PAdES test leaf"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, leafKey.Public(), caKey)
	if err != nil {
		panic(err)
	}
	chainPath := filepath.Join(dir, "pades-x5chain.pem")
	pemBytes := append(certPEM(leafDER), certPEM(caDER)...)
	if err := os.WriteFile(chainPath, pemBytes, 0o644); err != nil {
		panic(err)
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
}

// padesTestServer is shared across the pades tests because SignPAdES resolves
// its signing material once per process (sync.Once); a per-test server would be
// torn down before a later test's SignPAdES call reuses the cached endpoint.
var padesTestServerOnce sync.Once

func ensurePAdESTestServer(t *testing.T) {
	t.Helper()
	padesTestServerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dcs-pades-test")
		if err != nil {
			t.Fatal(err)
		}
		startSharedPAdESSigningServer(dir)
	})
}

func compileSignForTest(t *testing.T) []byte {
	t.Helper()
	ensurePAdESTestServer(t)

	ctx := context.Background()
	compiledAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base, err := CompilePDF(ctx, []byte(padesTestPayload), compiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	signed, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES: %v", err)
	}
	return signed
}

// TestPAdESSignedContentsCarriesCMSWithChain asserts the signature value
// dictionary's /Contents holds a real CMS SignedData that embeds the signer's
// certificate chain (DCS-OR-C2PA-010), parsed rather than length-estimated.
func TestPAdESSignedContentsCarriesCMSWithChain(t *testing.T) {
	signed := compileSignForTest(t)

	if !bytes.Contains(signed, []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Fatal("expected the signature dictionary to declare SubFilter ETSI.CAdES.detached")
	}
	if bytes.Contains(signed, []byte("/adbe.pkcs7.detached")) {
		t.Fatal("adbe.pkcs7.detached SubFilter must be relabelled to the PAdES CAdES baseline")
	}

	marker := []byte("/Contents<")
	pos := bytes.Index(signed, marker)
	if pos < 0 {
		t.Fatal("no signature /Contents hex string found")
	}
	hexStart := pos + len(marker)
	hexEnd := bytes.IndexByte(signed[hexStart:], '>')
	if hexEnd < 0 {
		t.Fatal("signature /Contents hex string not terminated")
	}
	der := decodeContentsHex(t, signed[hexStart:hexStart+hexEnd])
	p7, err := pkcs7.Parse(der)
	if err != nil {
		t.Fatalf("parse CMS SignedData from /Contents: %v", err)
	}
	if len(p7.Certificates) == 0 {
		t.Fatal("CMS SignedData embeds no X.509 certificates; the x5chain was not included")
	}
}

// TestPAdESSignThenUpdateChains reproduces the /sign -> /update ordering: a
// C2PA incremental update must still parse the trailer of a PAdES-signed PDF
// (its startxref keyword is followed by a blank line before the offset).
func TestPAdESSignThenUpdateChains(t *testing.T) {
	signed := compileSignForTest(t)

	if _, err := previousStartXref(signed); err != nil {
		t.Fatalf("previousStartXref on a PAdES-signed PDF: %v", err)
	}

	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	updated, err := UpdatePDFWithVC(context.Background(), signed, []byte(padesTestPayload), evidence, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdatePDFWithVC after PAdES signing: %v", err)
	}
	if len(updated) <= len(signed) {
		t.Fatal("incremental update did not append to the signed PDF")
	}
}

func decodeContentsHex(t *testing.T, h []byte) []byte {
	t.Helper()
	out := make([]byte, 0, len(h)/2)
	var hi int = -1
	for _, c := range h {
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			continue
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	// Trailing zero padding of the placeholder decodes to trailing 0x00 bytes,
	// which pkcs7.Parse tolerates as the DER is self-delimiting.
	return out
}

func TestPreviousStartXrefToleratesBlankLine(t *testing.T) {
	pdf := []byte("%PDF-1.7\n... xref ...\ntrailer\n<< /Size 40 >>\nstartxref\n\n423908\n%%EOF\n")
	got, err := previousStartXref(pdf)
	if err != nil {
		t.Fatalf("previousStartXref: %v", err)
	}
	if got != 423908 {
		t.Fatalf("startxref offset = %d, want 423908", got)
	}
}
