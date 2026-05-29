package c2pa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"digital-contracting-service/internal/base/ipfs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSigner returns a fixed signature for any input.
type stubSigner struct{ sig []byte }

func (s *stubSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return s.sig, nil
}

// stubStorer captures stored data and returns a fixed CID.
type stubStorer struct {
	storedData any
}

func (s *stubStorer) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	s.storedData = data
	r := &ipfs.IPFSResult{}
	r.Identifier.Value = "QmTestCID"
	return r, nil
}

func testAssertion() LifecycleAssertion {
	return NewLifecycleAssertion(
		"did:example:contract1",
		"filehash",
		"pdfhash",
		"1.0.0",
		"draft",
		"",
		"did:example:auth",
		"",
		"",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
}

// minimalValidPDF builds the smallest valid PDF that pdfcpu can parse.
// It has a Catalog, Pages dict, and one blank Page.
func minimalValidPDF() []byte {
	var b bytes.Buffer
	offsets := make([]int, 4) // indices 1-3 are obj offsets

	b.WriteString("%PDF-1.4\n")

	offsets[1] = b.Len()
	b.WriteString("1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")

	offsets[2] = b.Len()
	b.WriteString("2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n")

	offsets[3] = b.Len()
	b.WriteString("3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n")

	xrefOffset := b.Len()
	b.WriteString("xref\n0 4\n")
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<</Size 4 /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return b.Bytes()
}

func TestAppendManifest_IncrementalUpdatePreservesBaseLayer(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(),
		signer,
		TSAConfig{},
		storer,
		"did:example:issuer",
		testAssertion(),
		basePDF,
	)
	require.NoError(t, err)

	// Base layer must be unchanged at the start of the updated PDF.
	assert.True(t, bytes.HasPrefix(result.UpdatedPDF, basePDF),
		"base PDF bytes must appear unchanged at the start of the updated PDF")

	// IPFS CID must come from the storer.
	assert.Equal(t, "QmTestCID", result.IPFSCID)

	// ManifestHash must be non-empty hex.
	assert.NotEmpty(t, result.ManifestHash)
	_, decErr := hex.DecodeString(result.ManifestHash)
	assert.NoError(t, decErr, "ManifestHash should be valid hex")
}

func TestAppendManifest_ChainLinkage(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	// First assertion.
	result1, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF,
	)
	require.NoError(t, err)

	prevHash := result1.ManifestHash

	// Second assertion referencing the first via PrevManifestHash.
	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "filehash2", "pdfhash2", "1.0.0",
		"active", "", "did:example:auth", "", prevHash, time.Now(),
	)

	result2, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", assertion2, result1.UpdatedPDF,
	)
	require.NoError(t, err)

	// Second manifest must differ from first (different assertion content).
	assert.NotEqual(t, result1.ManifestHash, result2.ManifestHash)

	// PrevManifestHashFrom on the final PDF must return the second manifest's hash.
	extracted := PrevManifestHashFrom(result2.UpdatedPDF)
	assert.Equal(t, result2.ManifestHash, extracted)
}

func TestPrevManifestHashFrom_NoManifest(t *testing.T) {
	assert.Equal(t, "", PrevManifestHashFrom([]byte("%PDF-1.4\n%%EOF\n")))
}

func TestPrevManifestHashFrom_ExtractsLastHash(t *testing.T) {
	// Simulate a PDF that already has two DCS-C2PA-HASH comments from prior increments.
	h1 := sha256.Sum256([]byte("manifest1"))
	h2 := sha256.Sum256([]byte("manifest2"))
	hex1 := hex.EncodeToString(h1[:])
	hex2 := hex.EncodeToString(h2[:])

	pdf := []byte("%PDF-1.4\n%%EOF\n")
	pdf = append(pdf, []byte("%% DCS-C2PA-HASH: "+hex1+"\n")...)
	pdf = append(pdf, []byte("%% DCS-C2PA-HASH: "+hex2+"\n")...)

	assert.Equal(t, hex2, PrevManifestHashFrom(pdf))
}
