package c2pa

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedSigner struct {
	sig       []byte
	certChain [][]byte
}

func (s *fixedSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return s.sig, nil
}

func (s *fixedSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	chain := make([][]byte, len(s.certChain))
	for i := range s.certChain {
		chain[i] = append([]byte(nil), s.certChain[i]...)
	}
	return chain, nil
}

// newTestTSA returns an httptest.Server that responds with valid RFC 3161 tokens.
func newTestTSA(t *testing.T) (*httptest.Server, *x509.Certificate) {
	t.Helper()
	cert, key := mustTSACert(t)
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req, err := timestamp.ParseRequest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ts := &timestamp.Timestamp{
			HashAlgorithm: req.HashAlgorithm,
			HashedMessage: req.HashedMessage,
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(1),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
			Nonce:         req.Nonce,
		}
		resp, err := ts.CreateResponse(cert, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	return tsa, cert
}

func TestRequestTimestamp_Success(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()

	token, err := requestTimestamp(context.Background(), tsa.URL, []byte("hello world"))
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestRequestTimestamp_Non200(t *testing.T) {
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer tsa.Close()

	_, err := requestTimestamp(context.Background(), tsa.URL, []byte("hello world"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected TSA status")
}

func TestRequestTimestamp_HashMismatch(t *testing.T) {
	cert, key := mustTSACert(t)
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		req, err := timestamp.ParseRequest(body)
		require.NoError(t, err)

		ts := &timestamp.Timestamp{
			HashAlgorithm: req.HashAlgorithm,
			HashedMessage: []byte("not-the-request-hash"),
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(2),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
			Nonce:         req.Nonce,
		}
		resp, err := ts.CreateResponse(cert, key)
		require.NoError(t, err)
		_, _ = w.Write(resp)
	}))
	defer tsa.Close()

	_, err := requestTimestamp(context.Background(), tsa.URL, []byte("hello world"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hashed message mismatch")
}

// TestBuildManifest_FailsClosedWithNoTSAURL verifies that TSA is mandatory:
// building a manifest without a configured TSA URL must return an error.
func TestBuildManifest_FailsClosedWithNoTSAURL(t *testing.T) {
	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()

	_, _, err := BuildManifest(context.Background(), signer, TSAConfig{}, assertion, 0, 0, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TSA URL must not be empty")
}

// TestBuildManifest_FailsClosedWhenTSAConfiguredAndUnavailable verifies that a
// configured-but-unreachable TSA causes a hard failure.
func TestBuildManifest_FailsClosedWhenTSAConfiguredAndUnavailable(t *testing.T) {
	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()

	_, _, err := BuildManifest(
		context.Background(),
		signer,
		TSAConfig{URL: "http://127.0.0.1:1"},
		assertion,
		0,
		0,
		0,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request TSA timestamp")
}

// TestBuildManifest_ClaimV2SpecVersion verifies that the produced manifest
// contains claim_generator_info with specVersion = "2.4.0".
func TestBuildManifest_ClaimV2SpecVersion(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()

	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()

	manifestBytes, manifestHash, err := BuildManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL},
		assertion, 0, 0, 0,
	)
	require.NoError(t, err)
	require.NotEmpty(t, manifestHash)

	// Manifest bytes contain the string "2.4.0" from claim_generator_info.specVersion.
	assert.Contains(t, string(manifestBytes), "2.4.0", "specVersion must be 2.4.0")
	// Manifest is wrapped in a JUMBF superbox.
	assert.Equal(t, "jumb", string(manifestBytes[4:8]))
	assert.Contains(t, string(manifestBytes), "c2pa.manifest")
	assert.Contains(t, string(manifestBytes), "dcs.contract.lifecycle")
	assert.Contains(t, string(manifestBytes), "c2pa.actions.v2")
}

// TestBuildManifest_WrapsManifestInManifestStore verifies basic JUMBF structure.
func TestBuildManifest_WrapsManifestInManifestStore(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()

	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()

	manifestBytes, manifestHash, err := BuildManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL},
		assertion, 0, 0, 0,
	)
	require.NoError(t, err)
	require.NotEmpty(t, manifestHash)
	assert.Equal(t, "jumb", string(manifestBytes[4:8]))
	assert.True(t, bytes.Contains(manifestBytes, []byte{0x03, 'c', '2', 'p', 'a', 0x00}))
}

// TestBuildUpdateManifest_FailsClosedWithNoTSAURL verifies that update manifests
// also require a TSA URL.
func TestBuildUpdateManifest_FailsClosedWithNoTSAURL(t *testing.T) {
	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()
	assertion.PrevManifestHash = "aabbccdd" + "00000000" + "11111111" + "22222222" + "33333333" + "44444444" + "55555555" + "66666666"

	_, _, err := BuildUpdateManifest(
		context.Background(), signer, TSAConfig{},
		assertion,
		assertion.PrevManifestHash,
		"deadbeef"+"00000000"+"11111111"+"22222222"+"33333333"+"44444444"+"55555555"+"66666666",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TSA URL must not be empty")
}

// TestBuildUpdateManifest_ContainsIngredientV3 verifies that an update manifest
// contains the c2pa.ingredient.v3 parentOf assertion.
func TestBuildUpdateManifest_ContainsIngredientV3(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()

	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := testAssertion()
	prevHash := bytes.Repeat([]byte{0xAB}, 32)
	prevSigHash := bytes.Repeat([]byte{0xCD}, 32)
	assertion.PrevManifestHash = "ab" + "ababababababababababababababababababababababababababababababababababab"[0:62]

	manifestBytes, manifestHash, err := BuildUpdateManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL},
		assertion,
		"ababababababababababababababababababababababababababababababababababab"[0:64],
		"cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
	)
	_ = prevHash
	_ = prevSigHash
	require.NoError(t, err)
	require.NotEmpty(t, manifestHash)

	// Update manifest uses c2pa.update.manifest label.
	assert.Contains(t, string(manifestBytes), "c2pa.update.manifest", "update manifest must use c2um label")
	// Contains ingredient.v3.
	assert.Contains(t, string(manifestBytes), "c2pa.ingredient.v3")
	// Contains lifecycle assertion.
	assert.Contains(t, string(manifestBytes), "dcs.contract.lifecycle")
	// Update manifests must NOT contain c2pa.hash.data.
	assert.NotContains(t, string(manifestBytes), "c2pa.hash.data", "update manifests must not have hard binding")
}

func mustTSACert(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "test-tsa"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}
