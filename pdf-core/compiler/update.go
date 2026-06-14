package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrNoChanges is returned by UpdatePDF when the new payload is semantically
// identical to the current embedded one and no VC attachment is present.
var ErrNoChanges = errors.New("no changes: payloads are semantically identical")

// DiffNQuads returns the N-Quads present in newPayload but not oldPayload (added)
// and those present in oldPayload but not newPayload (removed).
func DiffNQuads(oldPayload, newPayload []byte) (added, removed []string, err error) {
	oldNQuads, _, err := NormalizePayload(oldPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize old payload: %w", err)
	}
	newNQuads, _, err := NormalizePayload(newPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize new payload: %w", err)
	}
	oldSet := nquadsToSet(oldNQuads)
	newSet := nquadsToSet(newNQuads)
	for q := range newSet {
		if !oldSet[q] {
			added = append(added, q)
		}
	}
	for q := range oldSet {
		if !newSet[q] {
			removed = append(removed, q)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed, nil
}

func nquadsToSet(nquads []byte) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(string(nquads), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			set[line] = true
		}
	}
	return set
}

var pdfTrailerSizeRE = regexp.MustCompile(`/Size (\d+)`)
var pdfTrailerIDRE = regexp.MustCompile(`/ID\s*(\[[^\]]*\])`)
var pdfKidsRE = regexp.MustCompile(`/Kids \[([^\]]+)\]`)
var pdfObjRefRE = regexp.MustCompile(`(\d+) 0 R`)

// extractTrailerID returns the raw /ID array string (e.g. "[<abc…> <def…>]")
// from the last trailer in the PDF, or an empty string if not found.
func extractTrailerID(pdf []byte) string {
	idx := bytes.LastIndex(pdf, []byte("trailer\n"))
	if idx < 0 {
		return ""
	}
	m := pdfTrailerIDRE.FindSubmatch(pdf[idx:])
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// findTrailerMaxObjID returns the maximum object ID in use (/Size - 1 from the last trailer).
func findTrailerMaxObjID(pdf []byte) (int, error) {
	idx := bytes.LastIndex(pdf, []byte("trailer\n"))
	if idx < 0 {
		return 0, fmt.Errorf("trailer not found in PDF")
	}
	m := pdfTrailerSizeRE.FindSubmatch(pdf[idx:])
	if len(m) < 2 {
		return 0, fmt.Errorf("/Size not found in PDF trailer")
	}
	size, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid trailer /Size: %w", err)
	}
	return size - 1, nil
}

// parseCurrentPagesKids returns the page object IDs from the most recent Pages object (obj 2).
func parseCurrentPagesKids(pdf []byte) ([]int, error) {
	pos := findLastObjectHeaderOffset(pdf, 2)
	if pos < 0 {
		return nil, fmt.Errorf("Pages object (2 0 obj) not found")
	}
	end := bytes.Index(pdf[pos:], []byte("endobj"))
	if end < 0 {
		return nil, fmt.Errorf("Pages object end not found")
	}
	objBytes := pdf[pos : pos+end]
	kidsMatch := pdfKidsRE.Find(objBytes)
	if kidsMatch == nil {
		return nil, fmt.Errorf("/Kids not found in Pages object")
	}
	refs := pdfObjRefRE.FindAllSubmatch(kidsMatch, -1)
	ids := make([]int, 0, len(refs))
	for _, m := range refs {
		id, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid page ref %q: %w", m[0], err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// UpdatePDF appends a PDF incremental update to oldPDF that replaces the
// visible page content with a freshly compiled version of newPayload.
// The original PDF bytes are preserved unchanged as a prefix so existing
// C2PA hard-binding signatures remain verifiable over the original byte range.
func UpdatePDF(ctx context.Context, oldPDF []byte, newPayload []byte) ([]byte, error) {
	return updatePDF(ctx, oldPDF, newPayload, nil)
}

// UpdatePDFWithVC appends a PDF incremental update that replaces visible page
// content with a freshly compiled version of newPayload AND embeds vcBytes as
// a "contract-lifecycle-vc.json" attached file.
//
// Unlike UpdatePDF, this function proceeds even when newPayload is semantically
// identical to the current embedded payload, because the VC attachment is itself
// a provenance event (e.g. attaching the initial lifecycle credential to a
// freshly compiled base PDF).
//
// When vcBytes is nil the call delegates to UpdatePDF unchanged.
func UpdatePDFWithVC(ctx context.Context, oldPDF []byte, newPayload []byte, vcBytes []byte) ([]byte, error) {
	if len(vcBytes) == 0 {
		return UpdatePDF(ctx, oldPDF, newPayload)
	}
	return updatePDF(ctx, oldPDF, newPayload, vcBytes)
}

// ExtractManifestStore returns the raw JUMBF C2PA manifest store bytes
// embedded in the PDF under the "content_credential.c2pa" file attachment.
func ExtractManifestStore(pdf []byte) ([]byte, error) {
	return extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
}

// updatePDF is the shared implementation used by UpdatePDF and UpdatePDFWithVC.
// The "no changes" guard is bypassed when vcBytes is non-nil.
func updatePDF(ctx context.Context, oldPDF []byte, newPayload []byte, vcBytes []byte) ([]byte, error) {
	oldPayload, err := ExtractEmbeddedJSONLD(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("extract embedded JSON-LD: %w", err)
	}

	oldNQuads, _, err := NormalizePayload(oldPayload)
	if err != nil {
		return nil, err
	}
	newNQuads, newExpanded, err := NormalizePayload(newPayload)
	if err != nil {
		return nil, err
	}
	oldHash := sha256.Sum256(oldNQuads)
	newHash := sha256.Sum256(newNQuads)
	oldHashHex := hex.EncodeToString(oldHash[:])
	newHashHex := hex.EncodeToString(newHash[:])

	if oldHashHex == newHashHex && len(vcBytes) == 0 {
		return nil, ErrNoChanges
	}

	maxObjID, err := findTrailerMaxObjID(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("find max object ID: %w", err)
	}
	prevStartXref, err := previousStartXref(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("find startxref: %w", err)
	}

	var rawNewRoot map[string]any
	json.Unmarshal(newPayload, &rawNewRoot) //nolint:errcheck // already validated by NormalizePayload
	newCtx, _ := rawNewRoot["@context"].(map[string]any)
	newRootID, _ := rawNewRoot["@id"].(string)
	newDoc := extractDocumentModel(newExpanded, newRootID, newCtx, newPayload, newHashHex)

	// Compile the new document into full page layouts and assign new object IDs
	// beyond the current maximum so the original objects are never overwritten.
	nextID := maxObjID + 1
	newPages := layoutDocumentPages(newDoc)
	for i := range newPages {
		newPages[i].ObjectID = nextID
		nextID++
		newPages[i].ContentID = nextID
		nextID++
		for j := range newPages[i].Annotations {
			newPages[i].Annotations[j].ObjectID = nextID
			nextID++
		}
		for j := range newPages[i].SigFields {
			newPages[i].SigFields[j].AppearanceObjectID = nextID
			nextID++
			newPages[i].SigFields[j].WidgetObjectID = nextID
			nextID++
		}
	}

	// Reserve IDs for VC embedded-file and filespec objects when a VC is present.
	vcFileObjID := 0
	vcSpecObjID := 0
	if vcBytes != nil {
		vcFileObjID = nextID
		nextID++
		vcSpecObjID = nextID
	}

	originalC2PA, err := extractEmbeddedStreamByFileSpecName(oldPDF, "content_credential.c2pa")
	if err != nil {
		return nil, fmt.Errorf("extract original C2PA: %w", err)
	}

	oldSize := maxObjID + 1
	fileID := extractTrailerID(oldPDF)
	hardBindingHash := make([]byte, 32)
	exclusions := []c2paExclusion{}
	var result []byte

	for range 6 {
		updatedC2PA, err := renderVerificationManifestStore(ctx, originalC2PA, updateManifestLabelFromHash(newHashHex), newHashHex, hardBindingHash, exclusions)
		if err != nil {
			return nil, fmt.Errorf("render update manifest: %w", err)
		}
		appendix := buildUpdateAppendixBytes(
			len(oldPDF), prevStartXref, oldSize, fileID,
			updatedC2PA, newDoc.CanonicalJSON, newDoc.PayloadHash,
			newPages, vcBytes, vcFileObjID, vcSpecObjID,
		)
		result = append(append([]byte(nil), oldPDF...), appendix...)

		streamStart, streamLen, found := findLastObjectStreamRange(result, 9)
		if !found {
			return result, nil
		}
		nextExclusions := buildC2PAExclusions(streamStart, streamLen)
		nextHash := sha256WithExclusions(result, nextExclusions)
		if bytes.Equal(hardBindingHash, nextHash[:]) && exclusionsEqual(exclusions, nextExclusions) {
			return result, nil
		}
		hardBindingHash = append([]byte(nil), nextHash[:]...)
		exclusions = nextExclusions
	}
	return result, nil
}

// buildUpdateAppendixBytes constructs the raw bytes of the PDF incremental
// update section. It supersedes:
//   - obj 2  (Pages)        — updated /Kids list pointing to new page objects
//   - obj 9  (C2PA manifest) — updated hard-binding hash and provenance chain
//   - obj 11 (embedded JSON-LD) — replaced with the new canonical payload
//
// New objects (page content streams, page dictionaries, annotations) are
// appended with IDs beyond the existing maximum so originals are unreachable
// via the updated xref chain but their bytes remain intact for signature
// verification.
func buildUpdateAppendixBytes(
	baseLen, prevStartXref, oldSize int,
	fileID string,
	updatedC2PA, newCanonicalJSON []byte,
	newPayloadHash string,
	newPages []pageLayout,
	vcBytes []byte, vcFileObjID, vcSpecObjID int,
) []byte {
	const (
		fontObjID  = 6
		pagesObjID = 2
		c2paObjID  = 9
		embFileID  = 11
		acroFormID = 14
	)

	type objEntry struct{ id, offset int }
	var entries []objEntry

	var buf bytes.Buffer
	buf.WriteString("\n% dcs-pdf-core incremental update\n")

	for _, page := range newPages {
		entries = append(entries, objEntry{page.ContentID, baseLen + buf.Len()})
		content := renderContentStream(page)
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", page.ContentID))
		buf.Write(streamObject([]byte(content), fmt.Sprintf("<< /Length %d >>", len(content))))
		buf.WriteString("\nendobj\n")

		entries = append(entries, objEntry{page.ObjectID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", page.ObjectID))
		buf.WriteString(renderPageObject(page, fontObjID, pagesObjID))
		buf.WriteString("\nendobj\n")

		for _, annotation := range page.Annotations {
			entries = append(entries, objEntry{annotation.ObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", annotation.ObjectID))
			buf.WriteString(renderAnnotationObject(annotation, newPages))
			buf.WriteString("\nendobj\n")
		}

		// Re-emit signature field appearance streams and widget objects so the
		// new pages reference valid (non-null) annotation objects. The original
		// widget objects on the superseded pages are no longer reachable via the
		// updated xref chain, so new ones must be written for each sig field.
		for _, sigField := range page.SigFields {
			appearance := renderSigFieldAppearanceStream(sigField)
			entries = append(entries, objEntry{sigField.AppearanceObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", sigField.AppearanceObjectID))
			buf.Write(streamObject([]byte(appearance), fmt.Sprintf(
				"<< /Type /XObject /Subtype /Form /BBox [0 0 %.2f %.2f] /Resources << /Font << /F1 %d 0 R >> >> /Length %d >>",
				sigField.Rect[2]-sigField.Rect[0], sigField.Rect[3]-sigField.Rect[1],
				fontObjID, len(appearance),
			)))
			buf.WriteString("\nendobj\n")

			entries = append(entries, objEntry{sigField.WidgetObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", sigField.WidgetObjectID))
			buf.WriteString(renderSigFieldWidgetObject(sigField, page.ObjectID, sigField.AppearanceObjectID))
			buf.WriteString("\nendobj\n")
		}
	}

	// Supersede obj 14 (AcroForm) when the new document has signature fields.
	// The original AcroForm referenced widgets on the old pages; those pages are
	// no longer current, so the AcroForm must point to the freshly emitted widgets.
	var sigFieldRefs []string
	for _, page := range newPages {
		for _, sigField := range page.SigFields {
			sigFieldRefs = append(sigFieldRefs, fmt.Sprintf("%d 0 R", sigField.WidgetObjectID))
		}
	}
	if len(sigFieldRefs) > 0 {
		entries = append(entries, objEntry{acroFormID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Fields [%s] /SigFlags 3 /DA (/F1 10 Tf 0 g) >>\nendobj\n",
			acroFormID, strings.Join(sigFieldRefs, " ")))
	}

	// Supersede obj 11: updated embedded JSON-LD.
	entries = append(entries, objEntry{embFileID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", embFileID))
	buf.Write(streamObject(newCanonicalJSON, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fld+json /Length %d /Params << /Size %d /CheckSum <%s> >> >>",
		len(newCanonicalJSON), len(newCanonicalJSON), newPayloadHash[:32],
	)))
	buf.WriteString("\nendobj\n")

	// Supersede obj 2: Pages now points only to the new compiled page objects,
	// replacing the old pages in the reader's view while leaving original
	// objects intact in the byte stream.
	newKids := make([]string, len(newPages))
	for i, p := range newPages {
		newKids[i] = fmt.Sprintf("%d 0 R", p.ObjectID)
	}
	entries = append(entries, objEntry{pagesObjID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
		pagesObjID, strings.Join(newKids, " "), len(newPages)))

	// Append VC embedded-file and filespec objects when a credential is supplied.
	// The EmbeddedFile stream is written before the Filespec so that ExtractVC
	// (which scans backwards from the filename marker) finds the correct stream.
	if len(vcBytes) > 0 {
		entries = append(entries, objEntry{vcFileObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", vcFileObjID))
		buf.Write(streamObject(vcBytes, fmt.Sprintf(
			"<< /Type /EmbeddedFile /Subtype /application#2Fjson /Length %d >>", len(vcBytes))))
		buf.WriteString("\nendobj\n")

		entries = append(entries, objEntry{vcSpecObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Filespec /F (contract-lifecycle-vc.json) /UF (contract-lifecycle-vc.json) /AFRelationship /Supplement /EF << /F %d 0 R >> >>\nendobj\n",
			vcSpecObjID, vcFileObjID))
	}

	// Supersede obj 9: updated C2PA manifest — written last so stream offset stabilises.
	entries = append(entries, objEntry{c2paObjID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", c2paObjID))
	buf.Write(streamObject(updatedC2PA, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(updatedC2PA))))
	buf.WriteString("\nendobj\n")

	// Write xref with contiguous subsections.
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	xrefStart := baseLen + buf.Len()
	buf.WriteString("xref\n")
	i := 0
	for i < len(entries) {
		j := i + 1
		for j < len(entries) && entries[j].id == entries[j-1].id+1 {
			j++
		}
		buf.WriteString(fmt.Sprintf("%d %d\n", entries[i].id, j-i))
		for k := i; k < j; k++ {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", entries[k].offset))
		}
		i = j
	}

	newMaxID := 0
	for _, e := range entries {
		if e.id > newMaxID {
			newMaxID = e.id
		}
	}
	newSize := oldSize
	if newMaxID+1 > newSize {
		newSize = newMaxID + 1
	}

	// ISO 19005-3:2012 clause 6.1.3 requires /ID in every trailer, including
	// incremental update trailers. Carry the original file's /ID forward.
	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		newSize, prevStartXref, idEntry, xrefStart,
	))
	return buf.Bytes()
}

// ExtractEmbeddedVC extracts the raw bytes of the "contract-lifecycle-vc.json"
// embedded-file attachment from a PDF produced by UpdatePDFWithVC.
// Returns (vcBytes, true, nil) when the attachment is present; (nil, false, nil)
// when absent; and (nil, false, err) on a malformed reference.
func ExtractEmbeddedVC(pdf []byte) ([]byte, bool, error) {
	specPos := bytes.LastIndex(pdf, []byte("/F (contract-lifecycle-vc.json)"))
	if specPos < 0 {
		return nil, false, nil
	}
	efPos := bytes.Index(pdf[specPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json filespec missing /EF reference")
	}
	efPos += specPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json object reference malformed")
	}
	objIDStr := strings.TrimSpace(string(pdf[efPos : efPos+refEnd]))
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json object id invalid: %w", err)
	}
	// Use LastIndex so the most recent definition wins (incremental update semantics).
	objMarker := []byte(fmt.Sprintf("%d 0 obj", objID))
	objPos := bytes.LastIndex(pdf, objMarker)
	if objPos < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json object %d not found", objID)
	}
	streamStart := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStart < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json stream start not found")
	}
	streamStart += objPos + len("stream\n")
	streamEnd := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEnd < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json stream end not found")
	}
	return append([]byte(nil), pdf[streamStart:streamStart+streamEnd]...), true, nil
}

// incrementalUpdateMarker is the comment written as the very first line of
// every incremental update section produced by UpdatePDF.
var incrementalUpdateMarker = []byte("\n% dcs-pdf-core incremental update\n")

// SplitAtIncrementalUpdate returns the original PDF prefix that precedes the
// first dcs-pdf-core incremental update marker, and ok=true.  If the PDF has
// no such marker (it is a plain compiled document) ok is false.
func SplitAtIncrementalUpdate(pdf []byte) (original []byte, ok bool) {
	idx := bytes.Index(pdf, incrementalUpdateMarker)
	if idx < 0 {
		return nil, false
	}
	return pdf[:idx], true
}

// VerifyIncrementalUpdate checks that an incrementally-updated PDF was produced
// deterministically from its embedded payloads.  It provides the same guarantee
// as the plain /verify check — the human-readable content is fully determined
// by the machine-readable JSON-LD — extended to cover the amendment history:
//
//  1. CompilePDF(oldPayload) == originalPrefix  (original was deterministic)
//  2. UpdatePDF(originalPrefix, newPayload) == pdf  (amendment was deterministic)
//
// Both conditions together prove the current visible state is reproducible from
// the current embedded payload.
func VerifyIncrementalUpdate(ctx context.Context, pdf []byte) error {
	original, ok := SplitAtIncrementalUpdate(pdf)
	if !ok {
		return fmt.Errorf("no incremental update marker found")
	}

	oldPayload, err := ExtractEmbeddedJSONLD(original)
	if err != nil {
		return fmt.Errorf("extract old payload from original prefix: %w", err)
	}
	newPayload, err := ExtractLatestEmbeddedJSONLD(pdf)
	if err != nil {
		return fmt.Errorf("extract new payload from amended PDF: %w", err)
	}

	freshOriginal, err := CompilePDF(ctx, oldPayload)
	if err != nil {
		return fmt.Errorf("recompile original payload: %w", err)
	}
	// The "original" prefix is the compiled PDF possibly followed by append-only
	// PAdES signature updates. PAdES appends bytes after %%EOF without altering
	// the preceding bytes, so the compiled output must be a byte-for-byte prefix
	// of whatever was submitted as the original.
	if !bytes.HasPrefix(original, freshOriginal) {
		return fmt.Errorf("original PDF prefix does not match deterministic recompilation from its embedded payload")
	}

	// Re-apply the amendment to the actual original (which may include PAdES
	// appendices) so the deterministic update covers the same base offsets.
	// Re-use any VC from the existing PDF so the deterministic output is
	// byte-for-byte identical.
	embeddedVC, vcPresent, _ := ExtractEmbeddedVC(pdf)
	var freshUpdated []byte
	if vcPresent && len(embeddedVC) > 0 {
		freshUpdated, err = UpdatePDFWithVC(ctx, original, newPayload, embeddedVC)
	} else {
		freshUpdated, err = UpdatePDF(ctx, original, newPayload)
	}
	if err != nil {
		return fmt.Errorf("re-apply amendment: %w", err)
	}
	// Similarly the submitted pdf may have additional PAdES signatures appended
	// after the dcs-pdf-core incremental update.
	if !bytes.HasPrefix(pdf, freshUpdated) {
		return fmt.Errorf("amended PDF does not match deterministic re-application of the amendment")
	}
	return nil
}
