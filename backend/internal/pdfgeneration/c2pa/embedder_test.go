package c2pa

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
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

func (s *stubSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	return [][]byte{[]byte("dummy-cert")}, nil
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
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
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
		nil,
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
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	prevHash := result1.ManifestHash

	// Second assertion referencing the first via PrevManifestHash.
	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.0",
		"active", "", "did:example:auth", "", prevHash, time.Now(),
	)

	result2, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err)

	// Second manifest must differ from first (different assertion content).
	assert.NotEqual(t, result1.ManifestHash, result2.ManifestHash)

	// PrevManifestHashFrom on the final PDF must return the second manifest's hash.
	extracted := PrevManifestHashFrom(result2.UpdatedPDF)
	assert.Equal(t, result2.ManifestHash, extracted)
}

func TestAppendManifest_FailsOnPrevManifestHashMismatch(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	_, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer",
		NewLifecycleAssertion(
			"did:example:contract1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "1.0.0",
			"draft", "", "did:example:auth", "", "deadbeef",
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		),
		basePDF, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prev_manifest_hash mismatch")
}

func TestAppendManifest_PreservesPreviousManifestNameTreeEntries(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	result1, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.0",
		"active", "", "did:example:auth", "", result1.ManifestHash,
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	result2, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err)

	objects := parsePDFObjects(result2.UpdatedPDF)
	filespecs := c2paFilespecNames(objects)

	first := manifestFileName(result1.ManifestHash)
	second := manifestFileName(result2.ManifestHash)
	assert.NotEqual(t, first, second)
	assert.Contains(t, filespecs, first)
	assert.Contains(t, filespecs, second)
	assert.GreaterOrEqual(t, len(filespecs), 2)
}

func TestAppendManifest_EmbedsC2PAFileSpecAndStream(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	objects := parsePDFObjects(result.UpdatedPDF)

	filespecObjNum, filespecObj, ok := findC2PAFileSpecObject(objects)
	require.True(t, ok, "expected a Filespec with AFRelationship /C2PA_Manifest")
	assert.Contains(t, string(filespecObj), "/Type /Filespec")
	assert.Contains(t, string(filespecObj), "/Subtype /application#2Fc2pa")
	assert.Contains(t, string(filespecObj), "/AFRelationship /C2PA_Manifest")

	streamObjNum := extractEmbeddedFileRef(t, filespecObj)
	streamObj, exists := objects[streamObjNum]
	require.True(t, exists, "expected referenced EmbeddedFile object to exist")

	assert.Contains(t, string(streamObj), "/Type /EmbeddedFile")
	assert.Contains(t, string(streamObj), "/Subtype /application#2Fc2pa")
	assert.Contains(t, string(streamObj), "/Params <</Size ")
	assert.NotZero(t, filespecObjNum)

	catalogObj, foundCatalog := findCatalogObject(objects)
	require.True(t, foundCatalog, "expected updated catalog object")
	assert.Contains(t, string(catalogObj), "/AF")
	assert.Regexp(t, regexp.MustCompile(`/AF\s*\[.*\d+\s+0\s+R.*\]`), string(catalogObj), "catalog /AF should contain the active manifest FileSpec")
	assert.Contains(t, string(catalogObj), "/Names")
	assert.Contains(t, string(catalogObj), "/EmbeddedFiles")
}

func TestAppendManifest_DocMDPUsesFileAttachmentReferencePath(t *testing.T) {
	basePDF := minimalValidPDF()
	basePDF = append(basePDF, []byte("\n% contains certifying signature marker /DocMDP\n")...)

	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	objects := parsePDFObjects(result.UpdatedPDF)
	_, filespecObj, ok := findC2PAFileSpecObject(objects)
	require.True(t, ok, "expected a Filespec with AFRelationship /C2PA_Manifest")

	annotObj, foundAnnot := findFileAttachmentAnnotationObject(objects)
	require.True(t, foundAnnot, "expected FileAttachment annotation for DocMDP path")
	assert.Contains(t, string(annotObj), "/FS")
	assert.Regexp(t, regexp.MustCompile(`/Subtype\s*/FileAttachment`), string(annotObj))

	catalogObj, foundCatalog := findCatalogObject(objects)
	require.True(t, foundCatalog, "expected updated catalog object")
	assert.Contains(t, string(catalogObj), "/AF")
	assert.NotContains(t, string(catalogObj), "/EmbeddedFiles")

	_ = filespecObj
}

func TestAppendManifest_ContainsExpectedC2PAPayloadLabels(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	objects := parsePDFObjects(result.UpdatedPDF)
	_, filespecObj, ok := findC2PAFileSpecObject(objects)
	require.True(t, ok, "expected a Filespec with AFRelationship /C2PA_Manifest")

	streamObjNum := extractEmbeddedFileRef(t, filespecObj)
	streamObj, exists := objects[streamObjNum]
	require.True(t, exists, "expected referenced EmbeddedFile object to exist")

	payload := extractEmbeddedFilePayload(t, streamObj)
	assert.Greater(t, len(payload), 0)
	assert.Equal(t, "jumb", string(payload[4:8]))
	assert.Contains(t, string(payload), "c2pa.manifest")
	assert.Contains(t, string(payload), "c2pa.claim")
	assert.Contains(t, string(payload), "dcs.contract.lifecycle")
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

func TestPrevManifestHashFrom_ReadsActiveEmbeddedManifestPayload(t *testing.T) {
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	result1, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.0",
		"active", "", "did:example:auth", "", result1.ManifestHash,
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	result2, err := AppendManifest(
		context.Background(), signer, TSAConfig{}, storer,
		"did:example:issuer", assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err)

	assert.NotContains(t, string(result2.UpdatedPDF), "%% DCS-C2PA-HASH:")
	assert.Equal(t, result2.ManifestHash, PrevManifestHashFrom(result2.UpdatedPDF))
}

func parsePDFObjects(pdf []byte) map[int][]byte {
	re := regexp.MustCompile(`(?s)(\d+)\s+0\s+obj\b(.*?)\bendobj`)
	objects := make(map[int][]byte)
	for _, match := range re.FindAllSubmatch(pdf, -1) {
		n, err := strconv.Atoi(string(match[1]))
		if err != nil {
			continue
		}
		objects[n] = match[2]
	}
	return objects
}

func findC2PAFileSpecObject(objects map[int][]byte) (int, []byte, bool) {
	for n, obj := range objects {
		if bytes.Contains(obj, []byte("/Type /Filespec")) && bytes.Contains(obj, []byte("/AFRelationship /C2PA_Manifest")) {
			return n, obj, true
		}
	}
	return 0, nil, false
}

func findCatalogObject(objects map[int][]byte) ([]byte, bool) {
	for _, obj := range objects {
		if bytes.Contains(obj, []byte("/Catalog")) {
			return obj, true
		}
	}
	return nil, false
}

func findFileAttachmentAnnotationObject(objects map[int][]byte) ([]byte, bool) {
	reFileAttachment := regexp.MustCompile(`/Subtype\s*/FileAttachment`)
	for _, obj := range objects {
		if reFileAttachment.Match(obj) {
			return obj, true
		}
	}
	return nil, false
}

func extractEmbeddedFileRef(t *testing.T, filespecObj []byte) int {
	t.Helper()
	re := regexp.MustCompile(`/EF\s*<<\s*/F\s*(\d+)\s+0\s+R\s*>>`)
	match := re.FindSubmatch(filespecObj)
	require.NotNil(t, match, "expected /EF << /F n 0 R >> in Filespec")
	n, err := strconv.Atoi(string(match[1]))
	require.NoError(t, err)
	return n
}

func extractEmbeddedFilePayload(t *testing.T, streamObj []byte) []byte {
	t.Helper()
	re := regexp.MustCompile(`(?s)<<(.+?)>>\s*stream\r?\n(.*?)\r?\nendstream`)
	match := re.FindSubmatch(streamObj)
	require.NotNil(t, match, "expected stream object with dictionary and stream payload")
	dict := match[1]
	payload := match[2]

	if bytes.Contains(dict, []byte("/Filter /FlateDecode")) {
		zr, err := zlib.NewReader(bytes.NewReader(payload))
		require.NoError(t, err)
		defer zr.Close()
		decoded, err := io.ReadAll(zr)
		require.NoError(t, err)
		return decoded
	}

	return payload
}

func c2paFilespecNames(objects map[int][]byte) []string {
	re := regexp.MustCompile(`/Type\s*/Filespec\b`)
	nameRe := regexp.MustCompile(`/F\s*\(([^)]+)\)`)
	out := make([]string, 0)
	for _, obj := range objects {
		if !re.Match(obj) || !bytes.Contains(obj, []byte("/AFRelationship /C2PA_Manifest")) {
			continue
		}
		match := nameRe.FindSubmatch(obj)
		if len(match) == 2 {
			out = append(out, string(match[1]))
		}
	}
	sort.Strings(out)
	return out
}
