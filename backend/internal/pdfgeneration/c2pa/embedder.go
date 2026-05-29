package c2pa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"digital-contracting-service/internal/base/ipfs"
)

// IPFSStorer stores bytes and returns their CID.
type IPFSStorer interface {
	CreateFile(ctx context.Context, data any) (*ipfs.IPFSResult, error)
}

// EmbedResult is returned by AppendManifest.
type EmbedResult struct {
	// UpdatedPDF is the PDF with the C2PA manifest appended as an incremental update.
	UpdatedPDF []byte
	// ManifestHash is the SHA-256 of the manifest JUMBF bytes (for prev_manifest_hash chaining).
	ManifestHash string
	// IPFSCID is the CID under which the updated PDF was stored (DCS-OR-C2PA-008).
	IPFSCID string
}

// AppendManifest appends a C2PA lifecycle assertion to existingPDF as a PDF
// incremental update (DCS-OR-C2PA-002, DCS-OR-C2PA-010). The JUMBF manifest
// is embedded as a proper PDF EmbeddedFile stream with AFRelationship /C2PA_Manifest
// so that standard C2PA tools (Acrobat, c2patool) can verify the provenance chain.
//
// Signing is delegated to the Crypto Provider Service (DCS-IR-SI-12); no private
// keys are held in the DCS process (DCS-IR-HI-01).
//
// vcBytes, when non-nil, is a signed W3C VC (JSON) that is embedded alongside
// the C2PA JUMBF as "contract-lifecycle-vc.json" (DCS-FR-SM-08).
func AppendManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	storer IPFSStorer,
	issuerDID string,
	assertion LifecycleAssertion,
	existingPDF []byte,
	vcBytes []byte,
) (*EmbedResult, error) {
	expectedPrev := PrevManifestHashFrom(existingPDF)
	if assertion.PrevManifestHash != expectedPrev {
		return nil, fmt.Errorf("prev_manifest_hash mismatch: assertion=%q actual=%q", assertion.PrevManifestHash, expectedPrev)
	}

	// Build the signed JUMBF manifest.
	manifestBytes, manifestHash, err := BuildManifest(ctx, signer, tsaCfg, issuerDID, assertion)
	if err != nil {
		return nil, fmt.Errorf("build C2PA manifest: %w", err)
	}

	// Embed the manifest as a proper PDF incremental update per the C2PA PDF binding spec.
	updatedPDF, err := writeC2PAIncrement(existingPDF, manifestBytes, manifestHash, vcBytes)
	if err != nil {
		return nil, fmt.Errorf("embed C2PA manifest in PDF: %w", err)
	}

	// Store in IPFS (DCS-OR-C2PA-008 remote manifest resilience).
	result, err := storer.CreateFile(ctx, base64Wrap(updatedPDF))
	if err != nil {
		return nil, fmt.Errorf("store updated PDF in IPFS: %w", err)
	}

	return &EmbedResult{
		UpdatedPDF:   updatedPDF,
		ManifestHash: manifestHash,
		IPFSCID:      result.Identifier.Value,
	}, nil
}

// writeC2PAIncrement embeds jumbfBytes into existingPDF as a proper PDF
// incremental update following the C2PA PDF binding specification:
//   - An EmbeddedFile stream object holds the JUMBF bytes
//     (Subtype /application#2Fc2pa, i.e. application/c2pa MIME type)
//   - A FileSpec dict with /AFRelationship /C2PA_Manifest references the stream
//   - The document catalog is updated with /AF pointing to the FileSpec
//   - The FileSpec is referenced from /Catalog/Names/EmbeddedFiles for discovery
//   - A well-formed xref increment and trailer preserve existing signatures
//     (DCS-OR-C2PA-010, ISO 32000 §7.5.6)
// vcBytes, when non-nil, is embedded as "contract-lifecycle-vc.json" with /AFRelationship /Data
// alongside the JUMBF manifest (DCS-FR-SM-08).
func writeC2PAIncrement(existingPDF, jumbfBytes []byte, manifestHash string, vcBytes []byte) ([]byte, error) {
	useFileAttachment := hasCertifyingSignature(existingPDF)

	// Parse the PDF to access the xref table and catalog.
	rs := bytes.NewReader(existingPDF)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false // classic xref tables; simpler to write manually

	ctx, err := pdfapi.ReadContext(rs, conf)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xrt := ctx.XRefTable

	prevStartXRef := extractLastStartXRef(existingPDF)
	maxObjNum := *xrt.Size - 1
	catalogObjNum := int(xrt.Root.ObjectNumber)
	manifestFilename := manifestFileName(manifestHash)

	// Assign new object numbers.
	jumbfObjNum := maxObjNum + 1
	filespecObjNum := maxObjNum + 2
	vcStreamObjNum := 0
	vcFilespecObjNum := 0
	nextObj := maxObjNum + 3
	if len(vcBytes) > 0 {
		vcStreamObjNum = nextObj
		vcFilespecObjNum = nextObj + 1
		nextObj += 2
	}
	annotObjNum := 0
	pageObjNum := 0
	if useFileAttachment {
		annotObjNum = nextObj
	}

	base := int64(len(existingPDF))
	offsets := map[int]int64{}
	var inc bytes.Buffer

	// --- Object: EmbeddedFile stream (C2PA JUMBF) ----------------------------
	offsets[jumbfObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", jumbfObjNum)
	fmt.Fprintf(&inc, "<</Type /EmbeddedFile /Subtype /application#2Fc2pa /Params <</Size %d>> /Length %d>>\n", len(jumbfBytes), len(jumbfBytes))
	inc.WriteString("stream\n")
	inc.Write(jumbfBytes)
	inc.WriteString("\nendstream\nendobj\n")

	// --- Object: FileSpec dict (C2PA manifest) --------------------------------
	offsets[filespecObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", filespecObjNum)
	fmt.Fprintf(&inc,
		"<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>> /Subtype /application#2Fc2pa /AFRelationship /C2PA_Manifest>>\n",
		manifestFilename, manifestFilename,
		jumbfObjNum)
	inc.WriteString("endobj\n")

	// --- Objects: W3C VC EmbeddedFile + FileSpec (DCS-FR-SM-08) --------------
	if len(vcBytes) > 0 {
		offsets[vcStreamObjNum] = base + int64(inc.Len())
		fmt.Fprintf(&inc, "%d 0 obj\n", vcStreamObjNum)
		fmt.Fprintf(&inc, "<</Type /EmbeddedFile /Subtype /application#2Fjson /Params <</Size %d>> /Length %d>>\n", len(vcBytes), len(vcBytes))
		inc.WriteString("stream\n")
		inc.Write(vcBytes)
		inc.WriteString("\nendstream\nendobj\n")

		offsets[vcFilespecObjNum] = base + int64(inc.Len())
		fmt.Fprintf(&inc, "%d 0 obj\n", vcFilespecObjNum)
		fmt.Fprintf(&inc,
			"<</Type /Filespec /F (contract-lifecycle-vc.json) /UF (contract-lifecycle-vc.json) /EF <</F %d 0 R>> /AFRelationship /Data>>\n",
			vcStreamObjNum)
		inc.WriteString("endobj\n")
	}

	// --- Object: updated catalog (AF array append) ---------------------------
	catDict, err := xrt.Catalog()
	if err != nil {
		return nil, fmt.Errorf("read PDF catalog: %w", err)
	}

	// Per C2PA PDF binding, /Catalog/AF must point at the active manifest FileSpec.
	var associatedFiles pdftypes.Array
	if existing, ok := catDict["AF"]; ok {
		if arr, err2 := xrt.DereferenceArray(existing); err2 == nil {
			associatedFiles = arr
		}
	}
	associatedFiles = append(associatedFiles, *pdftypes.NewIndirectRef(filespecObjNum, 0))
	if len(vcBytes) > 0 {
		associatedFiles = append(associatedFiles, *pdftypes.NewIndirectRef(vcFilespecObjNum, 0))
	}
	catDict.Update("AF", associatedFiles)

	if useFileAttachment {
		// For certifying-signature PDFs (DocMDP), use the FileAttachment annotation
		// reference path instead of touching /Catalog/Names/EmbeddedFiles.
		if err := xrt.EnsurePageCount(); err != nil {
			return nil, fmt.Errorf("ensure page count for FileAttachment path: %w", err)
		}
		pageDict, pageIndRef, _, err := xrt.PageDict(1, false)
		if err != nil {
			return nil, fmt.Errorf("read first page dict for FileAttachment: %w", err)
		}
		if pageIndRef == nil {
			return nil, fmt.Errorf("first page indirect reference is missing")
		}

		annotDict := pdftypes.Dict{
			"Type":     pdftypes.Name("Annot"),
			"Subtype":  pdftypes.Name("FileAttachment"),
			"Rect":     pdftypes.Array{pdftypes.Integer(0), pdftypes.Integer(0), pdftypes.Integer(0), pdftypes.Integer(0)},
			"FS":       *pdftypes.NewIndirectRef(filespecObjNum, 0),
			"Name":     pdftypes.Name("PushPin"),
			"Contents": pdftypes.StringLiteral("C2PA Manifest Store"),
		}

		offsets[annotObjNum] = base + int64(inc.Len())
		fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", annotObjNum, annotDict.PDFString())

		var annots pdftypes.Array
		if existing, ok := pageDict["Annots"]; ok {
			if arr, err2 := xrt.DereferenceArray(existing); err2 == nil {
				annots = arr
			}
		}
		annots = append(annots, *pdftypes.NewIndirectRef(annotObjNum, 0))
		pageDict.Update("Annots", annots)

		pageObjNum = int(pageIndRef.ObjectNumber)
		offsets[pageObjNum] = base + int64(inc.Len())
		fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", pageObjNum, pageDict.PDFString())
	} else {
		// Ensure discoverability through /Catalog/Names/EmbeddedFiles as required by
		// the C2PA PDF binding for document-level manifests.
		var namesDict pdftypes.Dict
		if existing, ok := catDict["Names"]; ok {
			if d, err2 := xrt.DereferenceDict(existing); err2 == nil {
				namesDict = d
			}
		}
		if namesDict == nil {
			namesDict = pdftypes.Dict{}
		}

		var embeddedFilesDict pdftypes.Dict
		if existing, ok := namesDict["EmbeddedFiles"]; ok {
			if d, err2 := xrt.DereferenceDict(existing); err2 == nil {
				embeddedFilesDict = d
			}
		}
		if embeddedFilesDict == nil {
			embeddedFilesDict = pdftypes.Dict{}
		}

		var embeddedNames pdftypes.Array
		if existing, ok := embeddedFilesDict["Names"]; ok {
			if arr, err2 := xrt.DereferenceArray(existing); err2 == nil {
				embeddedNames = arr
			}
		}
		if len(embeddedNames)%2 != 0 {
			// Guard against malformed legacy arrays so we never emit an invalid NameTree.
			embeddedNames = embeddedNames[:len(embeddedNames)-1]
		}

		alreadyPresent := false
		for i := 0; i+1 < len(embeddedNames); i += 2 {
			if key, ok := embeddedNames[i].(pdftypes.StringLiteral); ok && string(key) == manifestFilename {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			embeddedNames = append(embeddedNames,
				pdftypes.StringLiteral(manifestFilename),
				*pdftypes.NewIndirectRef(filespecObjNum, 0),
			)
		}
		// Add VC to EmbeddedFiles name tree so PDF readers can discover it (DCS-FR-SM-08).
		if len(vcBytes) > 0 {
			vcPresent := false
			for i := 0; i+1 < len(embeddedNames); i += 2 {
				if key, ok := embeddedNames[i].(pdftypes.StringLiteral); ok && string(key) == "contract-lifecycle-vc.json" {
					vcPresent = true
					break
				}
			}
			if !vcPresent {
				embeddedNames = append(embeddedNames,
					pdftypes.StringLiteral("contract-lifecycle-vc.json"),
					*pdftypes.NewIndirectRef(vcFilespecObjNum, 0),
				)
			}
		}
		embeddedFilesDict.Update("Names", embeddedNames)
		namesDict.Update("EmbeddedFiles", embeddedFilesDict)
		catDict.Update("Names", namesDict)
	}

	offsets[catalogObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", catalogObjNum, catDict.PDFString())

	// --- xref increment -------------------------------------------------------
	xrefOffset := base + int64(inc.Len())
	inc.WriteString("xref\n")

	allObjSet := map[int]struct{}{
		jumbfObjNum:    {},
		filespecObjNum: {},
		catalogObjNum:  {},
	}
	if len(vcBytes) > 0 {
		allObjSet[vcStreamObjNum] = struct{}{}
		allObjSet[vcFilespecObjNum] = struct{}{}
	}
	if useFileAttachment {
		allObjSet[annotObjNum] = struct{}{}
		if pageObjNum > 0 {
			allObjSet[pageObjNum] = struct{}{}
		}
	}
	allObjs := make([]int, 0, len(allObjSet))
	for n := range allObjSet {
		allObjs = append(allObjs, n)
	}
	sort.Ints(allObjs)

	// Write consecutive runs as subsections (PDF §7.5.4).
	for i := 0; i < len(allObjs); {
		j := i + 1
		for j < len(allObjs) && allObjs[j] == allObjs[j-1]+1 {
			j++
		}
		fmt.Fprintf(&inc, "%d %d\n", allObjs[i], j-i)
		for _, n := range allObjs[i:j] {
			fmt.Fprintf(&inc, "%010d 00000 n \n", offsets[n])
		}
		i = j
	}

	// --- Trailer and startxref -----------------------------------------------
	newMaxObjNum := filespecObjNum
	if len(vcBytes) > 0 && vcFilespecObjNum > newMaxObjNum {
		newMaxObjNum = vcFilespecObjNum
	}
	if useFileAttachment && annotObjNum > newMaxObjNum {
		newMaxObjNum = annotObjNum
	}
	newSize := newMaxObjNum + 1
	inc.WriteString("trailer\n<<\n")
	fmt.Fprintf(&inc, "/Size %d\n/Root %d 0 R\n/Prev %d\n", newSize, catalogObjNum, prevStartXRef)
	inc.WriteString(">>\n")
	fmt.Fprintf(&inc, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	result := make([]byte, 0, len(existingPDF)+inc.Len())
	result = append(result, existingPDF...)
	result = append(result, inc.Bytes()...)
	return result, nil
}

// extractLastStartXRef returns the numeric value of the last startxref keyword
// in the PDF, which becomes the /Prev entry in the incremental trailer.
func extractLastStartXRef(pdf []byte) int64 {
	kw := []byte("startxref")
	last := -1
	for i := 0; i <= len(pdf)-len(kw); i++ {
		if bytes.Equal(pdf[i:i+len(kw)], kw) {
			last = i
		}
	}
	if last == -1 {
		return 0
	}
	rest := bytes.TrimSpace(pdf[last+len(kw):])
	end := bytes.IndexAny(rest, " \t\r\n%")
	if end < 0 {
		end = len(rest)
	}
	v, _ := strconv.ParseInt(string(rest[:end]), 10, 64)
	return v
}

func manifestFileName(manifestHash string) string {
	if manifestHash == "" {
		return "c2pa_manifest.c2pa"
	}
	return "c2pa_manifest_" + manifestHash + ".c2pa"
}

func hasCertifyingSignature(pdfBytes []byte) bool {
	// ISO 32000 DocMDP indicates a certifying-signature permissions dictionary.
	// C2PA PDF binding requires FileAttachment annotation approach in this case.
	return bytes.Contains(pdfBytes, []byte("/DocMDP"))
}

// FileHashOf returns the SHA-256 hex of data, used for LifecycleAssertion.FileHash.
func FileHashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// BasePDFHashOf returns the SHA-256 hex of the PDF bytes up to the first EOF marker.
// This excludes incremental updates such as appended C2PA manifest stores.
func BasePDFHashOf(pdfBytes []byte) string {
	eofMarker := []byte("%%EOF")
	idx := bytes.Index(pdfBytes, eofMarker)
	if idx == -1 {
		return FileHashOf(pdfBytes)
	}
	end := idx + len(eofMarker)
	for end < len(pdfBytes) && (pdfBytes[end] == '\n' || pdfBytes[end] == '\r') {
		end++
	}
	return FileHashOf(pdfBytes[:end])
}

// PrevManifestHashFrom returns the SHA-256 hex of the active embedded C2PA
// manifest payload in pdfBytes, or "" if none exists.
//
// The primary source is the C2PA PDF binding metadata:
//   - /Catalog/AF points to the active C2PA FileSpec
//   - /FileSpec/EF/F points to the EmbeddedFile stream with C2PA bytes
//
// For backward compatibility with older DCS-generated PDFs, a fallback parser
// still reads the legacy private comment marker.
func PrevManifestHashFrom(pdfBytes []byte) string {
	if manifest := activeManifestPayloadFromPDF(pdfBytes); len(manifest) > 0 {
		h := sha256.Sum256(manifest)
		return hex.EncodeToString(h[:])
	}

	// Legacy fallback: older PDFs used a private comment marker after %%EOF.
	marker := []byte("%% DCS-C2PA-HASH: ")
	last := -1
	searchFrom := 0
	for {
		idx := bytes.Index(pdfBytes[searchFrom:], marker)
		if idx == -1 {
			break
		}
		last = searchFrom + idx
		searchFrom = last + len(marker)
	}
	if last == -1 {
		return ""
	}
	start := last + len(marker)
	end := bytes.IndexAny(pdfBytes[start:], "\r\n")
	if end < 0 {
		end = len(pdfBytes) - start
	}
	return strings.TrimSpace(string(pdfBytes[start : start+end]))
}

func activeManifestPayloadFromPDF(pdfBytes []byte) []byte {
	objects := parsePDFObjectsByNumber(pdfBytes)
	if len(objects) == 0 {
		return nil
	}

	catalog := findLatestCatalogObject(objects)
	if catalog == nil {
		return nil
	}

	fileSpecObjNum := extractActiveAFRefObjectNumber(catalog)
	if fileSpecObjNum == 0 {
		return nil
	}

	fileSpecObj, ok := objects[fileSpecObjNum]
	if !ok {
		return nil
	}
	reAFRel := regexp.MustCompile(`/AFRelationship\s*/C2PA_Manifest`)
	if !reAFRel.Match(fileSpecObj) {
		return nil
	}

	streamObjNum := extractEmbeddedFileObjectNumber(fileSpecObj)
	if streamObjNum == 0 {
		return nil
	}

	streamObj, ok := objects[streamObjNum]
	if !ok {
		return nil
	}

	payload := extractStreamPayload(streamObj)
	if len(payload) == 0 {
		return nil
	}

	return payload
}

func parsePDFObjectsByNumber(pdf []byte) map[int][]byte {
	re := regexp.MustCompile(`(?s)(\d+)\s+0\s+obj\b(.*?)\bendobj`)
	objects := make(map[int][]byte)
	for _, m := range re.FindAllSubmatch(pdf, -1) {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			continue
		}
		objects[n] = m[2]
	}
	return objects
}

func findLatestCatalogObject(objects map[int][]byte) []byte {
	for _, obj := range objects {
		if bytes.Contains(obj, []byte("/Catalog")) {
			return obj
		}
	}
	return nil
}

func extractActiveAFRefObjectNumber(catalogObj []byte) int {
	// Preferred form in current writer: /AF n 0 R
	reDirect := regexp.MustCompile(`/AF\s*(\d+)\s+0\s+R`)
	if m := reDirect.FindSubmatch(catalogObj); len(m) == 2 {
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}

	// Backward compatibility: /AF [ ... n 0 R ]
	reArray := regexp.MustCompile(`/AF\s*\[(.*?)\]`)
	if m := reArray.FindSubmatch(catalogObj); len(m) == 2 {
		reRef := regexp.MustCompile(`(\d+)\s+0\s+R`)
		refs := reRef.FindAllSubmatch(m[1], -1)
		if len(refs) > 0 {
			n, _ := strconv.Atoi(string(refs[len(refs)-1][1]))
			return n
		}
	}

	return 0
}

func extractEmbeddedFileObjectNumber(fileSpecObj []byte) int {
	re := regexp.MustCompile(`/EF\s*<<\s*/F\s*(\d+)\s+0\s+R\s*>>`)
	if m := re.FindSubmatch(fileSpecObj); len(m) == 2 {
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}

	reAlt := regexp.MustCompile(`/EF\s*<</F\s*(\d+)\s+0\s+R>>`)
	m := reAlt.FindSubmatch(fileSpecObj)
	if len(m) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

func extractStreamPayload(streamObj []byte) []byte {
	re := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	m := re.FindSubmatch(streamObj)
	if len(m) != 2 {
		return nil
	}
	return m[1]
}

// base64Wrap wraps raw bytes for IPFS CreateFile which expects JSON-serialisable input.
type base64Wrap []byte

func (b base64Wrap) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, 2+len(b)*4/3+4)
	out = append(out, '"')
	enc := make([]byte, ((len(b)+2)/3)*4)
	n := encodeBase64(enc, b)
	out = append(out, enc[:n]...)
	out = append(out, '"')
	return out, nil
}

func encodeBase64(dst, src []byte) int {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	n := 0
	for i := 0; i < len(src); i += 3 {
		rem := len(src) - i
		var b0, b1, b2 byte
		b0 = src[i]
		if rem > 1 {
			b1 = src[i+1]
		}
		if rem > 2 {
			b2 = src[i+2]
		}
		dst[n] = table[b0>>2]
		dst[n+1] = table[((b0&0x3)<<4)|(b1>>4)]
		if rem > 1 {
			dst[n+2] = table[((b1&0xf)<<2)|(b2>>6)]
		} else {
			dst[n+2] = '='
		}
		if rem > 2 {
			dst[n+3] = table[b2&0x3f]
		} else {
			dst[n+3] = '='
		}
		n += 4
	}
	return n
}
