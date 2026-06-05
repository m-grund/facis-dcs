package c2pa

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/veraison/go-cose"
)

// producer identifies this C2PA claim generator (C2PA 2.4 §9.1).
const producer = "DCS PDF Generation v1"

// producerVersion is the semantic version of this claim generator.
const producerVersion = "1.0"

// c2paSpecVersion is the C2PA specification version encoded in every claim
// (C2PA 2.4 claim.v2 claim_generator_info.specVersion).
const c2paSpecVersion = "2.4.0"

// c2paActionsAssertion is the label for the c2pa.actions.v2 assertion.
const c2paActionsAssertion = "c2pa.actions.v2"

// c2paIngredientAssertion is the label for the ingredient.v3 assertion in update manifests.
const c2paIngredientAssertion = "c2pa.ingredient.v3"

// Signer signs raw bytes via the Crypto Provider Service and returns the signature.
type Signer interface {
	Sign(ctx context.Context, data []byte) ([]byte, error)
}

// CertificateChainProvider can expose the signer certificate chain for COSE x5chain headers.
type CertificateChainProvider interface {
	CertificateChain(ctx context.Context) ([][]byte, error)
}

type coseSignerAdapter struct {
	ctx    context.Context
	signer Signer
}

func (a coseSignerAdapter) Algorithm() cose.Algorithm {
	return cose.AlgorithmES256
}

func (a coseSignerAdapter) Sign(_ io.Reader, content []byte) ([]byte, error) {
	return a.signer.Sign(a.ctx, content)
}

// TSAConfig holds RFC 3161 timestamp authority configuration (DCS-OR-C2PA-009).
// A configured, reachable TSA is mandatory for all manifest builds; the URL MUST
// NOT be empty on production paths (fail closed per DCS hard-failure policy).
type TSAConfig struct {
	// URL is the TSA endpoint. Must not be empty.
	URL string

	// TrustedRoots holds the DER-encoded certificates of trusted TSA root CAs.
	// When non-empty, the timestamp token's signing certificate chain is verified
	// against these roots on both the write and read paths (DCS-OR-C2PA-009).
	TrustedRoots [][]byte
}

// claimGeneratorInfo is the C2PA 2.4 claim_generator_info entry per §9.1.
type claimGeneratorInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	SpecVersion string `json:"specVersion"`
}

// claimV2 is the JSON structure inside the COSE_Sign1 payload (C2PA 2.4 claim.v2).
type claimV2 struct {
	ClaimGeneratorInfo []claimGeneratorInfo `json:"claim_generator_info"`
	// CreatedAssertions lists assertions authored by this signer.
	CreatedAssertions []assertionRef `json:"created_assertions,omitempty"`
	// GatheredAssertions lists assertions gathered from referenced manifests
	// (used in update manifests to hold c2pa.ingredient.v3).
	GatheredAssertions []assertionRef `json:"gathered_assertions,omitempty"`
	Signature          string         `json:"signature"`
	DCFormat           string         `json:"dc:format"`
	InstanceID         string         `json:"instanceID"`
	Alg                string         `json:"alg,omitempty"`
}

type assertionRef struct {
	URL  string `json:"url"`
	Alg  string `json:"alg"`
	Hash []byte `json:"hash"`
}

// hashedURI is a C2PA hashed URI (url + alg + hash) used in ingredient.v3.
type hashedURI struct {
	URL  string `json:"url"`
	Alg  string `json:"alg"`
	Hash []byte `json:"hash"`
}

// ingredientV3 is the C2PA ingredient.v3 assertion payload for update manifests.
type ingredientV3 struct {
	Title           string    `json:"title"`
	Relationship    string    `json:"relationship"`
	ActiveManifest  hashedURI `json:"activeManifest"`
	ClaimSignature  hashedURI `json:"claimSignature"`
}

const manifestLabel = "c2pa.manifest"
const updateManifestLabel = "c2pa.update.manifest"

var lifecycleContentUUID = [16]byte{0x5F, 0x93, 0x6D, 0x19, 0x2A, 0x0F, 0x4B, 0x8D, 0xA0, 0x77, 0x5D, 0x4A, 0x77, 0x63, 0xA5, 0x11}

// BuildManifest builds a JUMBF standard manifest (c2ma) for the genesis/first
// lifecycle assertion. It includes c2pa.hash.data (hard binding) in
// created_assertions, a c2pa.actions.v2 assertion with c2pa.created, and the
// org.facis.dcs.contract.lifecycle assertion. The TSA URL must be non-empty
// (fail closed per DCS-OR-C2PA-009).
//
// dataHashExclusionStart and dataHashExclusionLength define the byte range
// to exclude from the c2pa.hash.data hash (the appended C2PA increment itself).
// padLen bytes of zero padding are added to the c2pa.hash.data assertion's
// "pad" field to allow deterministic two-pass size stabilisation.
func BuildManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	assertion LifecycleAssertion,
	dataHashExclusionStart int,
	dataHashExclusionLength int,
	padLen int,
) (manifestBytes []byte, manifestHash string, err error) {
	if tsaCfg.URL == "" {
		return nil, "", fmt.Errorf("TSA URL must not be empty: trusted timestamping is mandatory (DCS-OR-C2PA-009)")
	}

	encMode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, "", fmt.Errorf("build canonical CBOR mode: %w", err)
	}

	// 1. Marshal the lifecycle assertion to JSON.
	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return nil, "", fmt.Errorf("marshal assertion: %w", err)
	}
	pdfHashBytes, err := hex.DecodeString(assertion.PDFHash)
	if err != nil || len(pdfHashBytes) == 0 {
		return nil, "", fmt.Errorf("lifecycle assertion PDFHash must be a non-empty hex string")
	}

	// 2. Build c2pa.hash.data assertion (hard binding, created_assertions).
	pad := make([]byte, padLen)
	dataHashAssertionMap := map[string]any{
		"alg":  "sha256",
		"name": "pdf-asset",
		"hash": pdfHashBytes,
		"pad":  pad,
	}
	if dataHashExclusionLength > 0 {
		dataHashAssertionMap["exclusions"] = []map[string]int{{
			"start":  dataHashExclusionStart,
			"length": dataHashExclusionLength,
		}}
	}
	dataHashAssertionCBOR, err := encMode.Marshal(dataHashAssertionMap)
	if err != nil {
		return nil, "", fmt.Errorf("marshal c2pa.hash.data assertion CBOR: %w", err)
	}
	dataHashAssertionBox := WriteSuperbox(c2paCBORAssertionUUID, "c2pa.hash.data", WriteCBORBox(dataHashAssertionCBOR))
	dataHashAssertionSum := sha256.Sum256(dataHashAssertionBox[8:])

	// 3. Build c2pa.actions.v2 assertion with c2pa.created action.
	actionsJSON, err := json.Marshal(map[string]any{
		"actions": []map[string]any{
			{"action": "c2pa.created"},
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("marshal c2pa.actions.v2 assertion: %w", err)
	}
	actionsBox := WriteSuperbox(c2paJSONAssertionUUID, c2paActionsAssertion, WriteJSONBox(actionsJSON))
	actionsSum := sha256.Sum256(actionsBox[8:])

	// 4. Build org.facis.dcs.contract.lifecycle assertion.
	lifecycleAssertionBox := WriteSuperbox(c2paUUIDAssertionUUID, lifecycleAssertionLabel, WriteUUIDBox(lifecycleContentUUID, assertionJSON))
	lifecycleAssertionSum := sha256.Sum256(lifecycleAssertionBox[8:])

	// 5. Build claim.v2 with all created_assertions.
	signatureURI := "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.signature"
	c := claimV2{
		ClaimGeneratorInfo: []claimGeneratorInfo{{
			Name:        producer,
			Version:     producerVersion,
			SpecVersion: c2paSpecVersion,
		}},
		CreatedAssertions: []assertionRef{
			{
				URL:  "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.assertions/c2pa.hash.data",
				Alg:  "sha256",
				Hash: dataHashAssertionSum[:],
			},
			{
				URL:  "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.assertions/" + c2paActionsAssertion,
				Alg:  "sha256",
				Hash: actionsSum[:],
			},
			{
				URL:  "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.assertions/" + lifecycleAssertionLabel,
				Alg:  "sha256",
				Hash: lifecycleAssertionSum[:],
			},
		},
		Signature:  signatureURI,
		DCFormat:   "application/pdf",
		InstanceID: "xmp:iid:" + uuid.New().String(),
		Alg:        "sha256",
	}
	claimCBOR, err := encMode.Marshal(c)
	if err != nil {
		return nil, "", fmt.Errorf("marshal claim CBOR: %w", err)
	}

	cosBytes, err := signClaim(ctx, signer, tsaCfg, claimCBOR)
	if err != nil {
		return nil, "", err
	}

	assertionStore := WriteSuperbox(c2paAssertionStoreUUID, "c2pa.assertions",
		dataHashAssertionBox, actionsBox, lifecycleAssertionBox)
	claimBox := WriteSuperbox(c2paClaimUUID, "c2pa.claim", WriteCBORBox(claimCBOR))
	signatureBox := WriteSuperbox(c2paSignatureUUID, "c2pa.signature", WriteCBORBox(cosBytes))

	manifestBox := WriteSuperbox(c2paManifestUUID, manifestLabel, assertionStore, claimBox, signatureBox)
	manifestBytes = WriteSuperbox(c2paBlockUUID, "c2pa", manifestBox)
	h := sha256.Sum256(manifestBytes)
	manifestHash = hex.EncodeToString(h[:])
	return manifestBytes, manifestHash, nil
}

// BuildUpdateManifest builds a JUMBF update manifest (c2um) for a lifecycle-event
// append per C2PA 2.4 §10.3. Update manifests carry:
//   - c2pa.ingredient.v3 (parentOf) in gathered_assertions referencing the prior manifest
//   - c2pa.actions.v2 with c2pa.edited.metadata in created_assertions
//   - org.facis.dcs.contract.lifecycle in created_assertions
//
// They do NOT contain c2pa.hash.data (no hard binding in update manifests).
// prevManifestHash and prevSignatureHash must be the SHA-256 hex of the prior
// manifest's JUMBF bytes and c2pa.signature box bytes respectively.
// The TSA URL must be non-empty.
func BuildUpdateManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	assertion LifecycleAssertion,
	prevManifestHash string,
	prevSignatureHash string,
) (manifestBytes []byte, manifestHash string, err error) {
	if tsaCfg.URL == "" {
		return nil, "", fmt.Errorf("TSA URL must not be empty: trusted timestamping is mandatory (DCS-OR-C2PA-009)")
	}
	if prevManifestHash == "" {
		return nil, "", fmt.Errorf("prevManifestHash must not be empty for update manifests")
	}

	encMode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, "", fmt.Errorf("build canonical CBOR mode: %w", err)
	}

	// 1. Build c2pa.ingredient.v3 (parentOf) assertion.
	prevManifestHashBytes, err := hex.DecodeString(prevManifestHash)
	if err != nil {
		return nil, "", fmt.Errorf("decode prevManifestHash: %w", err)
	}
	prevSigHashBytes, err := hex.DecodeString(prevSignatureHash)
	if err != nil {
		return nil, "", fmt.Errorf("decode prevSignatureHash: %w", err)
	}
	ing := ingredientV3{
		Title:        "c2pa_manifest_" + prevManifestHash + ".c2pa",
		Relationship: "parentOf",
		ActiveManifest: hashedURI{
			URL:  "self#jumbf=c2pa/" + manifestLabel,
			Alg:  "sha256",
			Hash: prevManifestHashBytes,
		},
		ClaimSignature: hashedURI{
			URL:  "self#jumbf=c2pa/" + manifestLabel + "/c2pa.signature",
			Alg:  "sha256",
			Hash: prevSigHashBytes,
		},
	}
	ingredientCBOR, err := encMode.Marshal(ing)
	if err != nil {
		return nil, "", fmt.Errorf("marshal c2pa.ingredient.v3 CBOR: %w", err)
	}
	ingredientBox := WriteSuperbox(c2paCBORAssertionUUID, c2paIngredientAssertion, WriteCBORBox(ingredientCBOR))
	ingredientSum := sha256.Sum256(ingredientBox[8:])

	// 2. Build c2pa.actions.v2 with c2pa.edited.metadata (update manifests use
	//    restricted action set per C2PA 2.4 §10.3).
	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return nil, "", fmt.Errorf("marshal assertion: %w", err)
	}
	actionsJSON, err := json.Marshal(map[string]any{
		"actions": []map[string]any{
			{"action": "c2pa.edited.metadata"},
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("marshal c2pa.actions.v2 assertion: %w", err)
	}
	actionsBox := WriteSuperbox(c2paJSONAssertionUUID, c2paActionsAssertion, WriteJSONBox(actionsJSON))
	actionsSum := sha256.Sum256(actionsBox[8:])

	// 3. Build org.facis.dcs.contract.lifecycle assertion.
	lifecycleAssertionBox := WriteSuperbox(c2paUUIDAssertionUUID, lifecycleAssertionLabel, WriteUUIDBox(lifecycleContentUUID, assertionJSON))
	lifecycleAssertionSum := sha256.Sum256(lifecycleAssertionBox[8:])

	// 4. Build claim.v2 with created_assertions + gathered_assertions.
	signatureURI := "self#jumbf=/c2pa/" + updateManifestLabel + "/c2pa.signature"
	c := claimV2{
		ClaimGeneratorInfo: []claimGeneratorInfo{{
			Name:        producer,
			Version:     producerVersion,
			SpecVersion: c2paSpecVersion,
		}},
		CreatedAssertions: []assertionRef{
			{
				URL:  "self#jumbf=/c2pa/" + updateManifestLabel + "/c2pa.assertions/" + c2paActionsAssertion,
				Alg:  "sha256",
				Hash: actionsSum[:],
			},
			{
				URL:  "self#jumbf=/c2pa/" + updateManifestLabel + "/c2pa.assertions/" + lifecycleAssertionLabel,
				Alg:  "sha256",
				Hash: lifecycleAssertionSum[:],
			},
		},
		GatheredAssertions: []assertionRef{
			{
				URL:  "self#jumbf=/c2pa/" + updateManifestLabel + "/c2pa.assertions/" + c2paIngredientAssertion,
				Alg:  "sha256",
				Hash: ingredientSum[:],
			},
		},
		Signature:  signatureURI,
		DCFormat:   "application/pdf",
		InstanceID: "xmp:iid:" + uuid.New().String(),
		Alg:        "sha256",
	}
	claimCBOR, err := encMode.Marshal(c)
	if err != nil {
		return nil, "", fmt.Errorf("marshal update manifest claim CBOR: %w", err)
	}

	cosBytes, err := signClaim(ctx, signer, tsaCfg, claimCBOR)
	if err != nil {
		return nil, "", err
	}

	assertionStore := WriteSuperbox(c2paAssertionStoreUUID, "c2pa.assertions",
		ingredientBox, actionsBox, lifecycleAssertionBox)
	claimBox := WriteSuperbox(c2paClaimUUID, "c2pa.claim", WriteCBORBox(claimCBOR))
	signatureBox := WriteSuperbox(c2paSignatureUUID, "c2pa.signature", WriteCBORBox(cosBytes))

	manifestBox := WriteSuperbox(c2paUpdateManifestUUID, updateManifestLabel, assertionStore, claimBox, signatureBox)
	manifestBytes = WriteSuperbox(c2paBlockUUID, "c2pa", manifestBox)
	h := sha256.Sum256(manifestBytes)
	manifestHash = hex.EncodeToString(h[:])
	return manifestBytes, manifestHash, nil
}

// signClaim signs claimCBOR using COSE_Sign1 and attaches the RFC 3161 timestamp token
// to the unprotected header as sigTst. The TSA URL must be non-empty.
func signClaim(ctx context.Context, signer Signer, tsaCfg TSAConfig, claimCBOR []byte) ([]byte, error) {
	chainProvider, ok := signer.(CertificateChainProvider)
	if !ok {
		return nil, fmt.Errorf("signer does not expose certificate chain")
	}
	certChain, err := chainProvider.CertificateChain(ctx)
	if err != nil {
		return nil, fmt.Errorf("get certificate chain: %w", err)
	}
	if len(certChain) == 0 {
		return nil, fmt.Errorf("certificate chain is empty")
	}
	leafCert, err := x509.ParseCertificate(certChain[0])
	if err != nil {
		return nil, fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}
	leafECPub, ok := leafCert.PublicKey.(*ecdsa.PublicKey)
	if !ok || leafECPub.Curve != elliptic.P256() {
		return nil, fmt.Errorf("x5chain leaf key must be ECDSA P-256 for ES256; got %T", leafCert.PublicKey)
	}

	protected := cose.ProtectedHeader{}
	protected.SetAlgorithm(cose.AlgorithmES256)
	protected["x5chain"] = certChain

	sig := &cose.Sign1Message{
		Headers: cose.Headers{
			Protected:   protected,
			Unprotected: cose.UnprotectedHeader{},
		},
		Payload: claimCBOR,
	}
	if err := sig.Sign(rand.Reader, nil, coseSignerAdapter{ctx: ctx, signer: signer}); err != nil {
		return nil, fmt.Errorf("sign COSE_Sign1: %w", err)
	}

	tsBytes, tsErr := requestTimestamp(ctx, tsaCfg.URL, sig.Signature)
	if tsErr != nil {
		return nil, fmt.Errorf("request TSA timestamp: %w", tsErr)
	}
	sig.Headers.Unprotected["sigTst"] = tsBytes

	cosBytes, err := sig.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("marshal COSE_Sign1: %w", err)
	}
	return cosBytes, nil
}

// requestTimestamp requests an RFC 3161 timestamp token from the given TSA URL.
// The token's message imprint is verified against the SHA-256 of data before returning.
func requestTimestamp(ctx context.Context, tsaURL string, data []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := timestamp.CreateRequest(bytes.NewReader(data), &timestamp.RequestOptions{
		Hash:         crypto.SHA256,
		Certificates: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create TSA request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tsaURL, bytes.NewReader(req))
	if err != nil {
		return nil, fmt.Errorf("create TSA HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/timestamp-query")
	httpReq.Header.Set("Accept", "application/timestamp-reply")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call TSA endpoint: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TSA response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected TSA status %d: %s", httpResp.StatusCode, string(body))
	}

	ts, err := timestamp.ParseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse TSA response: %w", err)
	}

	expectedHash := sha256.Sum256(data)
	if !bytes.Equal(ts.HashedMessage, expectedHash[:]) {
		return nil, fmt.Errorf("TSA hashed message mismatch")
	}
	if len(ts.RawToken) == 0 {
		return nil, fmt.Errorf("TSA response token is empty")
	}

	return ts.RawToken, nil
}
