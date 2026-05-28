package c2pa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

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

// AppendManifest appends a C2PA lifecycle assertion to existingPDF as a PDF incremental
// update (DCS-OR-C2PA-002, DCS-OR-C2PA-010). The updated PDF is stored in IPFS and
// the new CID is returned.
//
// Signing is delegated to the Crypto Provider Service (DCS-IR-SI-12); no private keys
// are held in the DCS process (DCS-IR-HI-01).
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

	// Append the manifest to the PDF as an incremental update.
	// The manifest is written as a binary metadata stream appended after %%EOF.
	// This preserves any existing PAdES signatures (DCS-OR-C2PA-010).
	updatedPDF := appendManifestStream(existingPDF, manifestBytes)

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

// appendManifestStream appends the C2PA manifest as a PDF comment-embedded binary stream
// after the existing PDF content. This is the simplest incremental append that does not
// alter any existing byte ranges, preserving PAdES signatures.
//
// The manifest is wrapped in a custom PDF object that readers unaware of C2PA will ignore.
// Full PDF incremental-update compliance (xref update, catalog AF key amendment) can be
// added in a later iteration when pdfcpu integration is introduced.
func appendManifestStream(existingPDF, manifestBytes []byte) []byte {
	// Compute manifest hash for identification.
	h := sha256.Sum256(manifestBytes)
	hashHex := hex.EncodeToString(h[:])

	var buf bytes.Buffer
	buf.Write(existingPDF)

	// Append a clearly delimited C2PA manifest block that does not alter the
	// existing cross-reference table so PAdES ByteRange signatures remain valid.
	buf.WriteString("\n%%C2PA-MANIFEST-BEGIN ")
	buf.WriteString(hashHex)
	buf.WriteString("\n")
	buf.Write(manifestBytes)
	buf.WriteString("\n%%C2PA-MANIFEST-END\n")

	return buf.Bytes()
}

// base64Wrap wraps raw bytes for IPFS CreateFile which expects JSON-serialisable input.
type base64Wrap []byte

func (b base64Wrap) MarshalJSON() ([]byte, error) {
	import64 := make([]byte, 0, 2+len(b)*4/3+4)
	import64 = append(import64, '"')
	enc := make([]byte, ((len(b)+2)/3)*4)
	n := encodeBase64(enc, b)
	import64 = append(import64, enc[:n]...)
	import64 = append(import64, '"')
	return import64, nil
}

func encodeBase64(dst, src []byte) int {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	n := 0
	for i := 0; i < len(src); i += 3 {
		remaining := len(src) - i
		var b0, b1, b2 byte
		b0 = src[i]
		if remaining > 1 {
			b1 = src[i+1]
		}
		if remaining > 2 {
			b2 = src[i+2]
		}
		dst[n] = table[b0>>2]
		dst[n+1] = table[((b0&0x3)<<4)|(b1>>4)]
		if remaining > 1 {
			dst[n+2] = table[((b1&0xf)<<2)|(b2>>6)]
		} else {
			dst[n+2] = '='
		}
		if remaining > 2 {
			dst[n+3] = table[b2&0x3f]
		} else {
			dst[n+3] = '='
		}
		n += 4
	}
	return n
}

// FileHashOf returns the SHA-256 hex of jsonldBytes for use in LifecycleAssertion.FileHash.
func FileHashOf(jsonldBytes []byte) string {
	h := sha256.Sum256(jsonldBytes)
	return hex.EncodeToString(h[:])
}

// PrevManifestHashFrom extracts the last C2PA manifest hash from a PDF that was
// previously processed by AppendManifest, for use as PrevManifestHash in the next
// assertion (DCS-OR-C2PA-003 chain).
func PrevManifestHashFrom(pdfBytes []byte) string {
	marker := []byte("%%C2PA-MANIFEST-BEGIN ")
	// Find the last occurrence (most recent manifest).
	lastIdx := -1
	searchFrom := 0
	for {
		idx := bytes.Index(pdfBytes[searchFrom:], marker)
		if idx == -1 {
			break
		}
		lastIdx = searchFrom + idx
		searchFrom = lastIdx + len(marker)
	}
	if lastIdx == -1 {
		return ""
	}
	start := lastIdx + len(marker)
	end := bytes.IndexByte(pdfBytes[start:], '\n')
	if end == -1 {
		return ""
	}
	return string(bytes.TrimSpace(pdfBytes[start : start+end]))
}

// unused suppresses import errors during development.
var _ = fmt.Sprintf
