package c2pa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// is embedded as a proper PDF EmbeddedFile stream with AFRelationship /C2PA
// so that standard C2PA tools (Acrobat, c2patool) can verify the provenance chain.
//
// Signing is delegated to the Crypto Provider Service (DCS-IR-SI-12); no private
// keys are held in the DCS process (DCS-IR-HI-01).
func AppendManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	storer IPFSStorer,
	issuerDID string,
	assertion LifecycleAssertion,
	existingPDF []byte,
) (*EmbedResult, error) {
	// Build the signed JUMBF manifest.
	manifestBytes, manifestHash, err := BuildManifest(ctx, signer, tsaCfg, issuerDID, assertion)
	if err != nil {
		return nil, fmt.Errorf("build C2PA manifest: %w", err)
	}

	// Embed the manifest as a proper PDF incremental update per the C2PA PDF binding spec.
	updatedPDF, err := writeC2PAIncrement(existingPDF, manifestBytes, manifestHash)
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
//   - A FileSpec dict with /AFRelationship /C2PA references the stream
//   - The document catalog is updated with /AF pointing to the FileSpec
//   - A well-formed xref increment and trailer preserve existing signatures
//     (DCS-OR-C2PA-010, ISO 32000 §7.5.6)
//
// manifestHash is appended as a DCS-private PDF comment after %%EOF for use
// in the manifest hash chain (PrevManifestHashFrom). It is invisible to PDF
// readers and does not affect any byte-range signatures.
func writeC2PAIncrement(existingPDF, jumbfBytes []byte, manifestHash string) ([]byte, error) {
	// Parse the PDF to access the xref table and catalog.
	rs := bytes.NewReader(existingPDF)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false // classic xref tables; simpler to write manually

	ctx, err := pdfapi.ReadValidateAndOptimize(rs, conf)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xrt := ctx.XRefTable

	prevStartXRef := extractLastStartXRef(existingPDF)
	maxObjNum := *xrt.Size - 1
	catalogObjNum := int(xrt.Root.ObjectNumber)

	// Assign new object numbers.
	jumbfObjNum := maxObjNum + 1
	filespecObjNum := maxObjNum + 2

	base := int64(len(existingPDF))
	offsets := map[int]int64{}
	var inc bytes.Buffer

	// --- Object: EmbeddedFile stream -----------------------------------------
	offsets[jumbfObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", jumbfObjNum)
	fmt.Fprintf(&inc, "<</Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d>>\n", len(jumbfBytes))
	inc.WriteString("stream\n")
	inc.Write(jumbfBytes)
	inc.WriteString("\nendstream\nendobj\n")

	// --- Object: FileSpec dict ------------------------------------------------
	offsets[filespecObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n", filespecObjNum)
	fmt.Fprintf(&inc,
		"<</Type /Filespec /F (c2pa_manifest.c2pa) /UF (c2pa_manifest.c2pa) /EF <</F %d 0 R>> /AFRelationship /C2PA>>\n",
		jumbfObjNum)
	inc.WriteString("endobj\n")

	// --- Object: updated catalog (AF array append) ---------------------------
	catDict, err := xrt.Catalog()
	if err != nil {
		return nil, fmt.Errorf("read PDF catalog: %w", err)
	}

	// Preserve any existing AF entries (prior C2PA manifests).
	var afArray pdftypes.Array
	if existing, ok := catDict["AF"]; ok {
		switch v := existing.(type) {
		case pdftypes.Array:
			afArray = v
		case pdftypes.IndirectRef:
			if obj, err2 := xrt.Dereference(v); err2 == nil {
				if arr, ok2 := obj.(pdftypes.Array); ok2 {
					afArray = arr
				}
			}
		}
	}
	afArray = append(afArray, pdftypes.NewIndirectRef(filespecObjNum, 0))
	catDict.Update("AF", afArray)

	offsets[catalogObjNum] = base + int64(inc.Len())
	fmt.Fprintf(&inc, "%d 0 obj\n%s\nendobj\n", catalogObjNum, catDict.PDFString())

	// --- xref increment -------------------------------------------------------
	xrefOffset := base + int64(inc.Len())
	inc.WriteString("xref\n")

	allObjs := []int{jumbfObjNum, filespecObjNum, catalogObjNum}
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
	newSize := filespecObjNum + 1
	inc.WriteString("trailer\n<<\n")
	fmt.Fprintf(&inc, "/Size %d\n/Root %d 0 R\n/Prev %d\n", newSize, catalogObjNum, prevStartXRef)
	inc.WriteString(">>\n")
	fmt.Fprintf(&inc, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	// Append private comment for manifest hash chain (not part of PDF object model;
	// invisible to readers; does not intersect any signature ByteRange).
	fmt.Fprintf(&inc, "%%%% DCS-C2PA-HASH: %s\n", manifestHash)

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

// FileHashOf returns the SHA-256 hex of data, used for LifecycleAssertion.FileHash.
func FileHashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// PrevManifestHashFrom returns the SHA-256 hex of the most recent C2PA manifest
// appended to pdfBytes by AppendManifest, or "" if none exists.
// It reads the private DCS comment written after %%EOF by writeC2PAIncrement.
func PrevManifestHashFrom(pdfBytes []byte) string {
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
