package c2pa

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
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

var testSignerCertDER = mustGenerateSignerCertDER()

func (s *stubSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return s.sig, nil
}

func (s *stubSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	return [][]byte{append([]byte(nil), testSignerCertDER...)}, nil
}

func mustGenerateSignerCertDER() []byte {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(20260601),
		Subject:               pkix.Name{CommonName: "c2pa-test-signer"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return der
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
		"1.0.1",
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
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(),
		signer,
		TSAConfig{URL: tsa.URL},
		storer,
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
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	tsaCfg := TSAConfig{URL: tsa.URL}
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	// First assertion.
	result1, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	prevHash := result1.ManifestHash

	// Second assertion referencing the first via PrevManifestHash.
	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.1",
		"active", "", "did:example:auth", "", prevHash, time.Now(),
	)

	result2, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err)

	// Second manifest must differ from first (different assertion content).
	assert.NotEqual(t, result1.ManifestHash, result2.ManifestHash)

	// PrevManifestHashFrom on the final PDF must return the second manifest's hash.
	extracted := PrevManifestHashFrom(result2.UpdatedPDF)
	assert.Equal(t, result2.ManifestHash, extracted)
}

func TestAppendManifest_FailsOnPrevManifestHashMismatch(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	_, err := AppendManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL}, storer,
		NewLifecycleAssertion(
			"did:example:contract1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "1.0.1",
			"draft", "", "did:example:auth", "", "deadbeef",
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		),
		basePDF, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prev_manifest_hash mismatch")
}

func TestAppendManifest_PreservesPreviousManifestNameTreeEntries(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	tsaCfg := TSAConfig{URL: tsa.URL}
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	result1, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.1",
		"active", "", "did:example:auth", "", result1.ManifestHash,
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	result2, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		assertion2, result1.UpdatedPDF, nil,
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
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL}, storer,
		testAssertion(), basePDF, nil,
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
	assert.Contains(t, string(streamObj), "/ModDate (D:19700101000000)")
	assert.NotZero(t, filespecObjNum)

	catalogObj, foundCatalog := findCatalogObject(objects)
	require.True(t, foundCatalog, "expected updated catalog object")
	assert.Contains(t, string(catalogObj), "/AF")
	assert.Regexp(t, regexp.MustCompile(`/AF\s*\[.*\d+\s+0\s+R.*\]`), string(catalogObj), "catalog /AF should contain the active manifest FileSpec")
	assert.Contains(t, string(catalogObj), "/Names")
	assert.Contains(t, string(catalogObj), "/EmbeddedFiles")
	assert.Contains(t, string(result.UpdatedPDF), "/ID [<")
}

func TestAppendManifest_ContainsExpectedC2PAPayloadLabels(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0xAB}, 64)}
	storer := &stubStorer{}

	result, err := AppendManifest(
		context.Background(), signer, TSAConfig{URL: tsa.URL}, storer,
		testAssertion(), basePDF, nil,
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

// TestAppendManifest_AFPointsToLatestManifestOnly verifies that /Catalog/AF references
// only the active (latest) manifest after multiple increments (C2PA PDF binding §8.2).
// Historical manifests must still be discoverable via /Names/EmbeddedFiles.
func TestAppendManifest_AFPointsToLatestManifestOnly(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	tsaCfg := TSAConfig{URL: tsa.URL}
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	result1, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	assertion2 := NewLifecycleAssertion(
		"did:example:contract1",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"1.0.1", "active", "", "did:example:auth", "", result1.ManifestHash,
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	result2, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		assertion2, result1.UpdatedPDF, nil,
	)
	require.NoError(t, err)

	// Find the final (latest) catalog object in the PDF.
	objects := parsePDFObjects(result2.UpdatedPDF)
	catalogObj, found := findLatestCatalogObjectFromRaw(objects)
	require.True(t, found, "must find an updated catalog object")

	// After two increments /AF must contain exactly ONE ref — the latest manifest only.
	afSection := extractAFSection(catalogObj)
	require.NotEmpty(t, afSection, "/AF must be present in catalog")

	refRe := regexp.MustCompile(`\d+\s+0\s+R`)
	refs := refRe.FindAll(afSection, -1)
	assert.Equal(t, 1, len(refs),
		"/AF must reference only the active manifest; got %d refs", len(refs))

	// Both manifests must still be discoverable via /Names/EmbeddedFiles.
	assert.Contains(t, string(catalogObj), "/EmbeddedFiles",
		"historical manifests must remain discoverable via /Names/EmbeddedFiles")
}

// extractAFSection returns the bytes of the /AF value from a catalog object dict string.
func extractAFSection(catalogObj []byte) []byte {
	re := regexp.MustCompile(`/AF\s*(\[[^\]]*\]|\d+\s+0\s+R)`)
	m := re.FindSubmatch(catalogObj)
	if len(m) < 2 {
		return nil
	}
	return m[1]
}

// findLatestCatalogObjectFromRaw returns the raw bytes of the catalog with the
// highest object number (i.e. the most-recently written one after increments).
func findLatestCatalogObjectFromRaw(objects map[int][]byte) ([]byte, bool) {
	maxN := -1
	var best []byte
	for n, obj := range objects {
		if bytes.Contains(obj, []byte("/Catalog")) && n > maxN {
			maxN = n
			best = obj
		}
	}
	return best, maxN >= 0
}

func TestPrevManifestHashFrom_NoManifest(t *testing.T) {
	assert.Equal(t, "", PrevManifestHashFrom([]byte("%PDF-1.4\n%%EOF\n")))
}

func TestPrevManifestHashFrom_ReadsActiveEmbeddedManifestPayload(t *testing.T) {
	tsa, _ := newTestTSA(t)
	defer tsa.Close()
	tsaCfg := TSAConfig{URL: tsa.URL}
	basePDF := minimalValidPDF()
	signer := &stubSigner{sig: bytes.Repeat([]byte{0x01}, 64)}
	storer := &stubStorer{}

	result1, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		testAssertion(), basePDF, nil,
	)
	require.NoError(t, err)

	assertion2 := NewLifecycleAssertion(
		"did:example:contract1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "1.0.1",
		"active", "", "did:example:auth", "", result1.ManifestHash,
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	result2, err := AppendManifest(
		context.Background(), signer, tsaCfg, storer,
		assertion2, result1.UpdatedPDF, nil,
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
