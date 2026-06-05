package builder

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/c2pa"

	"github.com/digitorus/timestamp"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixedJSONLD = []byte(`{
	"@context": "https://www.w3.org/2018/credentials/v1",
	"@type": "Contract",
	"contractId": "did:example:abc",
	"title": "Test Contract",
	"parties": ["did:example:alice", "did:example:bob"]
}`)

var fixedInput = ContractInput{
	DID:          "did:example:abc",
	State:        "draft",
	Version:      1,
	Name:         "Test Contract",
	Description:  "A test contract for determinism checks",
	CreatedBy:    "test-user",
	CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	UpdatedAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	ContractData: fixedJSONLD,
}

func TestContractBuilder_Determinism(t *testing.T) {
	out1, err := BuildContract(fixedInput)
	require.NoError(t, err)

	out2, err := BuildContract(fixedInput)
	require.NoError(t, err)

	h1 := sha256.Sum256(out1)
	h2 := sha256.Sum256(out2)
	assert.Equal(t, h1, h2, "same input must produce bit-identical PDF output")
}

func TestContractBuilder_StartsWithPDFMagic(t *testing.T) {
	out, err := BuildContract(fixedInput)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("%PDF-")), "output must start with PDF magic bytes")
}

func TestContractBuilder_ContainsJSONLDAttachment(t *testing.T) {
	out, err := BuildContract(fixedInput)
	require.NoError(t, err)
	// fpdf encodes the filename as UTF-16BE (BOM \xfe\xff followed by 2-byte chars).
	// Search for the UTF-16BE encoding of "contract.jsonld".
	utf16FileName := []byte{
		0xfe, 0xff, // BOM
		0x00, 'c', 0x00, 'o', 0x00, 'n', 0x00, 't', 0x00, 'r', 0x00, 'a', 0x00, 'c', 0x00, 't',
		0x00, '.', 0x00, 'j', 0x00, 's', 0x00, 'o', 0x00, 'n', 0x00, 'l', 0x00, 'd',
	}
	assert.True(t, bytes.Contains(out, utf16FileName),
		"PDF must contain the attachment filename 'contract.jsonld' (UTF-16BE encoded)")
	// Also assert an EmbeddedFile stream is present.
	assert.True(t, bytes.Contains(out, []byte("EmbeddedFile")),
		"PDF must contain an /EmbeddedFile stream")
}

func TestContractBuilder_PDFA3Metadata(t *testing.T) {
	out, err := BuildContract(fixedInput)
	require.NoError(t, err)
	// XMP metadata must declare PDF/A-3.
	assert.True(t, bytes.Contains(out, []byte("pdfaid:part")),
		"PDF must contain XMP pdfaid:part for PDF/A-3 compliance")
}

func TestContractBuilder_EmptyContractDataProducesPDF(t *testing.T) {
	in := fixedInput
	in.ContractData = nil
	out, err := BuildContract(in)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("%PDF-")))
}

// stubSigner and stubStorer satisfy the c2pa interfaces for test purposes.
type stubSigner struct{}

func (s *stubSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return bytes.Repeat([]byte{0xAB}, 64), nil
}

func (s *stubSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	return [][]byte{[]byte("dummy-cert")}, nil
}

type stubStorer struct{}

func (s *stubStorer) CreateFile(_ context.Context, _ any) (*ipfs.IPFSResult, error) {
	r := &ipfs.IPFSResult{}
	r.Identifier.Value = "QmTestCID"
	return r, nil
}

// TestC2PA_ChainLinkageWithRealPDF tests that a two-assertion chain is built
// correctly on a real fpdf-generated PDF: the second manifest must have a
// non-empty prev_manifest_hash pointing to the first, and PrevManifestHashFrom
// must return the second manifest's hash (so a third append would chain correctly).
func TestC2PA_ChainLinkageWithRealPDF(t *testing.T) {
	pdf, err := BuildContract(fixedInput)
	require.NoError(t, err)

	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	signer := mustNewECDSASigner(t)
	storer := &stubStorer{}

	fileHash := c2pa.FileHashOf(fixedJSONLD)
	pdfHash := c2pa.BasePDFHashOf(pdf)

	// First assertion: initial export (draft state, no prev hash).
	assertion1 := c2pa.NewLifecycleAssertion(
		fixedInput.DID, fileHash, pdfHash, "1.0.1",
		"draft", "", "did:example:issuer", "", "", time.Now(),
	)
	result1, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{URL: tsa.URL}, storer,
		assertion1, pdf, nil,
	)
	require.NoError(t, err, "first AppendManifest must succeed")
	assert.NotEmpty(t, result1.ManifestHash, "first manifest hash must be non-empty")

	// PrevManifestHashFrom must find the first manifest's hash in the PDF.
	prevHash := c2pa.PrevManifestHashFrom(result1.UpdatedPDF)
	assert.Equal(t, result1.ManifestHash, prevHash,
		"PrevManifestHashFrom must return the first manifest hash on the real fpdf PDF")

	// Second assertion: state advance to active, chaining from the first.
	assertion2 := c2pa.NewLifecycleAssertion(
		fixedInput.DID, fileHash, pdfHash, "1.0.1",
		"active", "", "did:example:issuer", "", prevHash, time.Now(),
	)
	result2, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{URL: tsa.URL}, storer,
		assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err, "second AppendManifest must succeed")
	assert.NotEqual(t, result1.ManifestHash, result2.ManifestHash,
		"second manifest hash must differ from first")

	// The final PDF must contain prev_manifest_hash bytes from assertion2.
	assert.True(t, bytes.Contains(result2.UpdatedPDF, []byte("prev_manifest_hash")),
		"final PDF must contain prev_manifest_hash in the second manifest JUMBF")

	// PrevManifestHashFrom on the two-manifest PDF must return the SECOND manifest's hash.
	assert.Equal(t, result2.ManifestHash, c2pa.PrevManifestHashFrom(result2.UpdatedPDF),
		"PrevManifestHashFrom must return the latest (second) manifest hash")
}

// TestC2PA_RoundTripWithRealPDF tests that AppendManifest produces a valid
// incremental PDF update when given a real fpdf-generated PDF, and that
// pdfcpu can parse the result without error.
func TestC2PA_RoundTripWithRealPDF(t *testing.T) {
	pdf, err := BuildContract(fixedInput)
	require.NoError(t, err)

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	// Base PDF must be parseable by pdfcpu.
	_, err = pdfapi.ReadValidateAndOptimize(bytes.NewReader(pdf), conf)
	require.NoError(t, err, "base PDF must be parseable by pdfcpu")

	fileHash := c2pa.FileHashOf(fixedJSONLD)
	pdfHash := c2pa.BasePDFHashOf(pdf)
	assertion := c2pa.NewLifecycleAssertion(
		fixedInput.DID, fileHash, pdfHash, "1.0.1",
		"draft", "", "did:example:issuer", "", "", time.Now(),
	)

	tsa2, _ := newTestTSA(t)
	defer tsa2.Close()
	result, err := c2pa.AppendManifest(
		context.Background(),
		mustNewECDSASigner(t),
		c2pa.TSAConfig{URL: tsa2.URL},
		&stubStorer{},
		assertion,
		pdf,
		nil,
	)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(result.UpdatedPDF, pdf), "base PDF must be unchanged")

	// Post-append PDF must still be readable by pdfcpu even with C2PA-specific
	// AFRelationship values that pdfcpu's validator does not currently recognize.
	_, err = pdfapi.ReadContext(bytes.NewReader(result.UpdatedPDF), conf)
	require.NoError(t, err, "post-append PDF must be readable by pdfcpu")

	// AF array must be present in the catalog (C2PA EmbeddedFile wired up).
	assert.True(t, bytes.Contains(result.UpdatedPDF, []byte("/AF")),
		"updated PDF must contain /AF array in catalog")
	assert.True(t, bytes.Contains(result.UpdatedPDF, []byte("C2PA_Manifest")),
		"updated PDF must reference AFRelationship /C2PA_Manifest")
}

// mustTSACert and newTestTSA are reproduced from c2pa/manifest_test.go to avoid
// an import cycle (this file is in package builder, not package c2pa).

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

func newTestTSA(t *testing.T) (*httptest.Server, *x509.Certificate) {
	t.Helper()
	cert, key := mustTSACert(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return srv, cert
}

// ecdsaBuilderSigner is an ECDSA-backed signer for builder tests.
// The stubSigner's dummy cert is not valid DER, which can cause issues with
// the TSA timestamp embedding. This signer uses a real self-signed cert.
type ecdsaBuilderSigner struct {
	priv    *ecdsa.PrivateKey
	certDER []byte
}

func (s *ecdsaBuilderSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	h := sha256.Sum256(data)
	r, ss, err := ecdsa.Sign(rand.Reader, s.priv, h[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	rb, sb := r.Bytes(), ss.Bytes()
	copy(out[32-len(rb):32], rb)
	copy(out[64-len(sb):], sb)
	return out, nil
}

func (s *ecdsaBuilderSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	return [][]byte{append([]byte(nil), s.certDER...)}, nil
}

func mustNewECDSASigner(t *testing.T) *ecdsaBuilderSigner {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "builder-test-signer"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	return &ecdsaBuilderSigner{priv: priv, certDER: der}
}
