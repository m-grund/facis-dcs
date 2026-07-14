package compiler

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/digitorus/pkcs7"
)

// TestPAdESValidationFailsWhenSigningEvidenceStreamTampered reproduces AC16
// (removing the signature-evidence attachment invalidates the PAdES
// validation) at the byte level: the evidence attachment is embedded before
// signing (embed-first-sign-second), so it sits inside the signature's
// /ByteRange-covered content. Overwriting its stream bytes after signing
// changes that covered content without moving any offset, so the CMS
// messageDigest no longer matches and verification must fail.
func TestPAdESValidationFailsWhenSigningEvidenceStreamTampered(t *testing.T) {
	signed := compileSignForTest(t)

	cleanContent, cleanDER := byteRangeContentAndCMS(t, signed)
	cleanP7, err := pkcs7.Parse(cleanDER)
	if err != nil {
		t.Fatalf("parse CMS SignedData: %v", err)
	}
	cleanP7.Content = cleanContent
	if err := cleanP7.Verify(); err != nil {
		t.Fatalf("expected the untampered signature to verify: %v", err)
	}

	tampered := zeroSigningEvidenceStream(t, signed)
	if bytes.Equal(tampered, signed) {
		t.Fatal("tampering must change the signed PDF bytes")
	}
	if len(tampered) != len(signed) {
		t.Fatalf("tampering must preserve length (in-place overwrite), got %d want %d", len(tampered), len(signed))
	}

	tamperedContent, tamperedDER := byteRangeContentAndCMS(t, tampered)
	tamperedP7, err := pkcs7.Parse(tamperedDER)
	if err != nil {
		t.Fatalf("parse CMS SignedData from tampered PDF: %v", err)
	}
	tamperedP7.Content = tamperedContent
	if err := tamperedP7.Verify(); err == nil {
		t.Fatal("expected CMS verification to fail after the signature-evidence attachment was tampered with")
	}
}

// zeroSigningEvidenceStream locates the signing-evidence embedded-file stream
// the same way ExtractSigningEvidence does and overwrites its bytes with
// zeroes in place, preserving the PDF's length and every other offset.
func zeroSigningEvidenceStream(t *testing.T, pdf []byte) []byte {
	t.Helper()

	specMarker := []byte(fmt.Sprintf("/F (%s)", signingEvidenceFileName))
	specPos := bytes.LastIndex(pdf, specMarker)
	if specPos < 0 {
		t.Fatal("no signing-evidence filespec found in signed PDF")
	}
	efPos := bytes.Index(pdf[specPos:], []byte("/EF << /F "))
	if efPos < 0 {
		t.Fatal("signing-evidence filespec missing /EF reference")
	}
	efPos += specPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		t.Fatal("signing-evidence object reference malformed")
	}
	objID, err := strconv.Atoi(strings.TrimSpace(string(pdf[efPos : efPos+refEnd])))
	if err != nil {
		t.Fatalf("signing-evidence object id invalid: %v", err)
	}
	objPos := bytes.LastIndex(pdf, []byte(fmt.Sprintf("%d 0 obj", objID)))
	if objPos < 0 {
		t.Fatalf("signing-evidence object %d not found", objID)
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		t.Fatal("signing-evidence stream start not found")
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		t.Fatal("signing-evidence stream end not found")
	}
	streamEnd += streamStart

	tampered := append([]byte(nil), pdf...)
	for i := streamStart; i < streamEnd; i++ {
		tampered[i] = 0
	}
	return tampered
}
