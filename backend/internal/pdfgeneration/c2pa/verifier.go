package c2pa

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/digitorus/timestamp"
	base58 "github.com/mr-tron/base58/base58"
	ld "github.com/piprate/json-gold/ld"
	"github.com/veraison/go-cose"
)

// VerifyOptions configures verification behaviour for both the COSE signature
// and the RFC 3161 timestamp token (DCS-OR-C2PA-006, DCS-OR-C2PA-009).
type VerifyOptions struct {
	// TrustedSignerRoots is the set of trusted root CAs for signer certificate
	// chain validation. When nil, signer chain validation is skipped (dev only).
	TrustedSignerRoots *x509.CertPool

	// TrustedTSARoots is the set of trusted root CAs for the TSA signing
	// certificate chain. When nil, TSA chain validation is skipped (dev only).
	// On production paths this must be non-nil (DCS-OR-C2PA-009).
	TrustedTSARoots *x509.CertPool
}

// TimestampResult holds the outcome of verifying the RFC 3161 sigTst token in
// a COSE_Sign1 unprotected header (DCS-OR-C2PA-009).
type TimestampResult struct {
	// Valid is true when the token was parsed, the signature verified, and the
	// message imprint matched the COSE signature bytes.
	Valid bool
	// TrustedTime is the genTime from the verified token.
	TrustedTime time.Time
	// Err is a human-readable reason if Valid is false.
	Err string
}

// ManifestVerifyResult is the extended result of verifying a C2PA manifest,
// including the COSE signature and the RFC 3161 timestamp.
type ManifestVerifyResult struct {
	// SignatureValid is true when the COSE_Sign1 signature verified successfully.
	SignatureValid bool
	// Timestamp holds the RFC 3161 timestamp verification result.
	Timestamp TimestampResult
}

// ExtractAndVerifyManifest extracts the JUMBF manifest from a PDF (if present)
// and verifies the COSE_Sign1 signature plus the RFC 3161 sigTst token.
// Returns (isValid, manifestBytes, error).  If no manifest is found, returns
// (false, nil, nil) — not an error.
func ExtractAndVerifyManifest(pdfBytes []byte) (isValid bool, manifestBytes []byte, err error) {
	result, manifestBytes, err := ExtractAndVerifyManifestFull(pdfBytes, VerifyOptions{})
	return result.SignatureValid, manifestBytes, err
}

// ExtractAndVerifyManifestFull is the full verification path with configurable
// trust roots.  It verifies the COSE signature, the RFC 3161 sigTst, and —
// when opts.TrustedSignerRoots is non-nil — the signer certificate chain.
// Returns (ManifestVerifyResult, manifestBytes, error).
func ExtractAndVerifyManifestFull(pdfBytes []byte, opts VerifyOptions) (ManifestVerifyResult, []byte, error) {
	c2paMarker := []byte("application#2Fc2pa")
	idx := bytes.LastIndex(pdfBytes, c2paMarker)
	if idx == -1 {
		return ManifestVerifyResult{}, nil, nil
	}

	streamKeyword := []byte("\nstream\n")
	streamIdx := bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	if streamIdx == -1 {
		streamKeyword = []byte("\nstream\r\n")
		streamIdx = bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	}
	if streamIdx == -1 {
		return ManifestVerifyResult{}, nil, fmt.Errorf("could not locate stream for C2PA manifest")
	}

	afterStream := streamIdx + len(streamKeyword)

	endstreamKeyword := []byte("\nendstream")
	endstreamIdx := bytes.Index(pdfBytes[afterStream:], endstreamKeyword)
	if endstreamIdx == -1 {
		return ManifestVerifyResult{}, nil, fmt.Errorf("could not locate endstream after C2PA manifest")
	}

	manifestBytes := pdfBytes[afterStream : afterStream+endstreamIdx]

	result, verifyErr := verifyCOSESignatureFull(manifestBytes, opts)
	if verifyErr != nil {
		return ManifestVerifyResult{}, manifestBytes, fmt.Errorf("verify COSE signature: %w", verifyErr)
	}

	return result, manifestBytes, nil
}

// VerifyManifestBytes verifies a standalone C2PA JUMBF manifest payload.
func VerifyManifestBytes(jumbfBytes []byte) (bool, error) {
	if len(jumbfBytes) == 0 {
		return false, fmt.Errorf("manifest bytes are empty")
	}
	result, err := verifyCOSESignatureFull(jumbfBytes, VerifyOptions{})
	return result.SignatureValid, err
}

// VerifyManifestBytesFull is the full variant of VerifyManifestBytes with trust
// root options.
func VerifyManifestBytesFull(jumbfBytes []byte, opts VerifyOptions) (ManifestVerifyResult, error) {
	if len(jumbfBytes) == 0 {
		return ManifestVerifyResult{}, fmt.Errorf("manifest bytes are empty")
	}
	return verifyCOSESignatureFull(jumbfBytes, opts)
}

// verifyCOSESignatureFull verifies the COSE_Sign1 signature in the JUMBF manifest,
// then verifies the RFC 3161 sigTst token from the unprotected header.
func verifyCOSESignatureFull(jumbfBytes []byte, opts VerifyOptions) (ManifestVerifyResult, error) {
	coseBytes, err := extractCOSESign1CBOR(jumbfBytes)
	if err != nil {
		return ManifestVerifyResult{}, err
	}

	msg := cose.NewSign1Message()
	if err := msg.UnmarshalCBOR(coseBytes); err != nil {
		return ManifestVerifyResult{}, fmt.Errorf("unmarshal COSE_Sign1: %w", err)
	}

	alg, err := msg.Headers.Protected.Algorithm()
	if err != nil {
		return ManifestVerifyResult{}, fmt.Errorf("read COSE alg header: %w", err)
	}

	rawChain, ok := msg.Headers.Protected["x5chain"]
	if !ok {
		return ManifestVerifyResult{}, fmt.Errorf("x5chain header not found in COSE protected headers")
	}
	chain, err := parseX5Chain(rawChain)
	if err != nil {
		return ManifestVerifyResult{}, err
	}
	if len(chain) == 0 {
		return ManifestVerifyResult{}, fmt.Errorf("x5chain header is empty")
	}

	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return ManifestVerifyResult{}, fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}

	// Signer certificate chain trust validation.
	if opts.TrustedSignerRoots != nil {
		intermediates := x509.NewCertPool()
		for i := 1; i < len(chain); i++ {
			c, parseErr := x509.ParseCertificate(chain[i])
			if parseErr == nil {
				intermediates.AddCert(c)
			}
		}
		_, verifyErr := leaf.Verify(x509.VerifyOptions{
			Roots:         opts.TrustedSignerRoots,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		if verifyErr != nil {
			return ManifestVerifyResult{}, fmt.Errorf("signer certificate chain not trusted: %w", verifyErr)
		}
	}

	verifier, err := cose.NewVerifier(alg, leaf.PublicKey)
	if err != nil {
		return ManifestVerifyResult{}, fmt.Errorf("create COSE verifier: %w", err)
	}
	if err := msg.Verify(nil, verifier); err != nil {
		return ManifestVerifyResult{}, fmt.Errorf("verify COSE_Sign1 signature: %w", err)
	}

	// Verify the RFC 3161 sigTst token from the unprotected header (DCS-OR-C2PA-009).
	tsResult := verifySignedTimestamp(msg, opts.TrustedTSARoots)

	return ManifestVerifyResult{
		SignatureValid: true,
		Timestamp:      tsResult,
	}, nil
}

// verifySignedTimestamp extracts and verifies the sigTst RFC 3161 token from the
// COSE_Sign1 unprotected header. It checks the token signature, the message
// imprint (must equal SHA-256 of the COSE signature bytes), and — when
// trustedTSARoots is non-nil — the TSA signing certificate chain.
func verifySignedTimestamp(msg *cose.Sign1Message, trustedTSARoots *x509.CertPool) TimestampResult {
	rawSigTst, ok := msg.Headers.Unprotected["sigTst"]
	if !ok {
		return TimestampResult{Valid: false, Err: "sigTst not present in COSE unprotected header"}
	}

	tokenBytes, ok := rawSigTst.([]byte)
	if !ok {
		return TimestampResult{Valid: false, Err: fmt.Sprintf("sigTst is not a byte string (got %T)", rawSigTst)}
	}
	if len(tokenBytes) == 0 {
		return TimestampResult{Valid: false, Err: "sigTst token is empty"}
	}

	ts, err := timestamp.ParseResponse(tokenBytes)
	if err != nil {
		// Some TSAs wrap the token in a ContentInfo; try parse as raw token.
		ts2, err2 := timestamp.Parse(tokenBytes)
		if err2 != nil {
			return TimestampResult{Valid: false, Err: fmt.Sprintf("parse sigTst token: %v (raw: %v)", err, err2)}
		}
		ts = ts2
	}

	// Verify message imprint: must equal SHA-256(COSE_Sign1 signature bytes).
	expectedImprint := sha256.Sum256(msg.Signature)
	if !bytes.Equal(ts.HashedMessage, expectedImprint[:]) {
		return TimestampResult{Valid: false, Err: "sigTst message imprint does not match COSE signature bytes"}
	}

	// Verify TSA certificate chain when trusted roots are configured.
	if trustedTSARoots != nil && ts.Certificates != nil && len(ts.Certificates) > 0 {
		tsaLeaf := ts.Certificates[0]
		intermediates := x509.NewCertPool()
		for _, c := range ts.Certificates[1:] {
			intermediates.AddCert(c)
		}
		_, chainErr := tsaLeaf.Verify(x509.VerifyOptions{
			Roots:         trustedTSARoots,
			Intermediates: intermediates,
			CurrentTime:   ts.Time,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		})
		if chainErr != nil {
			return TimestampResult{Valid: false, Err: fmt.Sprintf("TSA certificate chain not trusted: %v", chainErr)}
		}
	}

	return TimestampResult{Valid: true, TrustedTime: ts.Time}
}

// ExtractVC extracts the W3C VC bytes from a PDF without verifying the proof.
// Returns (found, vcBytes, error). When no VC is present found is false and
// vcBytes is nil; this is not an error.
func ExtractVC(pdfBytes []byte) (found bool, vcBytes []byte, err error) {
	vcMarker := []byte("contract-lifecycle-vc.json")
	idx := bytes.LastIndex(pdfBytes, vcMarker)
	if idx == -1 {
		return false, nil, nil
	}

	streamKeyword := []byte("\nstream\n")
	streamIdx := bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	if streamIdx == -1 {
		streamKeyword = []byte("\nstream\r\n")
		streamIdx = bytes.LastIndex(pdfBytes[:idx], streamKeyword)
	}
	if streamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate stream for VC")
	}

	afterStream := streamIdx + len(streamKeyword)
	endstreamIdx := bytes.Index(pdfBytes[afterStream:], []byte("\nendstream"))
	if endstreamIdx == -1 {
		return false, nil, fmt.Errorf("could not locate endstream after VC")
	}

	return true, pdfBytes[afterStream : afterStream+endstreamIdx], nil
}

// ExtractAndVerifyVC extracts the W3C VC from the PDF (if present) and runs a
// structural proof check (format validation only — does not verify the signature
// cryptographically). For full cryptographic verification inject a
// VCProofVerifier via ContractVerifier.VCProofVerifier (DCS-OR-C2PA-006).
func ExtractAndVerifyVC(pdfBytes []byte) (isValid bool, vcBytes []byte, err error) {
	found, vcBytes, err := ExtractVC(pdfBytes)
	if err != nil || !found {
		return false, nil, err
	}
	isValid, verifyErr := VerifyVCProofStructural(vcBytes)
	if verifyErr != nil {
		return false, vcBytes, fmt.Errorf("verify VC proof: %w", verifyErr)
	}
	return isValid, vcBytes, nil
}

// VerifyVCProofStructural checks the structural format of a VC proof without
// verifying the cryptographic signature. It confirms the proof field exists,
// has a recognised type, and that proofValue is present with the expected
// encoding prefix. This is sufficient for dev/test environments; production
// deployments MUST inject a VCProofVerifier for cryptographic verification.
func VerifyVCProofStructural(vcBytes []byte) (bool, error) {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return false, fmt.Errorf("parse VC JSON: %w", err)
	}

	proofRaw, ok := vcObj["proof"]
	if !ok {
		return false, fmt.Errorf("proof field not found in VC")
	}

	proofObj, ok := proofRaw.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("proof is not a JSON object")
	}

	proofType, _ := proofObj["type"].(string)
	switch proofType {
	case "Ed25519Signature2020":
		sigRaw, ok := proofObj["proofValue"].(string)
		if !ok || len(sigRaw) < 10 {
			return false, fmt.Errorf("proofValue not found or too short in Ed25519Signature2020 proof")
		}
		if sigRaw[0] != 'z' {
			return false, fmt.Errorf("Ed25519Signature2020 proofValue must be base58btc-encoded (expected 'z' prefix)")
		}
		return true, nil

	case "DataIntegrityProof":
		cryptosuite, _ := proofObj["cryptosuite"].(string)
		if cryptosuite == "" {
			return false, fmt.Errorf("DataIntegrityProof missing cryptosuite field")
		}
		sigRaw, ok := proofObj["proofValue"].(string)
		if !ok || len(sigRaw) < 10 {
			return false, fmt.Errorf("proofValue not found or too short in DataIntegrityProof")
		}
		return true, nil

	default:
		return false, fmt.Errorf("unsupported proof type %q (expected Ed25519Signature2020 or DataIntegrityProof)", proofType)
	}
}

// NewEd25519VCProofVerifier returns a VCProofVerifier that performs actual
// Ed25519Signature2020 / DataIntegrityProof (eddsa-rdfc-2022) cryptographic
// verification using URDNA2015 JSON-LD normalisation (DCS-OR-C2PA-006).
//
// trustedKeys maps verificationMethod URL → ed25519.PublicKey. When the proof's
// verificationMethod is not in trustedKeys the verification fails — configure all
// issuer keys that should be accepted. For dev mode pass nil as VCProofVerifier
// to fall back to the structural check.
func NewEd25519VCProofVerifier(trustedKeys map[string]ed25519.PublicKey) func([]byte) (bool, error) {
	return func(vcBytes []byte) (bool, error) {
		return verifyEd25519VCProof(vcBytes, trustedKeys)
	}
}

func verifyEd25519VCProof(vcBytes []byte, trustedKeys map[string]ed25519.PublicKey) (bool, error) {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return false, fmt.Errorf("parse VC JSON: %w", err)
	}

	proofRaw, ok := vcObj["proof"]
	if !ok {
		return false, fmt.Errorf("proof field not found in VC")
	}
	proof, ok := proofRaw.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("proof is not a JSON object")
	}

	proofType, _ := proof["type"].(string)
	switch proofType {
	case "Ed25519Signature2020", "DataIntegrityProof":
	default:
		return false, fmt.Errorf("unsupported proof type %q for key-based verification", proofType)
	}

	verificationMethod, _ := proof["verificationMethod"].(string)
	if verificationMethod == "" {
		return false, fmt.Errorf("verificationMethod missing from proof")
	}

	pubKey, ok := trustedKeys[verificationMethod]
	if !ok {
		return false, fmt.Errorf("no trusted key configured for verificationMethod %q", verificationMethod)
	}

	proofValueStr, _ := proof["proofValue"].(string)
	if len(proofValueStr) < 2 || proofValueStr[0] != 'z' {
		return false, fmt.Errorf("proofValue must be base58btc-encoded (expected 'z' prefix)")
	}
	sigBytes, err := base58.Decode(proofValueStr[1:])
	if err != nil {
		return false, fmt.Errorf("decode base58btc proofValue: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid Ed25519 signature length %d (expected %d)", len(sigBytes), ed25519.SignatureSize)
	}

	// Build the document without the proof and the proof options without proofValue.
	docWithoutProof := make(map[string]interface{}, len(vcObj))
	for k, v := range vcObj {
		if k != "proof" {
			docWithoutProof[k] = v
		}
	}
	proofOptions := map[string]interface{}{"@context": vcObj["@context"]}
	for k, v := range proof {
		if k != "proofValue" {
			proofOptions[k] = v
		}
	}

	// URDNA2015 normalise both halves, then hash and concatenate per
	// Ed25519Signature2020 / eddsa-rdfc-2022 spec.
	proc := ld.NewJsonLdProcessor()
	ldOpts := ld.NewJsonLdOptions("")
	ldOpts.Algorithm = "URDNA2015"
	ldOpts.Format = "application/n-quads"

	normOpts, err := proc.Normalize(proofOptions, ldOpts)
	if err != nil {
		return false, fmt.Errorf("URDNA2015 normalise proof options: %w", err)
	}
	normDoc, err := proc.Normalize(docWithoutProof, ldOpts)
	if err != nil {
		return false, fmt.Errorf("URDNA2015 normalise VC document: %w", err)
	}

	h1 := sha256.Sum256([]byte(normOpts.(string)))
	h2 := sha256.Sum256([]byte(normDoc.(string)))
	verifyData := append(h1[:], h2[:]...)

	return ed25519.Verify(pubKey, verifyData, sigBytes), nil
}

// ExtractLifecycleStatus extracts the "status" field from the lifecycle assertion
// embedded in a JUMBF manifest payload (DCS-OR-C2PA-006).
func ExtractLifecycleStatus(jumbfBytes []byte) string {
	label := []byte(lifecycleAssertionLabel)
	idx := bytes.Index(jumbfBytes, label)
	if idx == -1 {
		return ""
	}
	jsonStart := bytes.IndexByte(jumbfBytes[idx:], '{')
	if jsonStart == -1 {
		return ""
	}
	jsonStart += idx

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

// ExtractCredentialStatusFields parses statusListCredential and statusListIndex
// from the credentialStatus object embedded in vcBytes.
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
func ExtractStatusListURI(vcBytes []byte) string {
	var vcObj map[string]interface{}
	if err := json.Unmarshal(vcBytes, &vcObj); err != nil {
		return ""
	}

	credStatusRaw, ok := vcObj["credentialStatus"]
	if !ok {
		return ""
	}

	credStatusObj, ok := credStatusRaw.(map[string]interface{})
	if !ok {
		return ""
	}

	uri, _ := credStatusObj["id"].(string)
	return uri
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
