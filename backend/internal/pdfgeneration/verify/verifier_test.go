package verify

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/c2pa"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeC2PABasePDF produces the smallest PDF that satisfies both constraints:
//  1. pdfcpu can parse it (valid Catalog/Pages/Page + xref) so AppendManifest works.
//  2. ExtractJSONLD can find the embedded JSON-LD: the EmbeddedFile stream object
//     is written before the "(contract.jsonld)" name-tree string in file order,
//     so the backwards byte search in ExtractJSONLD locates the right stream.
//
// Layout: obj 4 (EmbeddedFile stream) → obj 1 (Catalog w/ Names ref) → obj 2/3 → xref → %%EOF
func makeC2PABasePDF(jsonldBytes []byte) []byte {
	compressed := zlibCompress(jsonldBytes)

	var b bytes.Buffer
	offsets := map[int]int{}

	b.WriteString("%PDF-1.4\n")

	// obj 4 — EmbeddedFile stream; written FIRST so ExtractJSONLD finds it
	// when it walks backwards from the "(contract.jsonld)" occurrence in obj 1.
	offsets[4] = b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<</Type /EmbeddedFile /Length %d /Filter /FlateDecode>>\n", len(compressed))
	b.WriteString("\nstream\n")
	b.Write(compressed)
	b.WriteString("\nendstream\nendobj\n")

	// obj 1 — Catalog referencing the embedded file by name "(contract.jsonld)"
	offsets[1] = b.Len()
	b.WriteString("1 0 obj\n<</Type /Catalog /Pages 2 0 R ")
	b.WriteString("/Names <</EmbeddedFiles <</Names [(contract.jsonld) 4 0 R]>>>>>>\n")
	b.WriteString("endobj\n")

	// obj 2 — Pages
	offsets[2] = b.Len()
	b.WriteString("2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n")

	// obj 3 — Page
	offsets[3] = b.Len()
	b.WriteString("3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n")

	xrefPos := b.Len()
	b.WriteString("xref\n0 5\n")
	b.WriteString("0000000000 65535 f \n")
	for _, n := range []int{1, 2, 3, 4} {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[n])
	}
	fmt.Fprintf(&b, "trailer\n<</Size 5 /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", xrefPos)

	return b.Bytes()
}

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

// TestVerifier_C2PAManifestFoundAndSignatureValid verifies that a PDF containing
// a real C2PA JUMBF embedded via AppendManifest causes C2PAManifestFound=true and
// C2PASignatureValid=true in the verify result (DCS-OR-C2PA-006).
func TestVerifier_C2PAManifestFoundAndSignatureValid(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:10"}`)

	// Build a realistic base PDF via the builder so the JSON-LD is embedded correctly.
	basePDF := makeC2PABasePDF(jsonld)

	// Append a C2PA manifest so the verifier can find it.
	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		"did:ex:10",
		c2pa.FileHashOf(jsonld),
		c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	result, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, nil,
	)
	require.NoError(t, err)
	pdfWithManifest := result.UpdatedPDF

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			// Re-build using the same builder so hashes match.
			return makeC2PABasePDF(extracted), nil
		},
	}

	verifyResult, err := v.Verify(pdfWithManifest)
	require.NoError(t, err)
	assert.True(t, verifyResult.C2PAManifestFound, "C2PAManifestFound must be true when JUMBF is embedded")
	assert.True(t, verifyResult.C2PASignatureValid, "C2PASignatureValid must be true for a well-formed COSE manifest")
}

// TestVerifier_VCProofValidWhenVCEmbedded verifies that a PDF with an embedded
// W3C VC (Ed25519Signature2020 proof) reports VCProofValid=true (DCS-OR-C2PA-006).
func TestVerifier_VCProofValidWhenVCEmbedded(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:11"}`)
	basePDF := makeC2PABasePDF(jsonld)

	vcJSON := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "did:ex:11"},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		"did:ex:11", c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	result, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, vcJSON,
	)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
	}

	verifyResult, err := v.Verify(result.UpdatedPDF)
	require.NoError(t, err)
	assert.True(t, verifyResult.VCProofValid, "VCProofValid must be true when a structurally valid Ed25519Signature2020 VC is embedded")
}

// TestVerifier_StatusListURIExtractedFromVC verifies that the credentialStatus.id
// from the embedded VC is surfaced in the verify result (DCS-OR-C2PA-006).
func TestVerifier_StatusListURIExtractedFromVC(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:12"}`)
	basePDF := makeC2PABasePDF(jsonld)

	statusURI := "http://statuslist/v1/tenants/default/status/1"
	vcJSON := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "did:ex:12"},
		"credentialStatus": {"id": "` + statusURI + `", "type": "StatusList2021Entry"},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		"did:ex:12", c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	result, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, vcJSON,
	)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
	}

	verifyResult, err := v.Verify(result.UpdatedPDF)
	require.NoError(t, err)
	assert.Equal(t, statusURI, verifyResult.StatusListURI,
		"StatusListURI must be extracted from credentialStatus.id in the embedded VC")
}

// TestVerifier_LifecycleStatusExtractedFromManifest verifies that the lifecycle
// status recorded in the C2PA manifest is surfaced in the verify result (DCS-OR-C2PA-006).
func TestVerifier_LifecycleStatusExtractedFromManifest(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:14"}`)
	basePDF := makeC2PABasePDF(jsonld)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		"did:ex:14", c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "approved", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	result, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, nil,
	)
	require.NoError(t, err)

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
	}

	verifyResult, err := v.Verify(result.UpdatedPDF)
	require.NoError(t, err)
	assert.Equal(t, "active", verifyResult.LifecycleStatus,
		"LifecycleStatus must be extracted from the lifecycle assertion in the C2PA manifest")
}

// TestVerifier_NoC2PAManifestReportsNotFound verifies the negative case:
// a plain PDF with no C2PA increment reports C2PAManifestFound=false.
func TestVerifier_NoC2PAManifestReportsNotFound(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:13"}`)
	pdf := makeFakePDF(jsonld, nil)

	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			base, _ := extractBasePDF(pdf)
			return base, nil
		},
	}

	result, err := v.Verify(pdf)
	require.NoError(t, err)
	assert.False(t, result.C2PAManifestFound, "C2PAManifestFound must be false for a PDF with no embedded JUMBF")
	assert.False(t, result.C2PASignatureValid, "C2PASignatureValid must be false when no manifest is present")
	assert.False(t, result.VCProofValid, "VCProofValid must be false when no VC is embedded")
}

// c2paStubSigner / c2paStubStorer are copies of the stubs in the c2pa package
// reproduced here to avoid an import cycle (verify_test is in package verify).
type c2paStubSigner struct {
	priv    *ecdsa.PrivateKey
	certDER []byte
}

func (s *c2paStubSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	h := sha256.Sum256(data)
	r, ss, err := ecdsa.Sign(rand.Reader, s.priv, h[:])
	if err != nil {
		return nil, err
	}

	// COSE ES256 uses 64-byte fixed-width r||s encoding.
	out := make([]byte, 64)
	rb := r.Bytes()
	sb := ss.Bytes()
	copy(out[32-len(rb):32], rb)
	copy(out[64-len(sb):], sb)
	return out, nil
}

func (s *c2paStubSigner) CertificateChain(_ context.Context) ([][]byte, error) {
	return [][]byte{append([]byte(nil), s.certDER...)}, nil
}

func mustNewC2PAStubSigner(t *testing.T) *c2paStubSigner {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "c2pa-test-signer"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	return &c2paStubSigner{priv: priv, certDER: der}
}

type c2paStubStorer struct{}

func (s *c2paStubStorer) CreateFile(_ context.Context, _ any) (*ipfs.IPFSResult, error) {
	r := &ipfs.IPFSResult{}
	r.Identifier.Value = "QmTestCID"
	return r, nil
}

// TestVerifier_CheckStatusFnCalledWithCorrectFields verifies that when a PDF
// contains an embedded VC with credentialStatus, the verifier calls CheckStatusFn
// with the correct statusListCredential URL and statusListIndex, and surfaces the
// result in StatusListStatus (DCS-OR-C2PA-006).
func TestVerifier_CheckStatusFnCalledWithCorrectFields(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:15"}`)
	basePDF := makeC2PABasePDF(jsonld)

	contractID := "did:ex:15"
	expectedIndex := c2pa.StatusListIndex(contractID)
	statusListCredential := "http://statuslist/v1/tenants/default/status/1"

	vcJSON := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "` + contractID + `"},
		"credentialStatus": {
			"id": "` + fmt.Sprintf("%s#%d", statusListCredential, expectedIndex) + `",
			"type": "StatusList2021Entry",
			"statusPurpose": "revocation",
			"statusListIndex": "` + fmt.Sprintf("%d", expectedIndex) + `",
			"statusListCredential": "` + statusListCredential + `"
		},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		contractID, c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	result, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, vcJSON,
	)
	require.NoError(t, err)

	var capturedCred string
	var capturedIndex uint32
	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
		CheckStatusFn: func(cred string, idx uint32) (string, error) {
			capturedCred = cred
			capturedIndex = idx
			return "active", nil
		},
	}

	verifyResult, err := v.Verify(result.UpdatedPDF)
	require.NoError(t, err)
	assert.Equal(t, statusListCredential, capturedCred, "CheckStatusFn must receive statusListCredential")
	assert.Equal(t, expectedIndex, capturedIndex, "CheckStatusFn must receive the correct StatusListIndex")
	assert.Equal(t, "active", verifyResult.StatusListStatus)
}

func TestVerifier_UsesLatestVCInChainedPDF(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:16"}`)
	basePDF := makeC2PABasePDF(jsonld)

	contractID := "did:ex:16"
	expectedIndex := c2pa.StatusListIndex(contractID)
	statusListCredentialV1 := "http://statuslist/v1/tenants/default/status/old"
	statusListCredentialV2 := "http://statuslist/v1/tenants/default/status/new"

	vcV1 := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "` + contractID + `"},
		"credentialStatus": {
			"id": "` + fmt.Sprintf("%s#%d", statusListCredentialV1, expectedIndex) + `",
			"type": "StatusList2021Entry",
			"statusPurpose": "revocation",
			"statusListIndex": "` + fmt.Sprintf("%d", expectedIndex) + `",
			"statusListCredential": "` + statusListCredentialV1 + `"
		},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)
	vcV2 := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "` + contractID + `"},
		"credentialStatus": {
			"id": "` + fmt.Sprintf("%s#%d", statusListCredentialV2, expectedIndex) + `",
			"type": "StatusList2021Entry",
			"statusPurpose": "revocation",
			"statusListIndex": "` + fmt.Sprintf("%d", expectedIndex) + `",
			"statusListCredential": "` + statusListCredentialV2 + `"
		},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertionV1 := c2pa.NewLifecycleAssertion(
		contractID, c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "draft", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	res1, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertionV1, basePDF, vcV1,
	)
	require.NoError(t, err)

	assertionV2 := c2pa.NewLifecycleAssertion(
		contractID, c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(res1.UpdatedPDF),
		"1.0.0", "active", "", "did:ex:auth", "", c2pa.PrevManifestHashFrom(res1.UpdatedPDF),
		time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	)
	res2, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertionV2, res1.UpdatedPDF, vcV2,
	)
	require.NoError(t, err)

	var capturedCred string
	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
		CheckStatusFn: func(cred string, idx uint32) (string, error) {
			capturedCred = cred
			assert.Equal(t, expectedIndex, idx)
			return "active", nil
		},
	}

	verifyResult, err := v.Verify(res2.UpdatedPDF)
	require.NoError(t, err)
	assert.Equal(t, "active", verifyResult.LifecycleStatus)
	assert.Equal(t, statusListCredentialV2, capturedCred,
		"verifier must use credentialStatus from the latest chained VC")
	assert.Equal(t, "active", verifyResult.StatusListStatus)
}

func TestVerifier_RemoteCanonicalVCUsedForStatusCheck(t *testing.T) {
	jsonld := []byte(`{"@type":"Contract","id":"did:ex:17"}`)
	basePDF := makeC2PABasePDF(jsonld)

	contractID := "did:ex:17"
	expectedIndex := c2pa.StatusListIndex(contractID)
	statusListCredential := "http://statuslist/v1/tenants/default/status/remote"
	vcJSON := []byte(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential"],
		"credentialSubject": {"id": "` + contractID + `"},
		"credentialStatus": {
			"id": "` + fmt.Sprintf("%s#%d", statusListCredential, expectedIndex) + `",
			"type": "StatusList2021Entry",
			"statusPurpose": "revocation",
			"statusListIndex": "` + fmt.Sprintf("%d", expectedIndex) + `",
			"statusListCredential": "` + statusListCredential + `"
		},
		"proof": {
			"type": "Ed25519Signature2020",
			"proofValue": "zABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHI"
		}
	}`)

	signer := mustNewC2PAStubSigner(t)
	storer := &c2paStubStorer{}
	assertion := c2pa.NewLifecycleAssertion(
		contractID, c2pa.FileHashOf(jsonld), c2pa.BasePDFHashOf(basePDF),
		"1.0.0", "active", "", "did:ex:auth", "", "",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	res, err := c2pa.AppendManifest(
		context.Background(), signer, c2pa.TSAConfig{}, storer,
		"did:ex:issuer", assertion, basePDF, vcJSON,
	)
	require.NoError(t, err)

	// Simulate verification of a stripped local file: no incremental updates.
	strippedPDF, err := extractBasePDF(res.UpdatedPDF)
	require.NoError(t, err)

	var capturedCred string
	v := &ContractVerifier{
		BuildFn: func(extracted []byte) ([]byte, error) {
			return makeC2PABasePDF(extracted), nil
		},
		FetchFn: func() ([]byte, error) {
			return res.UpdatedPDF, nil
		},
		CheckStatusFn: func(cred string, idx uint32) (string, error) {
			capturedCred = cred
			assert.Equal(t, expectedIndex, idx)
			return "active", nil
		},
	}

	verifyResult, err := v.Verify(strippedPDF)
	require.NoError(t, err)
	assert.Equal(t, "remote", verifyResult.ManifestSource)
	assert.Equal(t, statusListCredential, capturedCred,
		"status checks must use VC fields from the remote canonical PDF when fallback is used")
	assert.Equal(t, "active", verifyResult.StatusListStatus)
}

func TestSHA256Hex(t *testing.T) {
	data := []byte("hello")
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	assert.Equal(t, want, sha256hex(data))
}
