package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

// minimalPDFWithEmptySigField builds the smallest PDF the PAdES signer accepts:
// a single blank page whose AcroForm carries one empty (/V-less) /Sig field
// titled fieldName. Offsets are computed as the objects are written so the xref
// table is exact and digitorus/pdf resolves the field.
func minimalPDFWithEmptySigField(fieldName string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 5 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>",
		fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T (%s) /Rect [0 0 0 0] /F 4 /P 3 0 R >>", fieldName),
		"<< /Fields [4 0 R] /SigFlags 3 >>",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefStart)
	return buf.Bytes()
}

// oversizedSigner is a crypto.Signer whose signature is far larger than the
// SignatureMaxLength estimate reserves for an ECDSA leaf. It forces
// replaceSignature down its "signature too long, retry" path, which re-invokes
// SignPDF. The bytes need not verify: this test asserts the structural integrity
// of the produced incremental update (xref offsets, single field /V), which must
// hold no matter how many times the size estimate is grown and retried.
type oversizedSigner struct {
	pub  crypto.PublicKey
	size int
}

func (s oversizedSigner) Public() crypto.PublicKey { return s.pub }

func (s oversizedSigner) Sign(_ io.Reader, _ []byte, _ crypto.SignerOpts) ([]byte, error) {
	return bytes.Repeat([]byte{0x2a}, s.size), nil
}

// TestPAdESFieldLinkSurvivesSignatureRetry reproduces the production-only defect:
// when the signature placeholder is undersized (real cert chain + RFC 3161 TSA
// token exceed the estimate), replaceSignature grows the estimate and re-runs
// SignPDF. That re-run must start from a clean incremental-update state; if the
// accumulated xref entries and the filled-field marker survive from the first
// attempt, the emitted xref points at stale byte offsets and a strict validator
// (pyHanko, Adobe) can no longer resolve the signed revision.
func TestPAdESFieldLinkSurvivesSignatureRetry(t *testing.T) {
	base := minimalPDFWithEmptySigField("SignerOne")

	leaf, chain, signer := testLeafAndChain(t)
	signData := SignData{
		Signature: SignDataSignature{
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
			Info:       SignDataSignatureInfo{Name: "SignerOne", Date: time.Now().UTC()},
		},
		ExistingSignatureFieldName: "SignerOne",
		Signer:                     oversizedSigner{pub: signer.Public(), size: 4096},
		DigestAlgorithm:            crypto.SHA256,
		Certificate:                leaf,
		CertificateChains:          [][]*x509.Certificate{chain},
	}

	rdr, err := pdf.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	var out bytes.Buffer
	if err := Sign(bytes.NewReader(base), &out, rdr, int64(len(base)), signData); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	signed := out.Bytes()

	assertFinalXrefOffsetsResolve(t, signed)
	assertByteRangeCoversToEOF(t, signed)

	srdr, err := pdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatalf("parse signed: %v", err)
	}
	acro := srdr.Trailer().Key("Root").Key("AcroForm")
	if acro.IsNull() {
		t.Fatal("signed pdf has no AcroForm")
	}
	fields := acro.Key("Fields")
	var field pdf.Value
	found := false
	for i := 0; i < fields.Len(); i++ {
		if fields.Index(i).Key("T").Text() == "SignerOne" {
			field = fields.Index(i)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("AcroForm has no SignerOne field after signing")
	}
	if field.Key("V").IsNull() {
		t.Fatal("SignerOne field carries no /V after a retried signature: the signature is not linked to its form field")
	}
	if got := field.Key("V").Key("Type").Name(); got != "Sig" {
		t.Fatalf("field /V resolves to /Type %q, want Sig", got)
	}
}

// assertFinalXrefOffsetsResolve parses the last xref table in the PDF and checks
// each in-use offset points at the start of the object it indexes ("<id> 0 obj").
// A retry that reuses stale accumulated offsets leaves entries pointing into the
// wrong revision's bytes, which this catches independently of any lenient reader.
func assertFinalXrefOffsetsResolve(t *testing.T, pdfBytes []byte) {
	t.Helper()
	xrefPos := bytes.LastIndex(pdfBytes, []byte("\nxref\n"))
	if xrefPos < 0 {
		t.Fatal("no xref table in signed PDF")
	}
	rest := pdfBytes[xrefPos+len("\nxref\n"):]
	trailerRel := bytes.Index(rest, []byte("trailer"))
	if trailerRel < 0 {
		t.Fatal("no trailer after last xref")
	}
	lines := bytes.Split(rest[:trailerRel], []byte("\n"))
	var curID int
	inSubsection := false
	for _, raw := range lines {
		line := bytes.TrimRight(raw, "\r ")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) == 2 {
			start, err1 := strconv.Atoi(string(fields[0]))
			count, err2 := strconv.Atoi(string(fields[1]))
			if err1 == nil && err2 == nil && count >= 0 {
				curID = start
				inSubsection = true
				continue
			}
		}
		if !inSubsection || len(fields) < 3 {
			continue
		}
		if string(fields[2]) != "n" {
			curID++
			continue
		}
		offset, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			t.Fatalf("bad xref offset %q", fields[0])
		}
		want := []byte(fmt.Sprintf("%d 0 obj", curID))
		end := offset + len(want) + 2
		if end > len(pdfBytes) {
			end = len(pdfBytes)
		}
		if offset < 0 || offset >= len(pdfBytes) || !bytes.HasPrefix(pdfBytes[offset:end], want) {
			got := ""
			if offset >= 0 && offset < len(pdfBytes) {
				hi := offset + 20
				if hi > len(pdfBytes) {
					hi = len(pdfBytes)
				}
				got = string(pdfBytes[offset:hi])
			}
			t.Fatalf("xref offset for object %d points to %q, not %q: the retried signing pass wrote a stale xref", curID, got, want)
		}
		curID++
	}
}

// assertByteRangeCoversToEOF checks the last /ByteRange's second segment ends at
// the final byte of the PDF. A retried signing pass that writes its output twice
// leaves the covered document followed by an uncovered duplicate, which this
// catches (the second copy pushes EOF far past the ByteRange end).
func assertByteRangeCoversToEOF(t *testing.T, pdfBytes []byte) {
	t.Helper()
	idx := bytes.LastIndex(pdfBytes, []byte("/ByteRange"))
	if idx < 0 {
		t.Fatal("no /ByteRange in signed PDF")
	}
	open := bytes.IndexByte(pdfBytes[idx:], '[')
	closeB := bytes.IndexByte(pdfBytes[idx:], ']')
	if open < 0 || closeB < 0 {
		t.Fatal("malformed /ByteRange array")
	}
	fields := bytes.Fields(pdfBytes[idx+open+1 : idx+closeB])
	if len(fields) != 4 {
		t.Fatalf("/ByteRange needs 4 integers, got %d", len(fields))
	}
	nums := make([]int, 4)
	for i, f := range fields {
		v, err := strconv.Atoi(string(f))
		if err != nil {
			t.Fatalf("non-integer /ByteRange field %q", f)
		}
		nums[i] = v
	}
	covEnd := nums[2] + nums[3]
	if covEnd != len(pdfBytes) {
		t.Fatalf("/ByteRange covers up to byte %d but the PDF is %d bytes: a retried signing pass appended a second, unsigned copy of the document", covEnd, len(pdfBytes))
	}

	// The excluded gap holds "<" + hex + ">"; the hex digit count must be even.
	// An odd count leaves a dangling nibble that offsets the coverage accounting
	// standards validators perform and downgrades an ENTIRE_FILE signature.
	hexDigits := nums[2] - nums[1] - 2
	if hexDigits <= 0 || hexDigits%2 != 0 {
		t.Fatalf("/Contents hex placeholder has %d digits, want a positive even count: a retried pass produced an odd-length signature slot", hexDigits)
	}
}

func testLeafAndChain(t *testing.T) (*x509.Certificate, []*x509.Certificate, crypto.Signer) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "retry test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "retry test leaf"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(leafDER)
	return leaf, []*x509.Certificate{leaf, caCert}, leafKey
}
