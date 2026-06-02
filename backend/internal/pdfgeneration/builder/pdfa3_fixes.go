package builder

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

var xrefInUseLineRe = regexp.MustCompile(`^(\d{10}) (\d{5}) n\s*$`)
var objLineRe = regexp.MustCompile(`(?m)^(\d+)\s+0\s+obj\b`)

// fixPDFA3 applies deterministic post-processing fixes needed for PDF/A-3 checks.
func fixPDFA3(pdf []byte) []byte {
	if len(pdf) == 0 {
		return pdf
	}

	idSeed := append([]byte(nil), pdf...)
	withBinaryHeader := ensureBinaryHeaderComment(pdf)
	fixed, err := appendPDFA3Increment(withBinaryHeader, idSeed)
	if err != nil {
		return withBinaryHeader
	}
	return fixed
}

func ensureBinaryHeaderComment(pdf []byte) []byte {
	if bytes.HasPrefix(pdf, []byte("%PDF-1.3")) {
		patched := append([]byte(nil), pdf...)
		copy(patched[:8], []byte("%PDF-1.7"))
		pdf = patched
	}

	firstNL := bytes.IndexByte(pdf, '\n')
	if firstNL < 0 || firstNL+1 >= len(pdf) {
		return pdf
	}

	lineEnd := bytes.IndexByte(pdf[firstNL+1:], '\n')
	if lineEnd >= 0 {
		secondLine := pdf[firstNL+1 : firstNL+1+lineEnd]
		if len(secondLine) >= 5 && secondLine[0] == '%' && secondLine[1] > 127 && secondLine[2] > 127 && secondLine[3] > 127 && secondLine[4] > 127 {
			return pdf
		}
	}

	insert := []byte{'%', 0xe2, 0xe3, 0xcf, 0xd3, '\n'}
	insertPos := firstNL + 1
	oldStart := extractLastStartXRefLocal(pdf)
	if oldStart <= 0 {
		return pdf
	}

	out := make([]byte, 0, len(pdf)+len(insert))
	out = append(out, pdf[:insertPos]...)
	out = append(out, insert...)
	out = append(out, pdf[insertPos:]...)

	delta := int64(len(insert))
	newStart := oldStart + delta
	if !patchClassicXRefOffsets(out, int(newStart), delta) {
		return pdf
	}
	if !patchLastStartXRef(out, newStart) {
		return pdf
	}
	return out
}

func appendPDFA3Increment(pdf []byte, idSeed []byte) ([]byte, error) {
	rs := bytes.NewReader(pdf)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false

	ctx, err := pdfapi.ReadContext(rs, conf)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xrt := ctx.XRefTable

	prevStartXRef := extractLastStartXRefLocal(pdf)
	if prevStartXRef <= 0 {
		return nil, fmt.Errorf("missing startxref")
	}

	maxObjNum := *xrt.Size - 1
	catalogObjNum := int(xrt.Root.ObjectNumber)
	metadataObjNum := findObjectNumberByMarker(pdf, []byte("/Type /Metadata"))
	fileSpecObjNum, embeddedObjNum, err := findPrimaryAttachmentObjectNumbers(xrt)
	if err != nil {
		return nil, err
	}

	iccObjNum := maxObjNum + 1
	outputIntentObjNum := maxObjNum + 2

	catDict, err := xrt.Catalog()
	if err != nil {
		return nil, fmt.Errorf("read PDF catalog: %w", err)
	}
	catDict.Update("Type", pdftypes.Name("Catalog"))
	catDict.Update("OutputIntents", pdftypes.Array{*pdftypes.NewIndirectRef(outputIntentObjNum, 0)})
	if metadataObjNum > 0 {
		catDict.Update("Metadata", *pdftypes.NewIndirectRef(metadataObjNum, 0))
	}
	catDict.Update("AF", pdftypes.Array{*pdftypes.NewIndirectRef(fileSpecObjNum, 0)})

	fileSpecDict, err := dereferenceDictByObjNum(xrt, fileSpecObjNum)
	if err != nil {
		return nil, err
	}
	fileSpecDict.Update("AFRelationship", pdftypes.Name("Data"))
	fileSpecDict.Update("Subtype", pdftypes.Name("application#2Fjson"))

	embeddedStreamDict, err := dereferenceStreamDictByObjNum(xrt, embeddedObjNum)
	if err != nil {
		return nil, err
	}
	embeddedStreamDict.Dict.Update("Subtype", pdftypes.Name("application#2Fjson"))

	streamPayload := extractRawStreamPayload(pdf, embeddedObjNum)
	if len(streamPayload) == 0 {
		return nil, fmt.Errorf("embedded stream payload not found")
	}
	embeddedStreamDict.Dict.Update("Params", pdftypes.Dict{
		"Size":    pdftypes.Integer(len(streamPayload)),
		"ModDate": pdftypes.StringLiteral(pdftypes.DateString(epochTime)),
	})

	idHash := sha256.Sum256(idSeed)
	idHex := hex.EncodeToString(idHash[:16])
	icc := buildSRGBICC()

	base := int64(len(pdf))
	offsets := map[int]int64{}
	var inc bytes.Buffer

	offsets[iccObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", iccObjNum)
	fmt.Fprintf(&inc, "<</N 3 /Alternate /DeviceRGB /Length %d>>\n", len(icc))
	inc.WriteString("stream\n")
	inc.Write(icc)
	inc.WriteString("\nendstream\nendobj\n")

	offsets[outputIntentObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", outputIntentObjNum)
	fmt.Fprintf(&inc, "<</Type /OutputIntent /S /GTS_PDFA1 /OutputConditionIdentifier (sRGB IEC61966-2.1) /Info (sRGB IEC61966-2.1) /DestOutputProfile %d 0 R>>\n", iccObjNum)
	inc.WriteString("endobj\n")

	offsets[catalogObjNum] = base + int64(inc.Len())
	catStr := canonicalizeDictString(catDict.PDFString())
	fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", catalogObjNum, catStr)

	offsets[fileSpecObjNum] = base + int64(inc.Len())
	fileSpecStr := canonicalizeDictString(fileSpecDict.PDFString())
	fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", fileSpecObjNum, fileSpecStr)

	offsets[embeddedObjNum] = base + int64(inc.Len())
	embeddedStr := canonicalizeDictString(embeddedStreamDict.Dict.PDFString())
	fmt.Fprintf(&inc, "%d 0 obj\n%s\n", embeddedObjNum, embeddedStr)
	inc.WriteString("stream\n")
	inc.Write(streamPayload)
	inc.WriteString("\nendstream\nendobj\n")

	xrefOffset := base + int64(inc.Len())
	inc.WriteString("xref\n")

	objs := make([]int, 0, len(offsets))
	for objNum := range offsets {
		objs = append(objs, objNum)
	}
	sort.Ints(objs)

	for i := 0; i < len(objs); {
		j := i + 1
		for j < len(objs) && objs[j] == objs[j-1]+1 {
			j++
		}
		fmt.Fprintf(&inc, "%d %d\n", objs[i], j-i)
		for _, objNum := range objs[i:j] {
			fmt.Fprintf(&inc, "%010d 00000 n \n", offsets[objNum])
		}
		i = j
	}

	newMax := outputIntentObjNum
	inc.WriteString("trailer\n<<\n")
	fmt.Fprintf(&inc, "/Size %d\n/Root %d 0 R\n/Prev %d\n/ID [<%s> <%s>]\n", newMax+1, catalogObjNum, prevStartXRef, idHex, idHex)
	inc.WriteString(">>\n")
	fmt.Fprintf(&inc, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	out := make([]byte, 0, len(pdf)+inc.Len())
	out = append(out, pdf...)
	out = append(out, inc.Bytes()...)
	return out, nil
}

func patchClassicXRefOffsets(pdf []byte, xrefOffset int, delta int64) bool {
	if xrefOffset < 0 || xrefOffset >= len(pdf) {
		return false
	}
	if !bytes.HasPrefix(pdf[xrefOffset:], []byte("xref\n")) && !bytes.HasPrefix(pdf[xrefOffset:], []byte("xref\r\n")) {
		return false
	}
	trailerRel := bytes.Index(pdf[xrefOffset:], []byte("trailer"))
	if trailerRel < 0 {
		return false
	}
	sectionStart := xrefOffset
	sectionEnd := xrefOffset + trailerRel
	lines := bytes.Split(pdf[sectionStart:sectionEnd], []byte("\n"))
	for i, line := range lines {
		m := xrefInUseLineRe.FindSubmatch(bytes.TrimRight(line, "\r"))
		if len(m) != 3 {
			continue
		}
		off, err := strconv.ParseInt(string(m[1]), 10, 64)
		if err != nil {
			return false
		}
		lines[i] = []byte(fmt.Sprintf("%010d %s n ", off+delta, m[2]))
	}
	rebuilt := bytes.Join(lines, []byte("\n"))
	if len(rebuilt) != sectionEnd-sectionStart {
		return false
	}
	copy(pdf[sectionStart:sectionEnd], rebuilt)
	return true
}

func patchLastStartXRef(pdf []byte, newStart int64) bool {
	idx := bytes.LastIndex(pdf, []byte("startxref"))
	if idx < 0 {
		return false
	}
	i := idx + len("startxref")
	for i < len(pdf) && (pdf[i] == ' ' || pdf[i] == '\t' || pdf[i] == '\r' || pdf[i] == '\n') {
		i++
	}
	j := i
	for j < len(pdf) && pdf[j] >= '0' && pdf[j] <= '9' {
		j++
	}
	if i == j {
		return false
	}
	repl := []byte(strconv.FormatInt(newStart, 10))
	if len(repl) != j-i {
		return false
	}
	copy(pdf[i:j], repl)
	return true
}

func findObjectNumberByMarker(pdf []byte, marker []byte) int {
	matches := objLineRe.FindAllSubmatchIndex(pdf, -1)
	for i := 0; i < len(matches); i++ {
		start := matches[i][0]
		objNumStart := matches[i][2]
		objNumEnd := matches[i][3]
		end := len(pdf)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		if bytes.Contains(pdf[start:end], marker) {
			n, err := strconv.Atoi(string(pdf[objNumStart:objNumEnd]))
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func findPrimaryAttachmentObjectNumbers(xrt *model.XRefTable) (int, int, error) {
	cat, err := xrt.Catalog()
	if err != nil {
		return 0, 0, fmt.Errorf("read catalog: %w", err)
	}
	namesObj, ok := cat["Names"]
	if !ok {
		return 0, 0, fmt.Errorf("catalog has no /Names")
	}
	namesDict, err := xrt.DereferenceDict(namesObj)
	if err != nil {
		return 0, 0, fmt.Errorf("dereference names dict: %w", err)
	}
	embObj, ok := namesDict["EmbeddedFiles"]
	if !ok {
		return 0, 0, fmt.Errorf("names has no /EmbeddedFiles")
	}
	embDict, err := xrt.DereferenceDict(embObj)
	if err != nil {
		return 0, 0, fmt.Errorf("dereference embedded files dict: %w", err)
	}
	namesArrObj, ok := embDict["Names"]
	if !ok {
		return 0, 0, fmt.Errorf("embedded files has no /Names array")
	}
	namesArr, err := xrt.DereferenceArray(namesArrObj)
	if err != nil {
		return 0, 0, fmt.Errorf("dereference embedded names array: %w", err)
	}
	for i := 0; i+1 < len(namesArr); i += 2 {
		fileSpecObjNum := indirectObjNumber(namesArr[i+1])
		if fileSpecObjNum == 0 {
			continue
		}
		fsRef := pdftypes.NewIndirectRef(fileSpecObjNum, 0)
		fsDict, err := xrt.DereferenceDict(*fsRef)
		if err != nil {
			continue
		}
		efObj, ok := fsDict["EF"]
		if !ok {
			continue
		}
		efDict, err := xrt.DereferenceDict(efObj)
		if err != nil {
			continue
		}
		embObjRef, ok := efDict["F"]
		if !ok {
			continue
		}
		embObjNum := indirectObjNumber(embObjRef)
		if embObjNum == 0 {
			continue
		}
		return fileSpecObjNum, embObjNum, nil
	}
	return 0, 0, fmt.Errorf("attachment filespec/embeddedfile not found")
}

func indirectObjNumber(obj pdftypes.Object) int {
	switch v := obj.(type) {
	case pdftypes.IndirectRef:
		return int(v.ObjectNumber)
	case *pdftypes.IndirectRef:
		if v == nil {
			return 0
		}
		return int(v.ObjectNumber)
	default:
		return 0
	}
}

func extractRawStreamPayload(pdf []byte, objNum int) []byte {
	if objNum <= 0 {
		return nil
	}
	objRE := regexp.MustCompile(fmt.Sprintf(`(?ms)^%d 0 obj\s*(.*?)\nendobj`, objNum))
	objMatch := objRE.FindSubmatch(pdf)
	if len(objMatch) < 2 {
		return nil
	}
	streamRE := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	streamMatch := streamRE.FindSubmatch(objMatch[1])
	if len(streamMatch) < 2 {
		return nil
	}
	out := make([]byte, len(streamMatch[1]))
	copy(out, streamMatch[1])
	return out
}

func extractLastStartXRefLocal(pdf []byte) int64 {
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

func dereferenceDictByObjNum(xrt *model.XRefTable, objNum int) (pdftypes.Dict, error) {
	ref := pdftypes.NewIndirectRef(objNum, 0)
	d, err := xrt.DereferenceDict(*ref)
	if err != nil {
		return nil, fmt.Errorf("dereference dict object %d: %w", objNum, err)
	}
	return d, nil
}

func dereferenceStreamDictByObjNum(xrt *model.XRefTable, objNum int) (*pdftypes.StreamDict, error) {
	ref := pdftypes.NewIndirectRef(objNum, 0)
	sd, _, err := xrt.DereferenceStreamDict(*ref)
	if err != nil {
		return nil, fmt.Errorf("dereference stream object %d: %w", objNum, err)
	}
	return sd, nil
}

// buildSRGBICC returns a deterministic 476-byte ICC v2 profile for sRGB.
func buildSRGBICC() []byte {
	const profileSize = 476
	b := make([]byte, profileSize)

	putU32 := func(off int, v uint32) {
		binary.BigEndian.PutUint32(b[off:off+4], v)
	}
	putSig := func(off int, sig string) {
		copy(b[off:off+4], []byte(sig))
	}
	putXYZ := func(off int, x, y, z uint32) {
		putU32(off, x)
		putU32(off+4, y)
		putU32(off+8, z)
	}

	putU32(0, profileSize)
	putU32(8, 0x02100000)
	putSig(12, "mntr")
	putSig(16, "RGB ")
	putSig(20, "XYZ ")
	putSig(36, "acsp")
	putXYZ(68, 63189, 65536, 54074)

	tags := []struct {
		sig string
		off uint32
		sz  uint32
	}{
		{sig: "cprt", off: 240, sz: 32},
		{sig: "desc", off: 272, sz: 40},
		{sig: "wtpt", off: 312, sz: 20},
		{sig: "rXYZ", off: 332, sz: 20},
		{sig: "gXYZ", off: 352, sz: 20},
		{sig: "bXYZ", off: 372, sz: 20},
		{sig: "rTRC", off: 392, sz: 14},
		{sig: "gTRC", off: 392, sz: 14},
		{sig: "bTRC", off: 392, sz: 14},
	}
	putU32(128, uint32(len(tags)))
	for i, t := range tags {
		base := 132 + i*12
		putSig(base, t.sig)
		putU32(base+4, t.off)
		putU32(base+8, t.sz)
	}

	putSig(240, "text")
	copy(b[248:], []byte("Copyright 2026 sRGB\x00"))

	putSig(272, "desc")
	putU32(280, 18)
	copy(b[284:], []byte("sRGB IEC61966-2.1\x00"))

	putSig(312, "XYZ ")
	putXYZ(320, 63189, 65536, 54074)

	putSig(332, "XYZ ")
	putXYZ(340, 28584, 14582, 911)

	putSig(352, "XYZ ")
	putXYZ(360, 25235, 46991, 6367)

	putSig(372, "XYZ ")
	putXYZ(380, 9380, 3971, 46809)

	putSig(392, "curv")
	putU32(400, 1)
	binary.BigEndian.PutUint16(b[404:406], 563)

	return b
}

func hasCatalogWithMetadataLink(pdf []byte) bool {
	matches := objLineRe.FindAllSubmatchIndex(pdf, -1)
	for i := 0; i < len(matches); i++ {
		start := matches[i][0]
		end := len(pdf)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		obj := pdf[start:end]
		isCatalog := bytes.Contains(obj, []byte("/Type /Catalog")) || bytes.Contains(obj, []byte("/Type/Catalog"))
		if isCatalog && bytes.Contains(obj, []byte("/Metadata")) {
			return true
		}
	}
	return false
}

func canonicalizeDictString(s string) string {
	r := s
	r = strings.ReplaceAll(r, "/Type/Catalog", "/Type /Catalog")
	r = strings.ReplaceAll(r, "/AFRelationship/Data", "/AFRelationship /Data")
	r = strings.ReplaceAll(r, "/Subtype/application#232Fjson", "/Subtype /application#2Fjson")
	return r
}
