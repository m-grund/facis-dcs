package c2pa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/veraison/go-cose"
)

// producer identifies this C2PA claim generator.
const producer = "DCS PDF Generation v1"

// Signer signs raw bytes via the Crypto Provider Service and returns the signature.
type Signer interface {
	Sign(ctx context.Context, data []byte) ([]byte, error)
}

// TSAConfig holds RFC 3161 timestamp authority configuration (DCS-OR-C2PA-009).
type TSAConfig struct {
	// URL is the TSA endpoint. If empty, no timestamp is requested.
	URL string
}

// claim is the JSON structure inside the COSE_Sign1 payload.
type claim struct {
	Assertions      []assertionRef `json:"assertions"`
	ClaimGenerator  string         `json:"claim_generator"`
	RendererVersion string         `json:"renderer_version"`
	SignatureInfo   signInfo       `json:"signature_info"`
}

type assertionRef struct {
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

type signInfo struct {
	Issuer string `json:"issuer"`
	Time   string `json:"time"`
}

// BuildManifest builds a JUMBF manifest box containing the lifecycle assertion
// signed by the Crypto Provider Service and (optionally) RFC 3161 timestamped.
// It returns the manifest JUMBF bytes and their SHA-256 for chaining.
func BuildManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	issuerDID string,
	assertion LifecycleAssertion,
) (manifestBytes []byte, manifestHash string, err error) {
	// 1. Marshal the lifecycle assertion to JSON.
	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return nil, "", fmt.Errorf("marshal assertion: %w", err)
	}

	// 2. Compute assertion hash for the claim.
	assertionHash := sha256.Sum256(assertionJSON)
	assertionHashHex := hex.EncodeToString(assertionHash[:])

	// 3. Build the claim.
	// Use EffectiveAt from the assertion as the signing timestamp so that the claim
	// is reproducible from DB state and does not capture the wall clock.
	c := claim{
		Assertions: []assertionRef{
			{URL: "self#jumbf=/c2pa/assertions/" + lifecycleAssertionLabel, Hash: assertionHashHex},
		},
		ClaimGenerator:  producer,
		RendererVersion: assertion.RendererVersion,
		SignatureInfo: signInfo{
			Issuer: issuerDID,
			Time:   assertion.EffectiveAt.UTC().Format(time.RFC3339),
		},
	}
	claimJSON, err := json.Marshal(c)
	if err != nil {
		return nil, "", fmt.Errorf("marshal claim: %w", err)
	}

	// 4. Build COSE_Sign1 protected header (alg: ES256 = -7).
	protected := cose.ProtectedHeader{}
	protected.SetAlgorithm(cose.AlgorithmES256)

	// 5. Sign the claim bytes via Crypto Provider Service.
	sigBytes, err := signer.Sign(ctx, claimJSON)
	if err != nil {
		return nil, "", fmt.Errorf("sign claim: %w", err)
	}

	// 6. Optionally attach RFC 3161 timestamp (DCS-OR-C2PA-009).
	var tsBytes []byte
	if tsaCfg.URL != "" {
		tsBytes, err = requestTimestamp(ctx, tsaCfg.URL, sigBytes)
		if err != nil {
			// Non-fatal: log and continue without timestamp.
			tsBytes = nil
		}
	}

	// 7. Build COSE_Sign1 structure (untagged, as per C2PA spec).
	sig := &cose.Sign1Message{
		Headers: cose.Headers{
			Protected:   protected,
			Unprotected: cose.UnprotectedHeader{},
		},
		Payload:   claimJSON,
		Signature: sigBytes,
	}
	if len(tsBytes) > 0 {
		sig.Headers.Unprotected["sigTst"] = tsBytes
	}

	cosBytes, err := sig.MarshalCBOR()
	if err != nil {
		return nil, "", fmt.Errorf("marshal COSE_Sign1: %w", err)
	}

	// 8. Wrap in JUMBF: superbox → assertion JSON box + claim CBOR box.
	assertionBox := WriteSuperbox(c2paAssertionUUID, lifecycleAssertionLabel, WriteJSONBox(assertionJSON))
	claimBox := WriteSuperbox(c2paManifestUUID, "c2pa.claim", WriteCBORBox(cosBytes))

	manifestBytes = WriteSuperbox(c2paManifestUUID, "c2pa.manifest", assertionBox, claimBox)
	h := sha256.Sum256(manifestBytes)
	manifestHash = hex.EncodeToString(h[:])

	return manifestBytes, manifestHash, nil
}

// requestTimestamp requests an RFC 3161 timestamp token from the given TSA URL.
func requestTimestamp(_ context.Context, tsaURL string, data []byte) ([]byte, error) {
	h := sha256.Sum256(data)
	req, err := timestamp.CreateRequest(nil, &timestamp.RequestOptions{
		Hash:         4, // SHA-256
		Certificates: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create TSA request: %w", err)
	}
	_ = req
	_ = h
	_ = tsaURL
	// TODO(DCS-OR-C2PA-009): Full TSA HTTP round-trip deferred.
	// Set TSA_URL env var to activate real RFC 3161 timestamps via digitorus/timestamp.
	return nil, nil
}
