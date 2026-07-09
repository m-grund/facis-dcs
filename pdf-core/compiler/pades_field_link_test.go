package compiler

import (
	"bytes"
	"testing"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pkcs7"
)

// TestPAdESFieldCarriesSignatureValue asserts the AcroForm signature field named
// after the signatory (the empty placeholder pdf-core renders) carries a /V that
// resolves to a /Type /Sig value dictionary. A standards-conformant validator
// (Adobe Acrobat, pyHanko, EU DSS) enumerates signatures by walking AcroForm
// fields and resolving /V; a signature object present in the byte stream but not
// linked from its field is invisible to them (DCS-OR-C2PA-010).
func TestPAdESFieldCarriesSignatureValue(t *testing.T) {
	signed := compileSignForTest(t)

	field := findAcroFormFieldByName(t, signed, "SignerOne")

	v := field.Key("V")
	if v.IsNull() {
		t.Fatal("AcroForm field \"SignerOne\" has no /V: the signature object is not linked to its form field")
	}
	if got := v.Key("Type").Name(); got != "Sig" {
		t.Fatalf("field /V resolves to /Type %q, want Sig", got)
	}
	if v.Key("ByteRange").IsNull() {
		t.Fatal("field /V signature dictionary has no /ByteRange")
	}
}

// TestPAdESFieldValueIsByteRangeCMS binds the two halves together: the CMS the
// field's /V points to must be the very CMS that verifies over the document's
// /ByteRange. This rejects a fix that links the field to some placeholder or
// second signature object other than the one actually covering the bytes.
func TestPAdESFieldValueIsByteRangeCMS(t *testing.T) {
	signed := compileSignForTest(t)

	field := findAcroFormFieldByName(t, signed, "SignerOne")
	v := field.Key("V")
	if v.IsNull() {
		t.Fatal("AcroForm field \"SignerOne\" has no /V")
	}

	fieldDER := trimTrailingZeros([]byte(v.Key("Contents").RawString()))
	if len(fieldDER) == 0 {
		t.Fatal("field /V /Contents is empty")
	}

	signedContent, byteRangeDER := byteRangeContentAndCMS(t, signed)
	if !bytes.Equal(fieldDER, trimTrailingZeros(byteRangeDER)) {
		t.Fatal("field /V /Contents differs from the CMS carried in the /ByteRange gap: the field links to a different signature object")
	}

	p7, err := pkcs7.Parse(fieldDER)
	if err != nil {
		t.Fatalf("parse CMS from field /V /Contents: %v", err)
	}
	p7.Content = signedContent
	if err := p7.Verify(); err != nil {
		t.Fatalf("CMS the field /V points to does not verify over the document /ByteRange: %v", err)
	}
}

// findAcroFormFieldByName resolves the newest AcroForm and returns the terminal
// field whose /T text equals name.
func findAcroFormFieldByName(t *testing.T, signed []byte, name string) pdf.Value {
	t.Helper()
	rdr, err := pdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatalf("parse signed pdf: %v", err)
	}
	acroForm := rdr.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		t.Fatal("signed pdf has no /AcroForm")
	}
	fields := acroForm.Key("Fields")
	if fields.IsNull() || fields.Len() == 0 {
		t.Fatal("/AcroForm has no /Fields")
	}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		if field.Key("T").Text() == name {
			return field
		}
	}
	t.Fatalf("no AcroForm field named %q; the signatory placeholder field is missing", name)
	return pdf.Value{}
}

// trimTrailingZeros drops the 0x00 padding the fixed-width /Contents placeholder
// decodes to, so two CMS DERs can be compared for equality.
func trimTrailingZeros(b []byte) []byte {
	end := len(b)
	for end > 0 && b[end-1] == 0x00 {
		end--
	}
	return b[:end]
}
