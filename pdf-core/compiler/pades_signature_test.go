package compiler

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
)

const padesTestPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:pades-repro",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "PAdES Repro"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:pades-repro#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:pades-repro#s1", "children": ["urn:doc:pades-repro#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:pades-repro#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:pades-repro#c1", "content": ["Clause."]}
		]
	},
	"signatureFields": [
		{"@type": "SignatureField", "@id": "urn:doc:pades-repro#SignerOne", "signatoryName": "SignerOne"}
	]
}`

// startSharedPAdESSigningServer generates a P-256 leaf under a self-signed CA,
// writes the two-certificate x5chain PEM, and starts an endpoint mirroring the
// backend's POST /internal/pades/sign (ASN.1 DER ECDSA over the posted digest).
// The server outlives any single test because SignPAdES caches its resolved
// signing material for the process.
func startSharedPAdESSigningServer(dir string) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "DCS PAdES test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
	if err != nil {
		panic(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS PAdES test leaf"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, leafKey.Public(), caKey)
	if err != nil {
		panic(err)
	}
	chainPath := filepath.Join(dir, "pades-x5chain.pem")
	pemBytes := append(certPEM(leafDER), certPEM(caDER)...)
	if err := os.WriteFile(chainPath, pemBytes, 0o644); err != nil {
		panic(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Digest string `json:"digest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		digest, err := base64.StdEncoding.DecodeString(req.Digest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		der, err := ecdsa.SignASN1(rand.Reader, leafKey, digest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"signature": base64.StdEncoding.EncodeToString(der),
		})
	}))
	os.Setenv("DCS_PDF_CORE_PADES_SIGNING_ENDPOINT", srv.URL)
	os.Setenv("DCS_PDF_CORE_PADES_X5CHAIN_PEM_FILE", chainPath)
}

// padesTestServer is shared across the pades tests because SignPAdES resolves
// its signing material once per process (sync.Once); a per-test server would be
// torn down before a later test's SignPAdES call reuses the cached endpoint.
var padesTestServerOnce sync.Once

func ensurePAdESTestServer(t *testing.T) {
	t.Helper()
	padesTestServerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dcs-pades-test")
		if err != nil {
			t.Fatal(err)
		}
		startSharedPAdESSigningServer(dir)
	})
}

func compileSignForTest(t *testing.T) []byte {
	t.Helper()
	ensurePAdESTestServer(t)

	ctx := context.Background()
	compiledAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base, err := CompilePDF(ctx, []byte(padesTestPayload), compiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	signed, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES: %v", err)
	}
	return signed
}

// TestPAdESSignedContentsCarriesCMSWithChain asserts the signature value
// dictionary's /Contents holds a real CMS SignedData that embeds the signer's
// certificate chain (DCS-OR-C2PA-010), parsed rather than length-estimated.
func TestPAdESSignedContentsCarriesCMSWithChain(t *testing.T) {
	signed := compileSignForTest(t)

	if !bytes.Contains(signed, []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Fatal("expected the signature dictionary to declare SubFilter ETSI.CAdES.detached")
	}
	if bytes.Contains(signed, []byte("/adbe.pkcs7.detached")) {
		t.Fatal("adbe.pkcs7.detached SubFilter must be relabelled to the PAdES CAdES baseline")
	}

	marker := []byte("/Contents<")
	pos := bytes.Index(signed, marker)
	if pos < 0 {
		t.Fatal("no signature /Contents hex string found")
	}
	hexStart := pos + len(marker)
	hexEnd := bytes.IndexByte(signed[hexStart:], '>')
	if hexEnd < 0 {
		t.Fatal("signature /Contents hex string not terminated")
	}
	der := decodeContentsHex(t, signed[hexStart:hexStart+hexEnd])
	p7, err := pkcs7.Parse(der)
	if err != nil {
		t.Fatalf("parse CMS SignedData from /Contents: %v", err)
	}
	if len(p7.Certificates) == 0 {
		t.Fatal("CMS SignedData embeds no X.509 certificates; the x5chain was not included")
	}
}

// TestPAdESSignatureVerifiesOverByteRange is the load-bearing cryptographic
// check: the CMS SignedData in /Contents must verify as a detached signature
// over the exact bytes the /ByteRange covers. Any post-signing mutation of a
// byte inside the ByteRange (e.g. rewriting /SubFilter after the digest is
// computed) breaks the signed messageDigest and fails here.
func TestPAdESSignatureVerifiesOverByteRange(t *testing.T) {
	signed := compileSignForTest(t)

	signedContent, der := byteRangeContentAndCMS(t, signed)

	p7, err := pkcs7.Parse(der)
	if err != nil {
		t.Fatalf("parse CMS SignedData: %v", err)
	}
	p7.Content = signedContent
	if err := p7.Verify(); err != nil {
		t.Fatalf("PAdES CMS does not verify over its /ByteRange (signature is cryptographically invalid): %v", err)
	}
}

// byteRangeContentAndCMS parses the last /ByteRange array, returns the
// concatenation of the two covered byte segments (the signed content) and the
// DER of the CMS SignedData carried in the excluded /Contents gap.
func byteRangeContentAndCMS(t *testing.T, pdf []byte) (content, der []byte) {
	t.Helper()
	brIdx := bytes.LastIndex(pdf, []byte("/ByteRange"))
	if brIdx < 0 {
		t.Fatal("no /ByteRange in signed PDF")
	}
	open := bytes.IndexByte(pdf[brIdx:], '[')
	closeB := bytes.IndexByte(pdf[brIdx:], ']')
	if open < 0 || closeB < 0 {
		t.Fatal("malformed /ByteRange array")
	}
	fields := bytes.Fields(pdf[brIdx+open+1 : brIdx+closeB])
	if len(fields) != 4 {
		t.Fatalf("/ByteRange must have 4 integers, got %d", len(fields))
	}
	n := make([]int, 4)
	for i, f := range fields {
		v, err := strconv.Atoi(string(f))
		if err != nil {
			t.Fatalf("non-integer /ByteRange field %q: %v", f, err)
		}
		n[i] = v
	}
	o1, l1, o2, l2 := n[0], n[1], n[2], n[3]
	if o1 != 0 || o2+l2 > len(pdf) || o1+l1 >= o2 {
		t.Fatalf("implausible /ByteRange [%d %d %d %d] for %d-byte PDF", o1, l1, o2, l2, len(pdf))
	}
	content = append(append([]byte(nil), pdf[o1:o1+l1]...), pdf[o2:o2+l2]...)

	// The /Contents hex string sits in the excluded gap between the segments.
	gap := pdf[o1+l1 : o2]
	lt := bytes.IndexByte(gap, '<')
	gt := bytes.IndexByte(gap, '>')
	if lt < 0 || gt < 0 || gt <= lt {
		t.Fatal("no /Contents hex string in the excluded ByteRange gap")
	}
	der = decodeContentsHex(t, gap[lt+1:gt])
	return content, der
}

// TestPAdESSignThenUpdateChains reproduces the /sign -> /update ordering: a
// C2PA incremental update must still parse the trailer of a PAdES-signed PDF
// (its startxref keyword is followed by a blank line before the offset).
func TestPAdESSignThenUpdateChains(t *testing.T) {
	signed := compileSignForTest(t)

	if _, err := previousStartXref(signed); err != nil {
		t.Fatalf("previousStartXref on a PAdES-signed PDF: %v", err)
	}

	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	updated, err := UpdatePDFWithVC(context.Background(), signed, []byte(padesTestPayload), evidence, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdatePDFWithVC after PAdES signing: %v", err)
	}
	if len(updated) <= len(signed) {
		t.Fatal("incremental update did not append to the signed PDF")
	}
}

func decodeContentsHex(t *testing.T, h []byte) []byte {
	t.Helper()
	out := make([]byte, 0, len(h)/2)
	var hi int = -1
	for _, c := range h {
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			continue
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	// Trailing zero padding of the placeholder decodes to trailing 0x00 bytes,
	// which pkcs7.Parse tolerates as the DER is self-delimiting.
	return out
}

// TestPAdESSignedPDFRecompilesAsPrefix asserts the invariant the /verify plain
// path relies on: a deterministic recompilation of the embedded payload
// reproduces the leading bytes of a PAdES-signed PDF (the base), with the
// signature and evidence appended after it as an append-only revision.
func TestPAdESSignedPDFRecompilesAsPrefix(t *testing.T) {
	ensurePAdESTestServer(t)
	ctx := context.Background()
	compiledAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	base, err := CompilePDF(ctx, []byte(padesTestPayload), compiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)
	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	signed, err := SignPAdES(ctx, embedded, "SignerOne", "SignerOne")
	if err != nil {
		t.Fatalf("SignPAdES: %v", err)
	}

	effectiveAt, err := ExtractLifecycleEffectiveAt(signed)
	if err != nil {
		t.Fatalf("ExtractLifecycleEffectiveAt: %v", err)
	}
	payload, err := ExtractEmbeddedJSONLD(signed)
	if err != nil {
		t.Fatalf("ExtractEmbeddedJSONLD: %v", err)
	}
	recompiled, err := CompilePDF(ctx, payload, effectiveAt)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}

	if bytes.Equal(ZeroCOSESignatures(signed), ZeroCOSESignatures(recompiled)) {
		t.Fatal("a signed PDF must not be byte-equal to its bare recompilation (it carries appended evidence + signature)")
	}
	if !bytes.HasPrefix(ZeroCOSESignatures(signed), ZeroCOSESignatures(recompiled)) {
		t.Fatal("the recompiled base must be a byte-prefix of the signed PDF; /verify would otherwise report a false content mismatch")
	}
}

func TestPreviousStartXrefToleratesBlankLine(t *testing.T) {
	pdf := []byte("%PDF-1.7\n... xref ...\ntrailer\n<< /Size 40 >>\nstartxref\n\n423908\n%%EOF\n")
	got, err := previousStartXref(pdf)
	if err != nil {
		t.Fatalf("previousStartXref: %v", err)
	}
	if got != 423908 {
		t.Fatalf("startxref offset = %d, want 423908", got)
	}
}
