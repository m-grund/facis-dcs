package signer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// TestDSSSignerEmbedsThenSignsThroughDSS proves the DSS backend performs
// embed-first-sign-second: the evidence is attached via pdf-core's attach-only
// seam, then the resulting PDF is signed through the DSS rQES flow with the
// backend HSM key producing the signature value over the DSS data-to-be-sign.
func TestDSSSignerEmbedsThenSignsThroughDSS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certPEM := selfSignedCertPEM(t, key)

	const embeddedMarker = "PDF-WITH-EVIDENCE"
	dtbs := []byte("dss-data-to-be-signed")
	signedDoc := []byte("dss-signed-pdf")

	var embedCalled bool
	pdfCoreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/evidence/embed", r.URL.Path)
		embedCalled = true
		require.NoError(t, r.ParseMultipartForm(1<<20))
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(embeddedMarker))
	}))
	defer pdfCoreSrv.Close()

	var sawSignedDocument, sawCert, sawFieldID, sawAlg string
	var sawSignatureValue string
	dssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		params, _ := body["parameters"].(map[string]any)
		sawCert, _ = params["signingCertificate"].(map[string]any)["encodedCertificate"].(string)
		if img, ok := params["imageParameters"].(map[string]any); ok {
			if fp, ok := img["fieldParameters"].(map[string]any); ok {
				sawFieldID, _ = fp["fieldId"].(string)
			}
		}
		if doc, ok := body["toSignDocument"].(map[string]any); ok {
			if b, ok := doc["bytes"].(string); ok {
				decoded, _ := base64.StdEncoding.DecodeString(b)
				sawSignedDocument = string(decoded)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getDataToSign"):
			_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString(dtbs)})
		case strings.HasSuffix(r.URL.Path, "/signDocument"):
			sv, _ := body["signatureValue"].(map[string]any)
			sawSignatureValue, _ = sv["value"].(string)
			sawAlg, _ = sv["algorithm"].(string)
			_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString(signedDoc)})
		default:
			t.Fatalf("unexpected DSS path %q", r.URL.Path)
		}
	}))
	defer dssSrv.Close()

	s, err := NewDSSSigner(dss.New(dssSrv.URL), pdfcore.New(pdfCoreSrv.URL), key, certPEM, "PAdES_BASELINE_B")
	require.NoError(t, err)

	out, err := s.SignPDF(context.Background(), []byte("base-pdf"), "SignerOne", "Signer One", []byte("evidence-vp"))
	require.NoError(t, err)

	require.Equal(t, signedDoc, out, "returns the DSS-signed document")
	require.True(t, embedCalled, "evidence is embedded via pdf-core before signing")
	require.Equal(t, embeddedMarker, sawSignedDocument, "the DSS signs the evidence-embedded PDF, not the bare one")
	require.Equal(t, "SignerOne", sawFieldID, "the AcroForm signature field is named to the DSS")
	require.Equal(t, "ECDSA_SHA256", sawAlg)

	// The signing certificate handed to the DSS is the x5chain leaf.
	leafDER := firstCertDER(t, certPEM)
	require.Equal(t, base64.StdEncoding.EncodeToString(leafDER), sawCert)

	// The signature value verifies as the HSM key's ECDSA over SHA256(DTBS).
	sigDER, err := base64.StdEncoding.DecodeString(sawSignatureValue)
	require.NoError(t, err)
	digest := sha256.Sum256(dtbs)
	require.True(t, ecdsa.VerifyASN1(&key.PublicKey, digest[:], sigDER), "signature value is HSM ECDSA over the DSS DTBS")
}

// TestDSSSignerNoEvidenceSkipsEmbed proves the attach step is skipped when there
// is no evidence, signing the PDF as received.
func TestDSSSignerNoEvidenceSkipsEmbed(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var embedCalled bool
	pdfCoreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		embedCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer pdfCoreSrv.Close()

	var sawDoc string
	dssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if doc, ok := body["toSignDocument"].(map[string]any); ok {
			b, _ := doc["bytes"].(string)
			decoded, _ := base64.StdEncoding.DecodeString(b)
			sawDoc = string(decoded)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getDataToSign") {
			_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString([]byte("d"))})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]string{"bytes": base64.StdEncoding.EncodeToString([]byte("signed"))})
		}
	}))
	defer dssSrv.Close()

	s, err := NewDSSSigner(dss.New(dssSrv.URL), pdfcore.New(pdfCoreSrv.URL), key, selfSignedCertPEM(t, key), "")
	require.NoError(t, err)

	_, err = s.SignPDF(context.Background(), []byte("bare-pdf"), "F", "N", nil)
	require.NoError(t, err)
	require.False(t, embedCalled, "no evidence means no embed call")
	require.Equal(t, "bare-pdf", sawDoc, "the bare PDF is signed as received")
}

func selfSignedCertPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "dcs-pades-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func firstCertDER(t *testing.T, pemData string) []byte {
	t.Helper()
	block, _ := pem.Decode([]byte(pemData))
	require.NotNil(t, block)
	return block.Bytes
}
