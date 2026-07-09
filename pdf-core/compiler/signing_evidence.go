package compiler

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// signingEvidenceFileName is the attachment filename under which the SD-JWT VC
// PID presentation + ContractSigningSummaryCredential are embedded before a
// PAdES signature is applied (embed-first-sign-second, DCS-FR-SM-08).
const signingEvidenceFileName = "signing-evidence.json"

var signingEvidenceMarker = []byte("\n% dcs-pdf-core signing evidence\n")

// EmbedSigningEvidence appends a PDF incremental update that attaches evidence
// as an embedded file. The original bytes are preserved as a prefix so a
// subsequently applied PAdES signature's ByteRange covers the evidence. When
// evidence is empty the PDF is returned unchanged.
func EmbedSigningEvidence(pdfBytes, evidence []byte) ([]byte, error) {
	if len(evidence) == 0 {
		return pdfBytes, nil
	}

	maxObjID, err := findTrailerMaxObjID(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("embed evidence: find max object ID: %w", err)
	}
	prevStartXref, err := previousStartXref(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("embed evidence: find startxref: %w", err)
	}
	fileID := extractTrailerID(pdfBytes)

	baseLen := len(pdfBytes)
	fileObjID := maxObjID + 1
	specObjID := maxObjID + 2

	var buf bytes.Buffer
	buf.Write(signingEvidenceMarker)

	fileOffset := baseLen + buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", fileObjID))
	buf.Write(streamObject(evidence, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fjson /Length %d >>", len(evidence))))
	buf.WriteString("\nendobj\n")

	specOffset := baseLen + buf.Len()
	buf.WriteString(fmt.Sprintf(
		"%d 0 obj\n<< /Type /Filespec /F (%s) /UF (%s) /AFRelationship /Supplement /EF << /F %d 0 R >> >>\nendobj\n",
		specObjID, signingEvidenceFileName, signingEvidenceFileName, fileObjID))

	xrefStart := baseLen + buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString(fmt.Sprintf("%d 2\n", fileObjID))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", fileOffset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", specOffset))

	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		specObjID+1, prevStartXref, idEntry, xrefStart))

	return append(append([]byte(nil), pdfBytes...), buf.Bytes()...), nil
}

// ExtractSigningEvidence returns the raw evidence attachment bytes embedded by
// EmbedSigningEvidence. Returns (evidence, true, nil) when present, (nil, false,
// nil) when absent, and (nil, false, err) on a malformed reference.
func ExtractSigningEvidence(pdfBytes []byte) ([]byte, bool, error) {
	specMarker := []byte(fmt.Sprintf("/F (%s)", signingEvidenceFileName))
	specPos := bytes.LastIndex(pdfBytes, specMarker)
	if specPos < 0 {
		return nil, false, nil
	}
	efPos := bytes.Index(pdfBytes[specPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, false, fmt.Errorf("%s filespec missing /EF reference", signingEvidenceFileName)
	}
	efPos += specPos + len("/EF << /F ")
	refEnd := bytes.Index(pdfBytes[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, false, fmt.Errorf("%s object reference malformed", signingEvidenceFileName)
	}
	objID, err := strconv.Atoi(strings.TrimSpace(string(pdfBytes[efPos : efPos+refEnd])))
	if err != nil {
		return nil, false, fmt.Errorf("%s object id invalid: %w", signingEvidenceFileName, err)
	}
	objPos := bytes.LastIndex(pdfBytes, []byte(fmt.Sprintf("%d 0 obj", objID)))
	if objPos < 0 {
		return nil, false, fmt.Errorf("%s object %d not found", signingEvidenceFileName, objID)
	}
	streamStart := bytes.Index(pdfBytes[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, false, fmt.Errorf("%s stream start not found", signingEvidenceFileName)
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdfBytes[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, false, fmt.Errorf("%s stream end not found", signingEvidenceFileName)
	}
	return append([]byte(nil), pdfBytes[streamStart:streamStart+streamEnd]...), true, nil
}
