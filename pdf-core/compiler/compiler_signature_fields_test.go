package compiler

import (
	"bytes"
	"context"
	"testing"
)

func TestRenderPDFUsesCborContentBoxForSignature(t *testing.T) {
	doc := documentModel{
		Title: "sig fields",
		Sections: []sectionData{
			{Clauses: []clauseData{
				{Segments: []clauseSegment{{Type: "prose", Text: "Clause text."}}},
			}},
		},
		SignatureFields: []sigFieldDef{{Name: "SignerOne", Label: "Signer One"}},
		NamespaceMap:    map[string]string{},
		CanonicalJSON:   []byte(`{}`),
		PayloadHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		FileID:          "0123456789abcdef0123456789abcdef",
	}

	pdf, err := renderPDF(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	contentCredential, ok := extractEmbeddedStreamByFileSpecNameForTest(pdf, "content_credential.c2pa")
	if !ok {
		t.Fatalf("embedded C2PA stream not found")
	}

	label := []byte("c2pa.signature")
	labelPos := -1
	searchStart := 0
	for {
		idx := bytes.Index(contentCredential[searchStart:], label)
		if idx < 0 {
			break
		}
		idx += searchStart
		if idx >= 25 && idx >= 21 && string(contentCredential[idx-21:idx-17]) == "jumd" {
			labelPos = idx
			break
		}
		searchStart = idx + 1
	}
	if labelPos < 0 {
		t.Fatalf("signature JUMBF label not found")
	}
	descStart := labelPos - 25
	descSize := int(bytesToUint32ForTest(contentCredential[descStart : descStart+4]))
	if descStart+descSize > len(contentCredential) {
		t.Fatalf("signature description box is truncated")
	}
	contentStart := descStart + descSize
	if contentStart+8 > len(contentCredential) {
		t.Fatalf("signature content box is truncated")
	}
	contentBoxType := string(contentCredential[contentStart+4 : contentStart+8])
	if contentBoxType != "cbor" {
		t.Fatalf("signature content box type = %q, want %q", contentBoxType, "cbor")
	}
	if contentStart+10 > len(contentCredential) {
		t.Fatalf("signature content payload is truncated")
	}
	contentBoxPayload := contentCredential[contentStart+8 : contentStart+10]
	if !bytes.Equal(contentBoxPayload, []byte{0xD2, 0x84}) {
		t.Fatalf("signature content payload does not look like a COSE_Sign1 tag")
	}
}

func TestExtractDocumentModelParsesSignatureFields(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"schema": "https://schema.org/"
		},
		"@id": "urn:doc:sig-fields-test",
		"signatureFields": [
			{"schema:name": "SignerOne", "label": "Primary Signer"},
			{"name": "SignerTwo"},
			"SignerThree"
		]
	}`)

	model := mustExtractFromPayload(t, payload)

	if len(model.SignatureFields) != 3 {
		t.Fatalf("signature field count = %d, want 3", len(model.SignatureFields))
	}
	if model.SignatureFields[0].Name != "SignerOne" || model.SignatureFields[0].Label != "Primary Signer" {
		t.Fatalf("first signature field = %#v", model.SignatureFields[0])
	}
	if model.SignatureFields[1].Name != "SignerTwo" || model.SignatureFields[1].Label != "SignerTwo" {
		t.Fatalf("second signature field = %#v", model.SignatureFields[1])
	}
	if model.SignatureFields[2].Name != "SignerThree" || model.SignatureFields[2].Label != "SignerThree" {
		t.Fatalf("third signature field = %#v", model.SignatureFields[2])
	}
}

func TestRenderPDFIncludesAcroFormAndSigWidgets(t *testing.T) {
	doc := documentModel{
		Title: "sig fields",
		Sections: []sectionData{
			{Clauses: []clauseData{
				{Segments: []clauseSegment{{Type: "prose", Text: "Clause text."}}},
			}},
		},
		SignatureFields: []sigFieldDef{
			{Name: "SignerOne", Label: "Signer One"},
			{Name: "SignerTwo", Label: "Signer Two"},
		},
		NamespaceMap:  map[string]string{},
		CanonicalJSON: []byte(`{}`),
		PayloadHash:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		FileID:        "0123456789abcdef0123456789abcdef",
	}

	pdf, err := renderPDF(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}

	for _, marker := range [][]byte{
		[]byte("/AcroForm"),
		[]byte("/FT /Sig"),
		[]byte("/T (SignerOne)"),
		[]byte("/T (SignerTwo)"),
	} {
		if !bytes.Contains(pdf, marker) {
			t.Fatalf("PDF missing marker %q", string(marker))
		}
	}
}

func extractEmbeddedStreamByFileSpecNameForTest(pdf []byte, name string) ([]byte, bool) {
	needle := []byte("/F (" + name + ")")
	pos := bytes.Index(pdf, needle)
	if pos < 0 {
		return nil, false
	}
	efPos := bytes.Index(pdf[pos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, false
	}
	efPos += pos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, false
	}
	objNumber := pdf[efPos : efPos+refEnd]
	objMarker := append([]byte(string(objNumber)), []byte(" 0 obj")...)
	objPos := bytes.Index(pdf, objMarker)
	if objPos < 0 {
		return nil, false
	}
	streamPos := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamPos < 0 {
		return nil, false
	}
	streamPos += objPos + len("stream\n")
	endPos := bytes.Index(pdf[streamPos:], []byte("\nendstream"))
	if endPos < 0 {
		return nil, false
	}
	return pdf[streamPos : streamPos+endPos], true
}

func bytesToUint32ForTest(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
