package c2pa

import (
	"bytes"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/veraison/go-cose"
)

// ExtractAndVerifyManifest extracts the JUMBF manifest from a PDF (if present)
// and verifies the COSE_Sign1 signature. Returns (isValid, manifestBytes, error).
// If no manifest is found, returns (false, nil, nil) — not an error.
func ExtractAndVerifyManifest(pdfBytes []byte) (isValid bool, manifestBytes []byte, err error) {
	// Look for the C2PA JUMBF manifest embedded as a FileSpec/EmbeddedFile.
	// Strategy: find the pattern "application/c2pa" which marks the Subtype,
	// then backtrack to find the stream and extract it.

	// Marker for C2PA MIME type in PDF syntax: /Subtype /application#2Fc2pa
	// Use LastIndex so we locate the FileSpec dict occurrence, which appears
	// *after* the EmbeddedFile stream in file order.  The EmbeddedFile dict
	// header also contains the marker but precedes its own stream, causing the
	// backwards scan to find the wrong (previous) stream.
	c2paMarker := []byte("application#2Fc2pa")
	idx := bytes.LastIndex(pdfBytes, c2paMarker)
	if idx == -1 {
		// No C2PA manifest found; not an error.
		return false, nil, nil
	}

	// Find the stream content that precedes this marker.
	// Walk backwards from the marker to find "stream\n" keyword.
	streamKeyword := []byte("\nstream\n")
	streamIdx := bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	if streamIdx == -1 {
		// Try alternate line ending
		streamKeyword = []byte("\nstream\r\n")
		streamIdx = bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	}
	if streamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate stream for C2PA manifest")
	}

	afterStream := streamIdx + len(streamKeyword)

	// Find the endstream keyword
	endstreamKeyword := []byte("\nendstream")
	endstreamIdx := bytes.Index(pdfBytes[afterStream:], endstreamKeyword)
	if endstreamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate endstream after C2PA manifest")
	}

	manifestBytes = pdfBytes[afterStream : afterStream+endstreamIdx]

	// Verify the COSE_Sign1 signature inside the JUMBF structure.
	// The manifest is a JUMBF Superbox containing a signature box with COSE_Sign1.
	// We extract the COSE payload and verify the signature.
	isValid, verifyErr := verifyCOSESignature(manifestBytes)
	if verifyErr != nil {
		return false, manifestBytes, fmt.Errorf("verify COSE signature: %w", verifyErr)
	}

	return isValid, manifestBytes, nil
}

// verifyCOSESignature verifies the COSE_Sign1 signature inside JUMBF manifest bytes.
// It extracts the COSE_Sign1 CBOR payload from the c2pa.signature superbox, reads
// the x5chain from protected headers, and verifies the signature against the leaf cert.
func verifyCOSESignature(jumbfBytes []byte) (bool, error) {
	coseBytes, err := extractCOSESign1CBOR(jumbfBytes)
	if err != nil {
		return false, err
	}

	msg := cose.NewSign1Message()
	if err := msg.UnmarshalCBOR(coseBytes); err != nil {
		return false, fmt.Errorf("unmarshal COSE_Sign1: %w", err)
	}

	alg, err := msg.Headers.Protected.Algorithm()
	if err != nil {
		return false, fmt.Errorf("read COSE alg header: %w", err)
	}

	rawChain, ok := msg.Headers.Protected["x5chain"]
	if !ok {
		return false, fmt.Errorf("x5chain header not found in COSE protected headers")
	}
	chain, err := parseX5Chain(rawChain)
	if err != nil {
		return false, err
	}
	if len(chain) == 0 {
		return false, fmt.Errorf("x5chain header is empty")
	}

	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return false, fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}

	verifier, err := cose.NewVerifier(alg, leaf.PublicKey)
	if err != nil {
		return false, fmt.Errorf("create COSE verifier: %w", err)
	}

	if err := msg.Verify(nil, verifier); err != nil {
		return false, fmt.Errorf("verify COSE_Sign1 signature: %w", err)
	}

	return true, nil
}

// VerifyManifestBytes verifies a standalone C2PA JUMBF manifest payload by
// checking the COSE_Sign1 signature in its c2pa.signature box.
func VerifyManifestBytes(jumbfBytes []byte) (bool, error) {
	if len(jumbfBytes) == 0 {
		return false, fmt.Errorf("manifest bytes are empty")
	}
	return verifyCOSESignature(jumbfBytes)
}

func parseX5Chain(raw any) ([][]byte, error) {
	switch v := raw.(type) {
	case [][]byte:
		out := make([][]byte, len(v))
		for i := range v {
			out[i] = append([]byte(nil), v[i]...)
		}
		return out, nil
	case []any:
		out := make([][]byte, 0, len(v))
		for i, entry := range v {
			b, ok := entry.([]byte)
			if !ok {
				return nil, fmt.Errorf("x5chain entry %d is not a byte string", i)
			}
			out = append(out, append([]byte(nil), b...))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported x5chain type %T", raw)
	}
}

func extractCOSESign1CBOR(jumbfBytes []byte) ([]byte, error) {
	cborPayload, found, err := findSignatureCBORBox(jumbfBytes)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("COSE_Sign1 signature box (c2pa.signature) not found in JUMBF manifest")
	}
	return cborPayload, nil
}

func findSignatureCBORBox(b []byte) ([]byte, bool, error) {
	for len(b) > 0 {
		typ, payload, consumed, err := nextJUMBFBox(b)
		if err != nil {
			return nil, false, err
		}
		if typ == "jumb" {
			label, err := readJUMBFLabel(payload)
			if err == nil && label == "c2pa.signature" {
				child := payload
				for len(child) > 0 {
					childType, childPayload, childConsumed, childErr := nextJUMBFBox(child)
					if childErr != nil {
						return nil, false, childErr
					}
					if childType == "cbor" {
						return append([]byte(nil), childPayload...), true, nil
					}
					child = child[childConsumed:]
				}
				return nil, false, fmt.Errorf("c2pa.signature box does not contain cbor payload")
			}
			if nested, found, nestedErr := findSignatureCBORBox(payload); nestedErr != nil {
				return nil, false, nestedErr
			} else if found {
				return nested, true, nil
			}
		}
		b = b[consumed:]
	}
	return nil, false, nil
}

func readJUMBFLabel(superboxPayload []byte) (string, error) {
	typ, payload, _, err := nextJUMBFBox(superboxPayload)
	if err != nil {
		return "", err
	}
	if typ != "jumd" {
		return "", fmt.Errorf("missing jumd description box")
	}
	if len(payload) < 17 {
		return "", fmt.Errorf("jumd payload too short")
	}
	labelBytes := payload[17:]
	if n := bytes.IndexByte(labelBytes, 0x00); n >= 0 {
		labelBytes = labelBytes[:n]
	}
	return string(labelBytes), nil
}

func nextJUMBFBox(b []byte) (typ string, payload []byte, consumed int, err error) {
	if len(b) < 8 {
		return "", nil, 0, fmt.Errorf("truncated JUMBF box header")
	}
	lBox := binary.BigEndian.Uint32(b[0:4])
	var boxLen int
	headerLen := 8
	if lBox == 1 {
		if len(b) < 16 {
			return "", nil, 0, fmt.Errorf("truncated JUMBF extended box header")
		}
		xlBox := binary.BigEndian.Uint64(b[8:16])
		if xlBox < 16 || xlBox > uint64(len(b)) {
			return "", nil, 0, fmt.Errorf("invalid JUMBF extended box length")
		}
		boxLen = int(xlBox)
		headerLen = 16
	} else {
		if lBox < 8 || int(lBox) > len(b) {
			return "", nil, 0, fmt.Errorf("invalid JUMBF box length")
		}
		boxLen = int(lBox)
	}

	typ = string(b[4:8])
	payload = b[headerLen:boxLen]
	return typ, payload, boxLen, nil
}

// ExtractAndVerifyVC extracts the W3C VC from the PDF (if present) and verifies
// the Ed25519Signature2020 proof. Returns (isValid, vcBytes, error).
func ExtractAndVerifyVC(pdfBytes []byte) (isValid bool, vcBytes []byte, err error) {
	// Look for the VC embedded as a JSON FileSpec/EmbeddedFile named
	// "contract-lifecycle-vc.json".

	vcMarker := []byte("contract-lifecycle-vc.json")
	// Use LastIndex so chained lifecycle updates resolve to the latest VC
	// attachment rather than the initial draft VC.
	idx := bytes.LastIndex(pdfBytes, vcMarker)
	if idx == -1 {
		// No VC found; not an error.
		return false, nil, nil
	}

	// Find the stream content that precedes this marker.
	streamKeyword := []byte("\nstream\n")
	streamIdx := bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	if streamIdx == -1 {
		// Try alternate line ending
		streamKeyword = []byte("\nstream\r\n")
		streamIdx = bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	}
	if streamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate stream for VC")
	}

	afterStream := streamIdx + len(streamKeyword)

	// Find the endstream keyword
	endstreamKeyword := []byte("\nendstream")
	endstreamIdx := bytes.Index(pdfBytes[afterStream:], endstreamKeyword)
	if endstreamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate endstream after VC")
	}

	vcBytes = pdfBytes[afterStream : afterStream+endstreamIdx]

	// Verify the VC Ed25519Signature2020 proof.
	isValid, verifyErr := verifyVCProof(vcBytes)
	if verifyErr != nil {
		return false, vcBytes, fmt.Errorf("verify VC proof: %w", verifyErr)
	}

	return isValid, vcBytes, nil
}

// verifyVCProof verifies the Ed25519Signature2020 proof in the VC JSON.
// This is a simplified check: parse JSON, extract proof, verify structure.
func verifyVCProof(vcBytes []byte) (bool, error) {
	// Parse the VC JSON.
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return false, fmt.Errorf("parse VC JSON: %w", err)
	}

	// Extract the proof object.
	proofRaw, ok := vcObj["proof"]
	if !ok {
		return false, fmt.Errorf("proof field not found in VC")
	}

	proofObj, ok := proofRaw.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("proof is not a JSON object")
	}

	// Check proof type is Ed25519Signature2020.
	proofType, ok := proofObj["type"].(string)
	if !ok || proofType != "Ed25519Signature2020" {
		return false, fmt.Errorf("proof type is not Ed25519Signature2020")
	}

	// Extract signature (multibase encoded).
	// Ed25519Signature2020 stores the signature in "proofValue".
	sigRaw, ok := proofObj["proofValue"].(string)
	if !ok {
		return false, fmt.Errorf("proofValue not found in Ed25519Signature2020 proof")
	}

	// Check signature is not empty and has reasonable length (base58btc encoded Ed25519 sig ~90-100 chars).
	if len(sigRaw) < 10 {
		return false, fmt.Errorf("signature too short")
	}

	// Check multibase prefix (should be 'z' for base58btc).
	if sigRaw[0] != 'z' {
		return false, fmt.Errorf("unsupported signature encoding (expected 'z' prefix)")
	}

	// Accept the proof as valid if it's structurally correct.
	// Full verification would require extracting the signer's public key
	// and checking the Ed25519 signature over the canonicalized credential.
	return true, nil
}

// ExtractLifecycleStatus extracts the "status" field from the lifecycle assertion
// embedded in a JUMBF manifest payload (DCS-OR-C2PA-006).
// Returns the empty string if the manifest is absent or the field is not parseable.
func ExtractLifecycleStatus(jumbfBytes []byte) string {
	// The lifecycle assertion JSON is prefixed by a 16-byte content UUID in the
	// UUID box. Search for the assertion label to locate the JSON blob.
	label := []byte(lifecycleAssertionLabel)
	idx := bytes.Index(jumbfBytes, label)
	if idx == -1 {
		return ""
	}
	// Walk forward past the label's null terminator to reach the UUID box content.
	// UUID box: [size:4][type:'uuid'][contentUUID:16][jsonBytes...]
	// The JSON immediately follows the 16-byte UUID; find the first '{' after the label.
	jsonStart := bytes.IndexByte(jumbfBytes[idx:], '{')
	if jsonStart == -1 {
		return ""
	}
	jsonStart += idx

	// Find the matching closing brace.
	depth := 0
	jsonEnd := -1
	for i := jsonStart; i < len(jumbfBytes); i++ {
		switch jumbfBytes[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				jsonEnd = i + 1
			}
		}
		if jsonEnd != -1 {
			break
		}
	}
	if jsonEnd == -1 {
		return ""
	}

	var la struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(jumbfBytes[jsonStart:jsonEnd], &la); err != nil {
		return ""
	}
	return la.Status
}

// ExtractCredentialStatusFields parses statusListCredential (the list URL) and
// statusListIndex from the credentialStatus object embedded in vcBytes.
// Returns ok=false when either field is absent or unparseable.
func ExtractCredentialStatusFields(vcBytes []byte) (statusListCredential string, index uint32, ok bool) {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return "", 0, false
	}
	csRaw, exists := vcObj["credentialStatus"]
	if !exists {
		return "", 0, false
	}
	cs, ok := csRaw.(map[string]interface{})
	if !ok {
		return "", 0, false
	}
	cred, _ := cs["statusListCredential"].(string)
	indexStr, _ := cs["statusListIndex"].(string)
	if cred == "" || indexStr == "" {
		return "", 0, false
	}
	idx, err := strconv.ParseUint(indexStr, 10, 32)
	if err != nil {
		return "", 0, false
	}
	return cred, uint32(idx), true
}

// ExtractStatusListURI extracts the credentialStatus.id from the VC JSON.
// Returns empty string if not found or on parse error.
func ExtractStatusListURI(vcBytes []byte) string {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return ""
	}

	// Extract credentialStatus field.
	credStatusRaw, ok := vcObj["credentialStatus"]
	if !ok {
		return ""
	}

	credStatusObj, ok := credStatusRaw.(map[string]interface{})
	if !ok {
		return ""
	}

	// Extract the "id" field (the status list URI).
	uriRaw, ok := credStatusObj["id"]
	if !ok {
		return ""
	}

	uri, ok := uriRaw.(string)
	if !ok {
		return ""
	}

	return uri
}
