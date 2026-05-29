package verify

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalPDF is a skeleton PDF that includes an embedded file stream for
// "contract.jsonld" so ExtractJSONLD can parse it. It mirrors how fpdf
// embeds files: a stream object preceded by the filename in the name tree.
//
// Format for test purposes (not spec-complete PDF):
//
//	<stream for embedded content before filename reference>
//	\nstream\n<zlib bytes>\nendstream\n
//	contract.jsonld (name tree reference)
//	%%EOF
func makeFakePDF(jsonldContent []byte, appendExtra []byte) []byte {
	compressed := zlibCompress(jsonldContent)

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	// stream object preceding the filename reference in the name tree.
	buf.WriteString("1 0 obj\n<< /Type /EmbeddedFile >>\n")
	buf.WriteString("\nstream\n")
	buf.Write(compressed)
	buf.WriteString("\nendstream\n")
	buf.WriteString("endobj\n")
	// Filename reference.
	buf.WriteString("(contract.jsonld) 1 0 R\n")
	buf.WriteString("%%EOF\n")
	if len(appendExtra) > 0 {
		buf.Write(appendExtra)
	}
	return buf.Bytes()
}

func TestExtractBasePDF_StripsCRLFAfterEOF(t *testing.T) {
	pdf := []byte("%PDF-1.4\ncontent\n%%EOF\r\nextra data after")
	base, err := extractBasePDF(pdf)
	require.NoError(t, err)
	// extractBasePDF advances past the trailing CRLF and includes it, so the base
	// contains %%EOF followed by \r\n but NOT the "extra data after".
	assert.True(t, bytes.Contains(base, []byte("%%EOF")))
	assert.False(t, bytes.Contains(base, []byte("extra data after")))
}

func TestExtractBasePDF_NoEOFReturnsError(t *testing.T) {
	_, err := extractBasePDF([]byte("not a pdf"))
	assert.Error(t, err)
}

func TestExtractBasePDF_ReturnsUpToFirstEOF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n%%EOF\nmore stuff\n%%EOF\n")
	base, err := extractBasePDF(pdf)
	require.NoError(t, err)
	// Only the first %%EOF is kept.
	assert.Equal(t, 1, bytes.Count(base, []byte("%%EOF")))
}

func TestVerifier_MatchOnUntamperedPDF(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:1"}`)
	pdf := makeFakePDF(jsonld, nil)

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			// Simulate deterministic builder: produce the same base PDF bytes.
			return extractBasePDF(pdf)
		},
	}

	result, err := v.Verify(pdf)
	require.NoError(t, err)

	// Re-generated base equals stored base → match.
	assert.True(t, result.Match)
	assert.Equal(t, result.BasePDFHash, result.StoredBasePDFHash)
}

func TestVerifier_MismatchOnTamperedPDF(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:2"}`)
	pdf := makeFakePDF(jsonld, nil)

	v := &ContractVerifier{
		BuildFn: func(_ []byte) ([]byte, error) {
			// Simulate a different PDF (tampered re-generation diverges from stored).
			return []byte("%PDF-1.4\ntampered\n%%EOF\n"), nil
		},
	}

	result, err := v.Verify(pdf)
	require.NoError(t, err)
	assert.False(t, result.Match)
	assert.NotEqual(t, result.BasePDFHash, result.StoredBasePDFHash)
}

func TestVerifier_StripsIncrementalUpdates(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:3"}`)
	pdf := makeFakePDF(jsonld, []byte("startxref\n9999\n%%EOF\n%%C2PA-MANIFEST-BEGIN abc\nDUMMY\n%%C2PA-MANIFEST-END\n"))

	base, err := extractBasePDF(pdf)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(_ []byte) ([]byte, error) {
			return base, nil
		},
	}

	result, err := v.Verify(pdf)
	require.NoError(t, err)
	assert.True(t, result.Match, "incremental update should be stripped before hash comparison")
}

func TestVerifier_FetchFnUsedWhenManifestStripped(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:4"}`)
	// stripped = base PDF only, no incremental updates
	strippedPDF := makeFakePDF(jsonld, nil)
	// canonical = base PDF + a dummy incremental update appended (must contain
	// "startxref" so hasIncrementalUpdates detects it as a real PDF increment).
	canonicalPDF := makeFakePDF(jsonld, []byte("startxref\n9999\n%%EOF\n"))

	fetchCalled := false
	base, err := extractBasePDF(strippedPDF)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(_ []byte) ([]byte, error) {
			return base, nil
		},
		FetchFn: func() ([]byte, error) {
			fetchCalled = true
			return canonicalPDF, nil
		},
	}

	result, err := v.Verify(strippedPDF)
	require.NoError(t, err)
	assert.True(t, fetchCalled, "FetchFn should be called when no incremental updates present")
	assert.Equal(t, "remote", result.ManifestSource)
	assert.True(t, result.Match)
}

func TestVerifier_FetchFnNotCalledWhenManifestPresent(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:5"}`)
	// PDF already has incremental update appended (contains "startxref" so
	// hasIncrementalUpdates recognises it as a real PDF increment).
	pdf := makeFakePDF(jsonld, []byte("startxref\n9999\n%%EOF\n"))

	base, err := extractBasePDF(pdf)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(_ []byte) ([]byte, error) {
			return base, nil
		},
		FetchFn: func() ([]byte, error) {
			t.Fatal("FetchFn should not be called when manifest is already embedded")
			return nil, nil
		},
	}

	result, err := v.Verify(pdf)
	require.NoError(t, err)
	assert.Equal(t, "embedded", result.ManifestSource)
	assert.True(t, result.Match)
}

func TestVerifier_ManifestSourceNoneWhenFetchFnNil(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:6"}`)
	strippedPDF := makeFakePDF(jsonld, nil)
	base, err := extractBasePDF(strippedPDF)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(_ []byte) ([]byte, error) { return base, nil },
	}

	result, err := v.Verify(strippedPDF)
	require.NoError(t, err)
	assert.Equal(t, "none", result.ManifestSource)
}

func TestSHA256Hex(t *testing.T) {
	data := []byte("hello")
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	assert.Equal(t, want, sha256hex(data))
}
