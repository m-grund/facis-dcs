package compiler

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// pdfContentsRefRE matches a /Contents reference in a PDF page dictionary.
var pdfContentsRefRE = regexp.MustCompile(`/Contents (\d+) 0 R`)

// StripEmbeddedJSONLD returns a copy of pdf with the embedded JSON-LD stream
// content replaced by null bytes of the same length.  All object offsets remain
// unchanged so the resulting PDF is structurally valid.  This simulates what a
// mail client or document-management system does when it strips attachments.
func StripEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	start, length, err := findEmbeddedJSONLDStreamRange(pdf)
	if err != nil {
		return nil, err
	}
	result := append([]byte(nil), pdf...)
	for i := start; i < start+length; i++ {
		result[i] = 0
	}
	return result, nil
}

// findEmbeddedJSONLDStreamRange returns the byte offset and length of the
// JSON-LD stream content inside the first definition of the embedded-file
// object.  It is used by both StripEmbeddedJSONLD and the extraction helpers.
func findEmbeddedJSONLDStreamRange(pdf []byte) (start, length int, err error) {
	fileSpecPos := bytes.Index(pdf, []byte("/F (payload.jsonld)"))
	if fileSpecPos < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD filespec not found")
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD object reference not found")
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD object reference malformed")
	}
	objIDStr := string(pdf[efPos : efPos+refEnd])
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return 0, 0, fmt.Errorf("embedded JSON-LD object id invalid: %w", err)
	}

	objMarker := []byte(fmt.Sprintf("%d 0 obj", objID))
	objPos := bytes.Index(pdf, objMarker)
	if objPos < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD object not found")
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD stream start not found")
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD stream end not found")
	}
	return streamStart, streamEnd, nil
}

// MatchPageContent verifies that the page content streams of candidate match
// those of reference byte-for-byte.  Both PDFs must have been produced by the
// dcs-pdf-core compiler.  Returns nil when all pages match, or an error
// describing the first mismatch.
func MatchPageContent(candidate, reference []byte) error {
	candStreams, err := extractPageContentStreams(candidate)
	if err != nil {
		return fmt.Errorf("extract candidate page content: %w", err)
	}
	refStreams, err := extractPageContentStreams(reference)
	if err != nil {
		return fmt.Errorf("extract reference page content: %w", err)
	}
	if len(candStreams) != len(refStreams) {
		return fmt.Errorf("page count mismatch: submitted PDF has %d pages, compiled PDF has %d",
			len(candStreams), len(refStreams))
	}
	for i := range refStreams {
		if !bytes.Equal(candStreams[i], refStreams[i]) {
			return fmt.Errorf("page %d content does not match compiled output", i+1)
		}
	}
	return nil
}

// extractPageContentStreams follows the PDF page tree of pdf, returning the raw
// bytes of each page's content stream in document order.
func extractPageContentStreams(pdf []byte) ([][]byte, error) {
	pageIDs, err := parseCurrentPagesKids(pdf)
	if err != nil {
		return nil, err
	}
	streams := make([][]byte, 0, len(pageIDs))
	for _, pageID := range pageIDs {
		pos := findLastObjectHeaderOffset(pdf, pageID)
		if pos < 0 {
			return nil, fmt.Errorf("page object %d not found", pageID)
		}
		end := bytes.Index(pdf[pos:], []byte("endobj"))
		if end < 0 {
			return nil, fmt.Errorf("page object %d end not found", pageID)
		}
		objBytes := pdf[pos : pos+end]
		m := pdfContentsRefRE.FindSubmatch(objBytes)
		if len(m) < 2 {
			return nil, fmt.Errorf("page object %d has no /Contents reference", pageID)
		}
		contentID, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return nil, fmt.Errorf("page object %d /Contents ref invalid: %w", pageID, err)
		}
		stream, err := extractStreamContentByObjID(pdf, contentID)
		if err != nil {
			return nil, fmt.Errorf("content stream %d: %w", contentID, err)
		}
		streams = append(streams, stream)
	}
	return streams, nil
}

// extractStreamContentByObjID returns the raw bytes between stream\n and
// \nendstream for the latest definition of the given object.
func extractStreamContentByObjID(pdf []byte, objID int) ([]byte, error) {
	objMarker := []byte(fmt.Sprintf("%d 0 obj", objID))
	objPos := bytes.LastIndex(pdf, objMarker)
	if objPos < 0 {
		return nil, fmt.Errorf("object %d not found", objID)
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, fmt.Errorf("object %d has no stream", objID)
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, fmt.Errorf("object %d stream end not found", objID)
	}
	return append([]byte(nil), pdf[streamStart:streamStart+streamEnd]...), nil
}
