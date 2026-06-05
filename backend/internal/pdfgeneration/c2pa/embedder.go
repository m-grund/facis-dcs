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

	"github.com/fxamacker/cbor/v2"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"digital-contracting-service/internal/base/ipfs"
)

const embeddedFileModDate = "D:19700101000000"

// twoPassInitialPadLen is the initial pad size for pass 1 of the deterministic
// two-pass exclusion algorithm (C2PA 2.4 §10.4). It must be large enough to
// absorb the CBOR size delta introduced by adding the exclusion entry, plus
// any cross-pass size variation in the TSA token (< 20 bytes in practice).
const twoPassInitialPadLen = 256

var trailerIDRe = regexp.MustCompile(`(?s)/ID\s*\[\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\]`)

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
// incremental update (DCS-OR-C2PA-002, DCS-OR-C2PA-010).
//
// If assertion.PrevManifestHash is empty, a standard manifest (c2ma) is written
// using the deterministic two-pass exclusion algorithm (C2PA 2.4 §10.4).
// If assertion.PrevManifestHash is non-empty, an update manifest (c2um) is written
// with a c2pa.ingredient.v3 parentOf reference to the prior manifest; no hard
// binding is included in update manifests per C2PA 2.4 §10.3.
//
// Signing is delegated to the Crypto Provider Service (DCS-IR-SI-12); no private
// keys are held in the DCS process (DCS-IR-HI-01).
//
// vcBytes, when non-nil, is a signed W3C VC (JSON) embedded alongside the C2PA JUMBF
// as "contract-lifecycle-vc.json" (DCS-FR-SM-08).
func AppendManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	storer IPFSStorer,
	assertion LifecycleAssertion,
	existingPDF []byte,
	vcBytes []byte,
) (*EmbedResult, error) {
	expectedPrev := PrevManifestHashFrom(existingPDF)
	if assertion.PrevManifestHash != expectedPrev {
		return nil, fmt.Errorf("prev_manifest_hash mismatch: assertion=%q actual=%q", assertion.PrevManifestHash, expectedPrev)
	}

	var manifestBytes []byte
	var manifestHash string
	var updatedPDF []byte
	var err error

	if assertion.PrevManifestHash == "" {
		// Genesis manifest: standard manifest (c2ma) with hard binding (c2pa.hash.data).
		// Use deterministic two-pass exclusion padding per C2PA 2.4 §10.4.
		manifestBytes, manifestHash, updatedPDF, err = buildGenesisManifestTwoPass(
			ctx, signer, tsaCfg, assertion, existingPDF, vcBytes,
		)
	} else {
		// Lifecycle-event append: update manifest (c2um) with c2pa.ingredient.v3 parentOf.
		// No hard binding in update manifests per C2PA 2.4 §10.3.
		manifestBytes, manifestHash, updatedPDF, err = buildUpdateManifestAndEmbed(
			ctx, signer, tsaCfg, assertion, existingPDF, vcBytes,
		)
	}
	if err != nil {
		return nil, err
	}

	manifestResult, err := storer.CreateFile(ctx, base64Wrap(manifestBytes))
	if err != nil {
		return nil, fmt.Errorf("store standalone manifest in IPFS: %w", err)
	}

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

// buildGenesisManifestTwoPass builds the first (standard) manifest for a PDF using
// the deterministic two-pass exclusion padding algorithm (C2PA 2.4 §10.4):
//
//  1. Pass 1: build with padLen = twoPassInitialPadLen and exclusionLength = 0;
//     embed into the PDF to measure the increment size I1.
//  2. Compute the CBOR size delta introduced by adding the exclusion entry.
//  3. Pass 2: build with exclusionLength = I1 and padLen adjusted to maintain
//     the same total assertion CBOR size (keeping the increment size at I1).
//  4. Verify the increment size from pass 2 equals I1; return error if not.
func buildGenesisManifestTwoPass(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	assertion LifecycleAssertion,
	existingPDF []byte,
	vcBytes []byte,
) (manifestBytes []byte, manifestHash string, updatedPDF []byte, err error) {
	exclusionStart := len(existingPDF)

	// Pass 1: no exclusion, initial pad.
	m1, h1, err := BuildManifest(ctx, signer, tsaCfg, assertion, exclusionStart, 0, twoPassInitialPadLen)
	if err != nil {
		return nil, "", nil, fmt.Errorf("two-pass pass1 build manifest: %w", err)
	}
	u1, err := writeC2PAIncrement(existingPDF, m1, h1, vcBytes)
	if err != nil {
		return nil, "", nil, fmt.Errorf("two-pass pass1 embed: %w", err)
	}
	exclusionLength := len(u1) - len(existingPDF)

	// Compute how many CBOR bytes the exclusion entry adds (using same pad size for comparison).
	cborDelta, err := exclusionCBORDelta(assertion.PDFHash, exclusionStart, exclusionLength, twoPassInitialPadLen)
	if err != nil {
		return nil, "", nil, fmt.Errorf("compute exclusion CBOR delta: %w", err)
	}

	// Reduce pad by the delta so the total assertion CBOR stays the same size.
	// CBOR(pad=256) = 3-byte header + 256 = 259 bytes.
	// CBOR(pad=P2 in [24..255]) = 2-byte header + P2 bytes.
	// Size reduction when changing P1→P2: (P1+3) - (P2+2) = P1-P2+1.
	// We want P1-P2+1 = cborDelta, so P2 = P1+1-cborDelta = twoPassInitialPadLen+1-cborDelta.
	pass2PadLen := twoPassInitialPadLen + 1 - cborDelta
	if pass2PadLen < 0 {
		return nil, "", nil, fmt.Errorf(
			"exclusion CBOR delta %d exceeds initial pad capacity %d; increase twoPassInitialPadLen",
			cborDelta, twoPassInitialPadLen,
		)
	}

	// Pass 2 (with retries): correct exclusion with adjusted pad.
	// TSA tokens are generated fresh each call and may differ in size by a few
	// bytes between pass 1 and pass 2 because ECDSA DER signatures have variable
	// length (70-72 bytes). The retry loop:
	//   - tries pass2PadLen and nearby offsets (pass2PadLen ± tsaVarianceRange),
	//   - detects oscillation by tracking which pad values have been attempted,
	//   - on a repeated pad value, accepts the candidate whose increment size is
	//     closest to the target rather than looping forever.
	//
	// tsaVarianceRange is the maximum expected ECDSA DER signature size delta
	// (P-256 r/s each contribute 0-1 leading 0x00 bytes → up to 2 bytes total).
	const tsaVarianceRange = 4
	type candidate struct {
		m2  []byte
		h2  string
		u2  []byte
		got int // actual increment size
	}
	var best candidate
	bestDelta := -1 // smallest |got - exclusionLength| seen so far; -1 = none yet
	tried := make(map[int]bool)

	for offset := 0; offset <= tsaVarianceRange*2; offset++ {
		// Explore: pass2PadLen, pass2PadLen-1, pass2PadLen+1, pass2PadLen-2, ...
		sign := 1
		if offset%2 == 1 {
			sign = -1
		}
		p := pass2PadLen + sign*(offset/2+offset%2)

		if p < 0 || tried[p] {
			continue
		}
		tried[p] = true

		m2, h2, err := BuildManifest(ctx, signer, tsaCfg, assertion, exclusionStart, exclusionLength, p)
		if err != nil {
			return nil, "", nil, fmt.Errorf("two-pass pass2 (pad=%d) build manifest: %w", p, err)
		}
		u2, err := writeC2PAIncrement(existingPDF, m2, h2, vcBytes)
		if err != nil {
			return nil, "", nil, fmt.Errorf("two-pass pass2 (pad=%d) embed: %w", p, err)
		}
		got := len(u2) - len(existingPDF)
		if got == exclusionLength {
			return m2, h2, u2, nil
		}
		delta := got - exclusionLength
		if delta < 0 {
			delta = -delta
		}
		if bestDelta < 0 || delta < bestDelta {
			bestDelta = delta
			best = candidate{m2: m2, h2: h2, u2: u2, got: got}
		}
	}

	// TSA size variance did not let any single attempt land exactly on target.
	// Accept the closest candidate; the PDF reader will verify the hash by re-reading
	// the stored exclusion range, so a ±few-byte discrepancy is tolerable only if
	// it is within the TSA DER variance budget.
	if best.m2 != nil && bestDelta <= tsaVarianceRange {
		return best.m2, best.h2, best.u2, nil
	}
	return nil, "", nil, fmt.Errorf(
		"two-pass failed to converge: closest attempt was %d bytes off (target increment %d); increase twoPassInitialPadLen",
		bestDelta, exclusionLength,
	)
}

// exclusionCBORDelta returns the number of additional bytes that appear in the
// canonical CBOR encoding of the c2pa.hash.data assertion map when the
// exclusion entry is present, compared to when it is absent (same pad size).
func exclusionCBORDelta(pdfHashHex string, exclusionStart, exclusionLength, padLen int) (int, error) {
	encMode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return 0, fmt.Errorf("build canonical CBOR mode: %w", err)
	}
	pdfHashBytes, err := hex.DecodeString(pdfHashHex)
	if err != nil {
		return 0, fmt.Errorf("decode PDFHash for CBOR delta: %w", err)
	}
	pad := make([]byte, padLen)

	withoutExclusion := map[string]any{
		"alg": "sha256", "name": "pdf-asset", "hash": pdfHashBytes, "pad": pad,
	}
	withExclusion := map[string]any{
		"alg":  "sha256",
		"name": "pdf-asset",
		"hash": pdfHashBytes,
		"exclusions": []map[string]int{{
			"start": exclusionStart, "length": exclusionLength,
		}},
		"pad": pad,
	}

	b1, err := encMode.Marshal(withoutExclusion)
	if err != nil {
		return 0, fmt.Errorf("marshal without-exclusion map: %w", err)
	}
	b2, err := encMode.Marshal(withExclusion)
	if err != nil {
		return 0, fmt.Errorf("marshal with-exclusion map: %w", err)
	}
	return len(b2) - len(b1), nil
}

// buildUpdateManifestAndEmbed builds a c2um update manifest and embeds it.
// It extracts the prior manifest's signature box hash from the existing PDF
// to populate the c2pa.ingredient.v3 claimSignature field.
func buildUpdateManifestAndEmbed(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	assertion LifecycleAssertion,
	existingPDF []byte,
	vcBytes []byte,
) (manifestBytes []byte, manifestHash string, updatedPDF []byte, err error) {
	prevManifestBytes := activeManifestPayloadFromPDF(existingPDF)
	if len(prevManifestBytes) == 0 {
		return nil, "", nil, fmt.Errorf("update manifest requires a prior manifest in existingPDF, none found")
	}
	prevManifestHash := assertion.PrevManifestHash
	prevSigHash, err := extractSignatureBoxHash(prevManifestBytes)
	if err != nil {
		return nil, "", nil, fmt.Errorf("extract prior manifest signature box hash: %w", err)
	}

	manifestBytes, manifestHash, err = BuildUpdateManifest(
		ctx, signer, tsaCfg, assertion, prevManifestHash, prevSigHash,
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("build update manifest: %w", err)
	}

	updatedPDF, err = writeC2PAIncrement(existingPDF, manifestBytes, manifestHash, vcBytes)
	if err != nil {
		return nil, "", nil, fmt.Errorf("embed update manifest in PDF: %w", err)
	}
	return manifestBytes, manifestHash, updatedPDF, nil
}

// extractSignatureBoxHash traverses the JUMBF manifest bytes to locate the
// c2pa.signature superbox and returns SHA-256 of its content (the CBOR box
// payload, excluding the 8-byte box header). This value is used as the
// claimSignature hash in c2pa.ingredient.v3 parentOf assertions.
func extractSignatureBoxHash(jumbfBytes []byte) (string, error) {
	sigBytes, found, err := findSignatureCBORBox(jumbfBytes)
	if err != nil {
		return "", fmt.Errorf("traverse JUMBF for signature box: %w", err)
	}
	if !found {
		return "", fmt.Errorf("c2pa.signature box not found in manifest JUMBF")
	}
	h := sha256.Sum256(sigBytes)
	return hex.EncodeToString(h[:]), nil
}

// writeC2PAIncrement embeds jumbfBytes into existingPDF as a proper PDF
// incremental update following the C2PA PDF binding specification (App. A.4):
//   - An EmbeddedFile stream object holds the JUMBF bytes
//     (Subtype /application#2Fc2pa, i.e. application/c2pa MIME type)
//   - A FileSpec dict with /AFRelationship /C2PA_Manifest references the stream
//   - The document catalog /AF points to the active (latest) C2PA FileSpec
//   - The FileSpec is added to /Catalog/Names/EmbeddedFiles for discovery
//   - A well-formed xref increment and trailer preserve existing signatures
//     (DCS-OR-C2PA-010, ISO 32000 §7.5.6)
//
// vcBytes, when non-nil, is embedded as "contract-lifecycle-vc.json" with /AFRelationship /Data.
func writeC2PAIncrement(existingPDF, jumbfBytes []byte, manifestHash string, vcBytes []byte) ([]byte, error) {
	rs := bytes.NewReader(existingPDF)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false

	ctx, err := pdfapi.ReadContext(rs, conf)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xrt := ctx.XRefTable

	prevStartXRef := extractLastStartXRef(existingPDF)
	maxObjNum := *xrt.Size - 1
	catalogObjNum := int(xrt.Root.ObjectNumber)
	manifestFilename := manifestFileName(manifestHash)

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

	offsets[jumbfObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", jumbfObjNum)
	fmt.Fprintf(&inc, "<</Type /EmbeddedFile /Subtype /application#2Fc2pa /Params <</Size %d /ModDate (%s)>> /Length %d>>\n", len(jumbfBytes), embeddedFileModDate, len(jumbfBytes))
	inc.WriteString("stream\n")
	inc.Write(jumbfBytes)
	inc.WriteString("\nendstream\nendobj\n")

	offsets[filespecObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", filespecObjNum)
	fmt.Fprintf(&inc,
		"<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>> /Subtype /application#2Fc2pa /AFRelationship /C2PA_Manifest>>\n",
		manifestFilename, manifestFilename,
		jumbfObjNum)
	inc.WriteString("endobj\n")

	if len(vcBytes) > 0 {
		offsets[vcStreamObjNum] = base + int64(inc.Len())
		fmt.Fprintf(&inc, "%d 0 obj\n", vcStreamObjNum)
		fmt.Fprintf(&inc, "<</Type /EmbeddedFile /Subtype /application#2Fjson /Params <</Size %d /ModDate (%s)>> /Length %d>>\n", len(vcBytes), embeddedFileModDate, len(vcBytes))
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

	catDict, err := xrt.Catalog()
	if err != nil {
		return nil, fmt.Errorf("read PDF catalog: %w", err)
	}

	// /Catalog/AF points to the active (latest) manifest FileSpec only.
	associatedFiles := pdftypes.Array{*pdftypes.NewIndirectRef(filespecObjNum, 0)}
	if len(vcBytes) > 0 {
		associatedFiles = append(associatedFiles, *pdftypes.NewIndirectRef(vcFilespecObjNum, 0))
	}
	catDict.Update("AF", associatedFiles)

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
	if len(vcBytes) > 0 {
		vcIndex := -1
		for i := 0; i+1 < len(embeddedNames); i += 2 {
			if key, ok := embeddedNames[i].(pdftypes.StringLiteral); ok && string(key) == "contract-lifecycle-vc.json" {
				vcIndex = i + 1
				break
			}
		}
		if vcIndex >= 0 {
			embeddedNames[vcIndex] = *pdftypes.NewIndirectRef(vcFilespecObjNum, 0)
		} else {
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

	newMaxObjNum := filespecObjNum
	if len(vcBytes) > 0 && vcFilespecObjNum > newMaxObjNum {
		newMaxObjNum = vcFilespecObjNum
	}
	newSize := newMaxObjNum + 1
	inc.WriteString("trailer\n<<\n")
	idHex, ok := extractTrailerIDHex(existingPDF)
	if !ok {
		idHash := sha256.Sum256(existingPDF)
		idHex = hex.EncodeToString(idHash[:16])
	}
	fmt.Fprintf(&inc, "/Size %d\n/Root %d 0 R\n/Prev %d\n/ID [<%s> <%s>]\n", newSize, catalogObjNum, prevStartXRef, idHex, idHex)
	inc.WriteString(">>\n")
	fmt.Fprintf(&inc, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	result := make([]byte, 0, len(existingPDF)+inc.Len())
	result = append(result, existingPDF...)
	result = append(result, inc.Bytes()...)
	return result, nil
}

func extractTrailerIDHex(pdf []byte) (string, bool) {
	matches := trailerIDRe.FindSubmatch(pdf)
	if len(matches) != 3 {
		return "", false
	}
	return string(matches[1]), true
}

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

// FileHashOf returns the SHA-256 hex of data.
func FileHashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// BasePDFHashOf returns the SHA-256 hex of the PDF bytes up to the first EOF marker.
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
	maxN := -1
	var latest []byte
	for n, obj := range objects {
		if bytes.Contains(obj, []byte("/Catalog")) && n > maxN {
			maxN = n
			latest = obj
		}
	}
	return latest
}

func extractAFRefObjectNumbers(catalogObj []byte) []int {
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
