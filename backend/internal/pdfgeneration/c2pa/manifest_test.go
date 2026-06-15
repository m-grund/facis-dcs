package c2pa

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
	"encoding/asn1"
	"encoding/hex"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRequestTimestamp_Success(t *testing.T) {
	data := []byte("hello world")
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/timestamp-reply", r.Header.Get("Accept"))
		expectedHash := sha256.Sum256(data)
		assert.Equal(t, hex.EncodeToString(expectedHash[:]), strings.TrimPrefix(r.URL.Path, "/"))

		cert, key := mustTSACert(t)
		ts := &timestamp.Timestamp{
			HashAlgorithm: crypto.SHA256,
			HashedMessage: expectedHash[:],
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(1),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
		}
		resp, err := ts.CreateResponseWithOpts(cert, key, crypto.SHA256)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	defer tsa.Close()

	token, err := requestTimestamp(context.Background(), tsa.URL, data)
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
	tsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cert, key := mustTSACert(t)
		ts := &timestamp.Timestamp{
			HashAlgorithm: crypto.SHA256,
			// Intentionally wrong hash payload for mismatch test.
			HashedMessage: []byte("not-the-request-hash"),
			Time:          time.Now().UTC(),
			SerialNumber:  big.NewInt(2),
			Policy:        asn1.ObjectIdentifier{1, 2, 3, 4, 5},
		}
		resp, err := ts.CreateResponseWithOpts(cert, key, crypto.SHA256)
		require.NoError(t, err)
		_, _ = w.Write(resp)
	}))
	defer tsa.Close()

	_, err := requestTimestamp(context.Background(), tsa.URL, []byte("hello world"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hashed message mismatch")
}

func TestBuildManifest_FailsClosedWhenTSAConfiguredAndUnavailable(t *testing.T) {
	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := NewLifecycleAssertion(
		"did:example:contract1",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"1.0.1",
		"draft",
		"",
		"did:example:auth",
		"",
		"",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)

	_, _, err := BuildManifest(
		context.Background(),
		signer,
		TSAConfig{URL: "http://127.0.0.1:1"},
		"did:example:issuer",
		assertion,
		0,
		0,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request TSA timestamp")
}

func TestBuildManifest_WrapsManifestInManifestStore(t *testing.T) {
	signerCert, _ := mustTSACert(t)
	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, 64), certChain: [][]byte{signerCert.Raw}}
	assertion := NewLifecycleAssertion(
		"did:example:contract1",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"1.0.1",
		"draft",
		"",
		"did:example:auth",
		"",
		"",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)

	manifestBytes, manifestHash, err := BuildManifest(context.Background(), signer, TSAConfig{}, "did:example:issuer", assertion, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, manifestHash)
	assert.Equal(t, "jumb", string(manifestBytes[4:8]))
	assert.Contains(t, string(manifestBytes), "c2pa.manifest")
	assert.Contains(t, string(manifestBytes), "dcs.contract.lifecycle")
	// Top-level manifest store label: toggle byte (0x03), label, NUL terminator.
	assert.True(t, bytes.Contains(manifestBytes, []byte{0x03, 'c', '2', 'p', 'a', 0x00}))
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
