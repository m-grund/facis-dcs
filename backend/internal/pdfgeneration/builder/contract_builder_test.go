package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/c2pa"

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
		fixedInput.DID, fileHash, pdfHash, "1.0.0",
		"draft", "", "did:example:issuer", "", "", time.Now(),
	)

	result, err := c2pa.AppendManifest(
		context.Background(),
		&stubSigner{},
		c2pa.TSAConfig{},
		&stubStorer{},
		"did:example:issuer",
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
