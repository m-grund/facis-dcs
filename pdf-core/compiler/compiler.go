package compiler

import (
	_ "embed"

	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// liberationSansTTF is the Liberation Sans Regular font program (SIL Open Font
// License). It is metrically compatible with Helvetica/Arial and is embedded
// in every generated PDF to satisfy ISO 19005-3:2012 clause 6.2.11.4.1 which
// requires that all font programs used for rendering be embedded in the file.
//
//go:embed testdata/fonts/LiberationSans-Regular.ttf
var liberationSansTTF []byte

func CompilePDF(payload []byte) ([]byte, error) {
	nquads, expanded, err := NormalizePayload(payload)
	if err != nil {
		return nil, err
	}
	// Hash the URDNA2015 N-Quads for a graph-canonical FileID.
	sum := sha256.Sum256(nquads)
	hashHex := hex.EncodeToString(sum[:])

	// Extract the raw @context so extractDocumentModel can build the namespace
	// map and fetch ontology terms. The JSON is already validated by NormalizePayload.
	var rawRoot map[string]any
	json.Unmarshal(payload, &rawRoot) //nolint:errcheck // already validated above
	rawCtx, _ := rawRoot["@context"].(map[string]any)
	rootID, _ := rawRoot["@id"].(string)

	// Embed the original JSON-LD so /verify can extract and re-compile it.
	doc := extractDocumentModel(expanded, rootID, rawCtx, payload, hashHex)
	return renderPDF(doc), nil
}

func ExtractEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	fileSpecPos := bytes.Index(pdf, []byte("/F (payload.jsonld)"))
	if fileSpecPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD filespec not found")
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference not found")
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference malformed")
	}
	objIDStr := strings.TrimSpace(string(pdf[efPos : efPos+refEnd]))
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return nil, fmt.Errorf("embedded JSON-LD object id invalid: %w", err)
	}

	objMarker := []byte(fmt.Sprintf("%d 0 obj", objID))
	objPos := bytes.Index(pdf, objMarker)
	if objPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object not found")
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, fmt.Errorf("embedded JSON-LD stream start not found")
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, fmt.Errorf("embedded JSON-LD stream end not found")
	}
	return append([]byte(nil), pdf[streamStart:streamStart+streamEnd]...), nil
}

// ExtractLatestEmbeddedJSONLD returns the JSON-LD stream from the most recent
// definition of the embedded-file object.  For an incrementally updated PDF
// this is the superseding object written by UpdatePDF; for a plain compiled PDF
// it is the same object returned by ExtractEmbeddedJSONLD.
func ExtractLatestEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	fileSpecPos := bytes.Index(pdf, []byte("/F (payload.jsonld)"))
	if fileSpecPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD filespec not found")
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference not found")
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference malformed")
	}
	objIDStr := strings.TrimSpace(string(pdf[efPos : efPos+refEnd]))
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return nil, fmt.Errorf("embedded JSON-LD object id invalid: %w", err)
	}

	// Use LastIndex so that the superseding definition in an incremental update
	// takes precedence over the original, mirroring PDF xref resolution rules.
	objMarker := []byte(fmt.Sprintf("%d 0 obj", objID))
	objPos := bytes.LastIndex(pdf, objMarker)
	if objPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object not found")
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, fmt.Errorf("embedded JSON-LD stream start not found")
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, fmt.Errorf("embedded JSON-LD stream end not found")
	}
	return append([]byte(nil), pdf[streamStart:streamStart+streamEnd]...), nil
}

func AppendVerificationWitness(pdf []byte, payload []byte) ([]byte, error) {
	nquads, _, err := NormalizePayload(payload)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(nquads)
	startxref, err := previousStartXref(pdf)
	if err != nil {
		return nil, err
	}
	hashHex := hex.EncodeToString(hash[:])
	payloadHashHex := hex.EncodeToString(hash[:])
	originalC2PA, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return nil, err
	}

	fileID := extractTrailerID(pdf)
	hardBindingHash := make([]byte, 32)
	exclusions := []c2paExclusion{}
	var candidate []byte
	for iteration := 0; iteration < 6; iteration++ {
		updatedC2PA, err := renderVerificationManifestStore(originalC2PA, witnessManifestLabel(hardBindingHash), payloadHashHex, hardBindingHash, exclusions)
		if err != nil {
			return nil, err
		}

		appendix := buildVerificationAppendixBytes(len(pdf), startxref, hashHex, updatedC2PA, fileID)
		candidate = append(append([]byte(nil), pdf...), appendix...)

		streamStart, streamLen, found := findLastObjectStreamRange(candidate, 9)
		if !found {
			return candidate, nil
		}
		nextExclusions := buildC2PAExclusions(streamStart, streamLen)
		nextHash := sha256WithExclusions(candidate, nextExclusions)
		if bytes.Equal(hardBindingHash, nextHash[:]) && exclusionsEqual(exclusions, nextExclusions) {
			return candidate, nil
		}

		hardBindingHash = append([]byte(nil), nextHash[:]...)
		exclusions = nextExclusions
	}

	// All human-visible page content must remain within C2PA coverage after the
	// incremental witness update. Fail verification explicitly if this invariant
	// is violated rather than silently returning an unprovenanced PDF.
	if err := CheckPageContentC2PACoverage(candidate); err != nil {
		return nil, fmt.Errorf("C2PA coverage invariant violated after verification witness: %w", err)
	}

	return candidate, nil
}

// encodeXRefEntry encodes one XRef stream entry using /W [1 4 2]:
// 1-byte type (1=in-use), 4-byte big-endian offset, 2-byte big-endian generation.
func encodeXRefEntry(typ byte, offset int) []byte {
	return []byte{
		typ,
		byte(offset >> 24), byte(offset >> 16), byte(offset >> 8), byte(offset),
		0, 0,
	}
}

// buildVerificationAppendixBytes constructs the incremental update section
// appended to a PDF by the /verify endpoint. It supersedes obj 9 (the C2PA
// manifest) and adds two new objects:
//   - 9000: /Type /Sig  — carries the verification-witness hash payload
//   - 9001: /Type /XRef — cross-reference stream that also serves as the
//     incremental trailer, carrying /ID and /Prev per ISO 19005-3:2012 §6.1
//
// Using a cross-reference stream (rather than a traditional xref section) lets
// the /ID and metadata travel together and satisfies veraPDF's stream-length
// consistency check (clause 6.1.7.1).
func buildVerificationAppendixBytes(baseLen int, previousStartXref int, witnessHashHex string, c2paManifest []byte, fileID string) []byte {
	var appendix bytes.Buffer
	appendix.WriteString("\n% dcs-pdf-core incremental witness\n")

	c2paObjOffset := baseLen + appendix.Len()
	appendix.WriteString("9 0 obj\n")
	appendix.Write(streamObject(c2paManifest, fmt.Sprintf("<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(c2paManifest))))
	appendix.WriteString("\nendobj\n")

	witnessObjOffset := baseLen + appendix.Len()
	appendix.WriteString(fmt.Sprintf("9000 0 obj\n<< /Type /Sig /Name (verification-witness) /Reason (symmetric-rerender-match) /M (D:19700101000000Z) /Contents <%s> >>\nendobj\n", witnessHashHex))

	// Pre-record the offset of the XRef stream object so we can include it in
	// the stream data (self-referential cross-reference entry for obj 9001).
	xrefObjOffset := baseLen + appendix.Len()

	// Build the cross-reference stream data for /W [1 4 2] and
	// /Index [9 1 9000 2]: three entries covering objs 9, 9000, 9001.
	var xrefData bytes.Buffer
	xrefData.Write(encodeXRefEntry(1, c2paObjOffset))
	xrefData.Write(encodeXRefEntry(1, witnessObjOffset))
	xrefData.Write(encodeXRefEntry(1, xrefObjOffset))
	xrefBytes := xrefData.Bytes()

	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	xrefStreamDict := fmt.Sprintf(
		"<< /Type /XRef /W [1 4 2] /Index [9 1 9000 2] /Size 9002"+
			" /Root 1 0 R /Prev %d%s"+
			" /Info << /verification-witness (sha256:%s) >>"+
			" /Length %d >>",
		previousStartXref, idEntry, witnessHashHex, len(xrefBytes),
	)
	appendix.WriteString("9001 0 obj\n")
	appendix.Write(streamObject(xrefBytes, xrefStreamDict))
	appendix.WriteString("\nendobj\n")

	appendix.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefObjOffset))
	return appendix.Bytes()
}

func extractEmbeddedStreamByFileSpecName(pdf []byte, fileName string) ([]byte, error) {
	needle := []byte(fmt.Sprintf("/F (%s)", fileName))
	fileSpecPos := bytes.Index(pdf, needle)
	if fileSpecPos < 0 {
		return nil, fmt.Errorf("filespec %s not found", fileName)
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, fmt.Errorf("embedded stream reference for %s not found", fileName)
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, fmt.Errorf("embedded stream reference for %s malformed", fileName)
	}
	objID, err := strconv.Atoi(strings.TrimSpace(string(pdf[efPos : efPos+refEnd])))
	if err != nil {
		return nil, fmt.Errorf("embedded stream object id invalid for %s: %w", fileName, err)
	}
	objPos := findLastObjectHeaderOffset(pdf, objID)
	if objPos < 0 {
		return nil, fmt.Errorf("embedded stream object %d not found for %s", objID, fileName)
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, fmt.Errorf("embedded stream start not found for %s", fileName)
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, fmt.Errorf("embedded stream end not found for %s", fileName)
	}
	return append([]byte(nil), pdf[streamStart:streamStart+streamEnd]...), nil
}
