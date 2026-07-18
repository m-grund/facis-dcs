package dss

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLive_PAdESThroughRealDSS proves the rQES two-call flow against a REAL EU
// DSS: it takes a pdf-core-compiled PDF (with the "SignerOne" AcroForm field),
// runs getDataToSign, signs the DTBS with a local EC key, runs signDocument,
// and asserts the result is a standards-shaped PAdES (a /ByteRange signature
// dictionary declaring SubFilter ETSI.CAdES.detached in the named field).
//
// Gated on DSS_LIVE_URL (the DSS REST base, e.g. http://localhost:18099) and
// PDF_CORE_URL (default http://localhost:8080) so it stays out of the normal
// unit suite. Run: DSS_LIVE_URL=... go test ./internal/signingmanagement/dss/ -run Live -v
func TestLive_PAdESThroughRealDSS(t *testing.T) {
	dssURL := os.Getenv("DSS_LIVE_URL")
	if dssURL == "" {
		t.Skip("set DSS_LIVE_URL to run the live DSS integration test")
	}
	pdfCoreURL := os.Getenv("PDF_CORE_URL")
	if pdfCoreURL == "" {
		pdfCoreURL = "http://localhost:8080"
	}

	basePDF := compileSignablePDF(t, pdfCoreURL)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leaf := selfSignedLeaf(t, key)

	hashSigner := func(_ context.Context, dtbs []byte) ([]byte, string, error) {
		digest := sha256.Sum256(dtbs)
		der, serr := key.Sign(rand.Reader, digest[:], crypto.SHA256)
		return der, "ECDSA_SHA256", serr
	}

	signed, err := New(dssURL).Sign(context.Background(), basePDF, "contract.pdf", SignParams{
		Format:             FormatPAdES,
		SignatureLevel:     "PAdES_BASELINE_B",
		SigningCertificate: base64.StdEncoding.EncodeToString(leaf),
		DigestAlgorithm:    "SHA256",
		SignatureFieldID:   "SignerOne",
	}, hashSigner)
	require.NoError(t, err, "the DSS must produce a signed document")

	require.True(t, bytes.HasPrefix(signed, []byte("%PDF")), "result is a PDF")
	require.Greater(t, len(signed), len(basePDF), "the signed PDF is larger than the base")
	require.True(t, bytes.Contains(signed, []byte("/ByteRange")), "PAdES signature dictionary /ByteRange present")
	require.True(t, bytes.Contains(signed, []byte("ETSI.CAdES.detached")), "PAdES SubFilter ETSI.CAdES.detached present")
	require.True(t,
		bytes.Contains(signed, []byte("/T (SignerOne)")) || bytes.Contains(signed, []byte("/T(SignerOne)")) ||
			bytes.Contains(signed, utf16be("SignerOne")),
		"the signature lands in the named AcroForm field")
}

func compileSignablePDF(t *testing.T, pdfCoreURL string) []byte {
	t.Helper()
	const payload = `{
      "@context": {"@vocab":"https://w3id.org/facis/dcs/ontology/v1#","dcs":"https://w3id.org/facis/dcs/ontology/v1#"},
      "@id": "urn:doc:dss-live", "@type": "ContractTemplate",
      "metadata": {"@type":"TemplateMetadata","title":"DSS live"},
      "documentStructure": {"@type":"DocumentStructure",
        "layout": [
          {"@type":"LayoutNode","isRoot":true,"children":["urn:doc:dss-live#s1"]},
          {"@type":"LayoutNode","@id":"urn:doc:dss-live#s1","children":["urn:doc:dss-live#c1"]}
        ],
        "blocks": [
          {"@type":"Section","@id":"urn:doc:dss-live#s1","title":"1. Test"},
          {"@type":"Clause","@id":"urn:doc:dss-live#c1","content":["clause one"]}
        ]},
      "signatureFields": [
        {"@type":"SignatureField","@id":"urn:doc:dss-live#SignerOne","signatoryName":"SignerOne"}
      ]
    }`
	req, err := http.NewRequest(http.MethodPost, pdfCoreURL+"/download", bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/ld+json")
	// pdf-core renders a C2PA manifest on /download, calling back to the
	// backend's authenticated internal signing endpoint; forward the in-cluster
	// system credential so that call is accepted.
	token := os.Getenv("DCS_SYSTEM_TOKEN")
	if token == "" {
		token = "dcs-dev-system-token"
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "pdf-core /download: %s", body)
	require.True(t, bytes.HasPrefix(body, []byte("%PDF")), "pdf-core returned a PDF")
	return body
}

func selfSignedLeaf(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "dcs-pades-live-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	// sanity: PEM round-trip
	_ = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return der
}

func utf16be(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}
