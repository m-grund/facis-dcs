package builder

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

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
