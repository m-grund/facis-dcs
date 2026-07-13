package compiler

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"

	"example.com/m/V2/internal/pdfsign/sign"
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

// ---------------------------------------------------------------------------
// TSA protocol fix (bodyless GET {TSA.URL}/{sha256-hex}, per
// deployment/helm/charts/orce/flows/tsa_orce_flow.json) and the PAdES-B-B
// fallback's WARN log.
// ---------------------------------------------------------------------------

// buildTestSignData constructs a sign.SignData for the "SignerOne" field
// using the shared test PAdES signing material (see
// ensurePAdESTestServer), with TSA set to tsaURL (empty disables the
// timestamp). Unlike SignPAdES's cached loadPAdESMaterial, this re-resolves
// the material from the current environment on every call, so tests can
// point at their own per-test TSA server without fighting SignPAdES's
// process-wide sync.Once (which locks in whatever DCS_PDF_CORE_TSA_URL was
// set — or unset — the first time any test calls SignPAdES).
func buildTestSignData(t *testing.T, tsaURL string) sign.SignData {
	t.Helper()
	material := resolvePAdESMaterial(os.Getenv, os.ReadFile)
	if material.err != nil {
		t.Fatalf("resolvePAdESMaterial: %v", material.err)
	}
	signData := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.ApprovalSignature,
			DocMDPPerm: sign.AllowFillingExistingFormFieldsAndSignaturesPerms,
			Info: sign.SignDataSignatureInfo{
				Name: "SignerOne",
				Date: time.Now().UTC(),
			},
		},
		ExistingSignatureFieldName: "SignerOne",
		Signer:                     material.signer,
		DigestAlgorithm:            crypto.SHA256,
		Certificate:                material.leaf,
		CertificateChains:          [][]*x509.Certificate{material.chain},
	}
	if tsaURL != "" {
		signData.TSA = sign.TSA{URL: tsaURL}
	}
	return signData
}

// compileAndEmbedForTest compiles padesTestPayload and embeds signing
// evidence, returning a PDF ready for signPAdESWithFallback/signPAdESBytes.
func compileAndEmbedForTest(t *testing.T) []byte {
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
	return embedded
}

// startMockORCETSAServer starts an httptest server speaking the shape the
// deployed ORCE TSA flow actually implements
// (deployment/helm/charts/orce/flows/tsa_orce_flow.json): a bodyless GET at
// /tsa/{sha256-hex}, responding with a raw RFC 3161 TSR (TimeStampResp).
func startMockORCETSAServer(t *testing.T) *httptest.Server {
	t.Helper()
	tsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate TSA key: %v", err)
	}
	tsaTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "DCS mock ORCE TSA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
	}
	tsaDER, err := x509.CreateCertificate(rand.Reader, tsaTmpl, tsaTmpl, tsaKey.Public(), tsaKey)
	if err != nil {
		t.Fatalf("create TSA cert: %v", err)
	}
	tsaCert, err := x509.ParseCertificate(tsaDER)
	if err != nil {
		t.Fatalf("parse TSA cert: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tsa/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		hashHex := strings.TrimPrefix(r.URL.Path, "/tsa/")
		hashBytes, err := hex.DecodeString(hashHex)
		if err != nil || len(hashBytes) != 32 {
			http.Error(w, "bad hash", http.StatusBadRequest)
			return
		}
		ts := timestamp.Timestamp{
			HashAlgorithm:     crypto.SHA256,
			HashedMessage:     hashBytes,
			Time:              time.Now().UTC(),
			Policy:            asn1.ObjectIdentifier{1, 2, 3, 4, 1},
			Ordering:          true,
			AddTSACertificate: true,
		}
		tsr, err := ts.CreateResponseWithOpts(tsaCert, tsaKey, crypto.SHA256)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tsr)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// TestSignPAdESEmbedsRealTimestampWhenTSAWorks is the key regression test for
// the TSA protocol fix: GetTSA previously POSTed a client-built ASN.1
// TimeStampReq to the bare TSA URL, but the deployed ORCE TSA flow
// (deployment/helm/charts/orce/flows/tsa_orce_flow.json) serves a bodyless
// GET at {TSA.URL}/{sha256-hex}. Against a server that speaks that shape, a
// real RFC 3161 timestamp token must be embedded as an unauthenticated CMS
// attribute (id-aa-timeStampToken, 1.2.840.113549.1.9.16.2.14).
func TestSignPAdESEmbedsRealTimestampWhenTSAWorks(t *testing.T) {
	embedded := compileAndEmbedForTest(t)
	tsaServer := startMockORCETSAServer(t)

	signData := buildTestSignData(t, tsaServer.URL+"/tsa")
	signed, err := signPAdESWithFallback(embedded, signData)
	if err != nil {
		t.Fatalf("signPAdESWithFallback: %v", err)
	}

	_, der := byteRangeContentAndCMS(t, signed)
	timeStampTokenOID, err := asn1.Marshal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14})
	if err != nil {
		t.Fatalf("marshal id-aa-timeStampToken OID: %v", err)
	}
	if !bytes.Contains(der, timeStampTokenOID) {
		t.Error("signed CMS does not carry the id-aa-timeStampToken (1.2.840.113549.1.9.16.2.14) unauthenticated attribute; the TSA timestamp was not embedded")
	}
}

// TestSignPAdESLogsWarningOnTSAFallback verifies that when the TSA fails,
// SignPAdES's PAdES-B-B fallback (kept, not removed) is loud about it: a WARN
// line naming the TSA URL and the underlying error, rather than silently
// downgrading the signature with no trace.
func TestSignPAdESLogsWarningOnTSAFallback(t *testing.T) {
	embedded := compileAndEmbedForTest(t)

	// A server that is closed before use guarantees every request to it fails
	// with connection refused.
	deadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadServer.Close()

	signData := buildTestSignData(t, deadServer.URL+"/tsa")

	var logBuf bytes.Buffer
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()

	signed, err := signPAdESWithFallback(embedded, signData)
	if err != nil {
		t.Fatalf("signPAdESWithFallback: %v (fallback to PAdES-B-B should have succeeded)", err)
	}
	if len(signed) == 0 {
		t.Fatal("signPAdESWithFallback returned no bytes despite a nil error")
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "WARN pades: TSA "+deadServer.URL+"/tsa failed") {
		t.Errorf("expected a WARN log naming the failed TSA URL, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "falling back to PAdES-B-B (no timestamp)") {
		t.Errorf("expected the WARN log to name the PAdES-B-B fallback, got: %q", logOutput)
	}
}
