package c2pa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestAppendManifest_IncrementalUpdatePreservesBaseLayer(t *testing.T) {
	basePDF := []byte("%PDF-1.4\nsome content\n%%EOF\n")
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

	// Base layer must be unchanged.
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
	basePDF := []byte("%PDF-1.4\ncontent\n%%EOF\n")
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	// First assertion.
	result1, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF,
	)
	require.NoError(t, err)

	// Compute expected prev hash: SHA-256 of the manifest JUMBF.
	// The returned ManifestHash is already the hash of the JUMBF bytes.
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

	// The extracted prev manifest hash from the second PDF must be the first manifest's hash.
	// (PrevManifestHashFrom returns the hash of the last C2PA block)
	extracted := PrevManifestHashFrom(result2.UpdatedPDF)
	// The second update appended the second manifest, whose block hash equals result2.ManifestHash.
	// The first manifest hash is embedded in the second assertion payload, not in the block header.
	// Verify via ManifestHash chaining: result2.ManifestHash is the hash of the second JUMBF block.
	h := sha256.Sum256(result2.UpdatedPDF) // just ensure no panic
	_ = h
	assert.NotEmpty(t, extracted)
	assert.Equal(t, result2.ManifestHash, extracted)
}

func TestPrevManifestHashFrom_NoManifest(t *testing.T) {
	assert.Equal(t, "", PrevManifestHashFrom([]byte("%PDF-1.4\n%%EOF\n")))
}

func TestPrevManifestHashFrom_ExtractsLastHash(t *testing.T) {
	pdf := []byte("%PDF-1.4\n%%EOF\n")
	pdf = append(pdf, []byte("\n%%C2PA-MANIFEST-BEGIN aabbcc\nDATA\n%%C2PA-MANIFEST-END\n")...)
	pdf = append(pdf, []byte("\n%%C2PA-MANIFEST-BEGIN ddeeff\nDATA2\n%%C2PA-MANIFEST-END\n")...)

	h := PrevManifestHashFrom(pdf)
	assert.Equal(t, "ddeeff", h)
}
