package compiler

import (
	"bytes"
	"fmt"
)

// ExtractPageContentByteRanges returns the [start, end) byte ranges of every
// page content stream in pdfBytes, in document order. A stream is identified as
// a page content stream if its data contains a BT (begin-text) operator — the
// operator that bounds all human-visible text rendering in PDF. Streams that
// do not contain BT (embedded files, ICC profiles, font programs, XRef streams)
// are excluded.
func ExtractPageContentByteRanges(pdf []byte) ([][2]int, error) {
	var ranges [][2]int
	search := pdf
	pos := 0
	for {
		streamMarker := []byte("stream\n")
		streamIdx := bytes.Index(search, streamMarker)
		if streamIdx < 0 {
			break
		}
		streamDataStart := pos + streamIdx + len(streamMarker)

		endMarker := []byte("\nendstream")
		endIdx := bytes.Index(search[streamIdx+len(streamMarker):], endMarker)
		if endIdx < 0 {
			break
		}
		streamDataEnd := streamDataStart + endIdx

		streamData := search[streamIdx+len(streamMarker) : streamIdx+len(streamMarker)+endIdx]
		if bytes.Contains(streamData, []byte("BT")) && !isC2PAManifestDict(search, streamIdx) {
			ranges = append(ranges, [2]int{streamDataStart, streamDataEnd})
		}

		advance := streamIdx + len(streamMarker) + endIdx + len(endMarker)
		pos += advance
		search = search[advance:]
	}
	return ranges, nil
}

// isC2PAManifestDict reports whether the object dictionary immediately
// preceding the stream keyword at streamIdx declares the stream as the
// embedded C2PA manifest (/Subtype /application#2Fc2pa). The manifest's
// binary JUMBF payload can incidentally contain the bytes "BT", which would
// otherwise misclassify the manifest stream as page content and make it
// "overlap" its own exclusion window exactly (seen as an intermittent
// compiler-invariant panic in /sign under real load).
func isC2PAManifestDict(search []byte, streamIdx int) bool {
	dictStart := 0
	if objIdx := bytes.LastIndex(search[:streamIdx], []byte(" obj")); objIdx >= 0 {
		dictStart = objIdx
	}
	return bytes.Contains(search[dictStart:streamIdx], []byte("/Subtype /application#2Fc2pa"))
}

// rangesOverlap reports whether the half-open intervals [aStart, aEnd) and
// [bStart, bEnd) overlap.
func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

// checkCoverageWithExclusions is the inner implementation of the coverage
// invariant check, accepting explicit exclusions for testability.
// It returns an error if any page content stream byte range overlaps any
// exclusion window — meaning that human-visible content would be excluded from
// the C2PA hard binding hash and therefore unprovenanced.
func checkCoverageWithExclusions(pdf []byte, exclusions []c2paExclusion) error {
	contentRanges, err := ExtractPageContentByteRanges(pdf)
	if err != nil {
		return fmt.Errorf("extracting page content ranges: %w", err)
	}
	for _, r := range contentRanges {
		for _, ex := range exclusions {
			if ex.Length <= 0 {
				continue
			}
			if rangesOverlap(r[0], r[1], ex.Start, ex.Start+ex.Length) {
				return fmt.Errorf(
					"page content stream [%d, %d) overlaps C2PA exclusion [%d, %d): human-visible content is not provenanced",
					r[0], r[1], ex.Start, ex.Start+ex.Length,
				)
			}
		}
	}
	return nil
}

// CheckPageContentC2PACoverage returns an error if any page content stream byte
// in pdfBytes falls within a C2PA exclusion window. A nil return means all
// human-visible content is covered by the hard binding hash.
//
// The C2PA manifest stream (object 9) is the only permitted exclusion region.
// If the exclusion window were to extend into page content territory — due to a
// compiler bug or a tampered manifest — this function detects it.
func CheckPageContentC2PACoverage(pdf []byte) error {
	const c2paObjectID = 9
	streamStart, streamLen, found := findLastObjectStreamRange(pdf, c2paObjectID)
	if !found {
		return fmt.Errorf("C2PA manifest stream (obj %d) not found in PDF", c2paObjectID)
	}
	return checkCoverageWithExclusions(pdf, []c2paExclusion{{Start: streamStart, Length: streamLen}})
}
