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
	// ManifestIPFSCID is the CID under which the standalone manifest JUMBF bytes are stored.
	// This enables explicit remote-manifest retrieval (DCS-OR-C2PA-008).
	ManifestIPFSCID string
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

	// Build the signed JUMBF manifest and converge on a correct exclusion range for
	// appended incremental bytes. C2PA verifiers recompute c2pa.hash.data on the
	// final file while excluding declared byte ranges.
	manifestBytes, manifestHash, updatedPDF, err := buildAndEmbedWithExclusionConvergence(
		ctx, signer, tsaCfg, issuerDID, assertion, existingPDF, vcBytes,
	)
	if err != nil {
		return nil, err
	}
	manifestResult, err := storer.CreateFile(ctx, base64Wrap(manifestBytes))
	if err != nil {
		return nil, fmt.Errorf("store standalone manifest in IPFS: %w", err)
	}

	// Store in IPFS (DCS-OR-C2PA-008 remote manifest resilience).
	result, err := storer.CreateFile(ctx, base64Wrap(updatedPDF))
	if err != nil {
		return nil, fmt.Errorf("store updated PDF in IPFS: %w", err)
	}

	return &EmbedResult{
		UpdatedPDF:      updatedPDF,
		ManifestHash:    manifestHash,
		ManifestIPFSCID: manifestResult.Identifier.Value,
		IPFSCID:         result.Identifier.Value,
	}, nil
}

func buildAndEmbedWithExclusionConvergence(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	issuerDID string,
	assertion LifecycleAssertion,
	existingPDF []byte,
	vcBytes []byte,
) (manifestBytes []byte, manifestHash string, updatedPDF []byte, err error) {
	const maxIterations = 6
	exclusionStart := len(existingPDF)
	exclusionLength := 0

	for i := 0; i < maxIterations; i++ {
		manifestBytes, manifestHash, err = BuildManifest(
			ctx,
			signer,
			tsaCfg,
			issuerDID,
			assertion,
			exclusionStart,
			exclusionLength,
		)
		if err != nil {
			return nil, "", nil, fmt.Errorf("build C2PA manifest: %w", err)
		}

		updatedPDF, err = writeC2PAIncrement(existingPDF, manifestBytes, manifestHash, vcBytes)
		if err != nil {
			return nil, "", nil, fmt.Errorf("embed C2PA manifest in PDF: %w", err)
		}

		newExclusionLength := len(updatedPDF) - len(existingPDF)
		if newExclusionLength == exclusionLength {
			return manifestBytes, manifestHash, updatedPDF, nil
		}
		exclusionLength = newExclusionLength
	}

	return nil, "", nil, fmt.Errorf("c2pa exclusion convergence did not stabilize after %d iterations", maxIterations)
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
//
// vcBytes, when non-nil, is embedded as "contract-lifecycle-vc.json" with /AFRelationship /Data
// alongside the JUMBF manifest (DCS-FR-SM-08).
func writeC2PAIncrement(existingPDF, jumbfBytes []byte, manifestHash string, vcBytes []byte) ([]byte, error) {
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
	_ = nextObj

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

	// /Catalog/AF must reference ONLY the active (latest) manifest (C2PA PDF binding §8.2).
	// Historical manifests are discoverable via the prev_manifest_hash chain and /Names/EmbeddedFiles.
	associatedFiles := pdftypes.Array{*pdftypes.NewIndirectRef(filespecObjNum, 0)}
	if len(vcBytes) > 0 {
		associatedFiles = append(associatedFiles, *pdftypes.NewIndirectRef(vcFilespecObjNum, 0))
	}
	catDict.Update("AF", associatedFiles)

	// Ensure discoverability through /Catalog/Names/EmbeddedFiles.
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
	// Per DCS-OR-C2PA-003, each lifecycle assertion must have a corresponding VC.
	// When appending a new manifest to an existing PDF, replace the old VC reference with the new one.
	if len(vcBytes) > 0 {
		vcIndex := -1
		for i := 0; i+1 < len(embeddedNames); i += 2 {
			if key, ok := embeddedNames[i].(pdftypes.StringLiteral); ok && string(key) == "contract-lifecycle-vc.json" {
				vcIndex = i + 1 // The object reference is at i+1
				break
			}
		}
		if vcIndex >= 0 {
			// Replace the old VC reference with the new one (chained lifecycle update)
			embeddedNames[vcIndex] = *pdftypes.NewIndirectRef(vcFilespecObjNum, 0)
		} else {
			// No existing VC, append the new one
			embeddedNames = append(embeddedNames,
				pdftypes.StringLiteral("contract-lifecycle-vc.json"),
				*pdftypes.NewIndirectRef(vcFilespecObjNum, 0),
			)
		}
	}
	embeddedFilesDict.Update("Names", embeddedNames)
	namesDict.Update("EmbeddedFiles", embeddedFilesDict)
	catDict.Update("Names", namesDict)

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
// manifest payload in pdfBytes, or "" if no manifest is present.
//
// Resolution follows the C2PA PDF binding:
//   - /Catalog/AF points to the active C2PA FileSpec
//   - /FileSpec/EF/F points to the EmbeddedFile stream with the JUMBF bytes
func PrevManifestHashFrom(pdfBytes []byte) string {
	manifest := activeManifestPayloadFromPDF(pdfBytes)
	if len(manifest) == 0 {
		return ""
	}
	h := sha256.Sum256(manifest)
	return hex.EncodeToString(h[:])
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

	reAFRel := regexp.MustCompile(`/AFRelationship\s*/C2PA_Manifest`)
	for _, fileSpecObjNum := range extractAFRefObjectNumbers(catalog) {
		fileSpecObj, ok := objects[fileSpecObjNum]
		if !ok {
			continue
		}
		if !reAFRel.Match(fileSpecObj) {
			continue
		}

		streamObjNum := extractEmbeddedFileObjectNumber(fileSpecObj)
		if streamObjNum == 0 {
			continue
		}
		streamObj, ok := objects[streamObjNum]
		if !ok {
			continue
		}
		payload := extractStreamPayload(streamObj)
		if len(payload) > 0 {
			return payload
		}
	}

	return nil
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

// extractAFRefObjectNumbers returns all indirect object numbers listed in
// /Catalog/AF. Both the direct form (/AF n 0 R) and the array form
// (/AF [n 0 R m 0 R …]) are handled. The caller must inspect each returned
// filespec to find the one with /AFRelationship /C2PA_Manifest.
func extractAFRefObjectNumbers(catalogObj []byte) []int {
	// Array form: /AF [ n 0 R ... ]
	reArray := regexp.MustCompile(`/AF\s*\[(.*?)\]`)
	if m := reArray.FindSubmatch(catalogObj); len(m) == 2 {
		reRef := regexp.MustCompile(`(\d+)\s+0\s+R`)
		var nums []int
		for _, ref := range reRef.FindAllSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(string(ref[1]))
			if n > 0 {
				nums = append(nums, n)
			}
		}
		return nums
	}

	// Direct form: /AF n 0 R (single entry, no brackets)
	reDirect := regexp.MustCompile(`/AF\s*(\d+)\s+0\s+R`)
	if m := reDirect.FindSubmatch(catalogObj); len(m) == 2 {
		n, _ := strconv.Atoi(string(m[1]))
		if n > 0 {
			return []int{n}
		}
	}

	return nil
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
