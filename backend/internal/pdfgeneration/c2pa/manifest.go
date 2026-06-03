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
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digitorus/timestamp"
	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/veraison/go-cose"
)

// producer identifies this C2PA claim generator.
const producer = "DCS PDF Generation v1"

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
type TSAConfig struct {
	// URL is the TSA endpoint. If empty, no timestamp is requested.
	URL string
}

// claim is the JSON structure inside the COSE_Sign1 payload.
type claim struct {
	Assertions     []assertionRef `json:"assertions"`
	ClaimGenerator string         `json:"claim_generator"`
	Signature      string         `json:"signature"`
	DCFormat       string         `json:"dc:format"`
	InstanceID     string         `json:"instanceID"`
	Alg            string         `json:"alg,omitempty"`
}

type assertionRef struct {
	URL  string `json:"url"`
	Alg  string `json:"alg"`
	Hash []byte `json:"hash"`
}

const manifestLabel = "c2pa.manifest"

var lifecycleContentUUID = [16]byte{0x5F, 0x93, 0x6D, 0x19, 0x2A, 0x0F, 0x4B, 0x8D, 0xA0, 0x77, 0x5D, 0x4A, 0x77, 0x63, 0xA5, 0x11}

// BuildManifest builds a JUMBF manifest box containing the lifecycle assertion
// signed by the Crypto Provider Service and (optionally) RFC 3161 timestamped.
// It returns the manifest JUMBF bytes and their SHA-256 for chaining.
func BuildManifest(
	ctx context.Context,
	signer Signer,
	tsaCfg TSAConfig,
	issuerDID string,
	assertion LifecycleAssertion,
	dataHashExclusionStart int,
	dataHashExclusionLength int,
) (manifestBytes []byte, manifestHash string, err error) {
	// 1. Marshal the lifecycle assertion to JSON.
	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return nil, "", fmt.Errorf("marshal assertion: %w", err)
	}
	pdfHashBytes, err := hex.DecodeString(assertion.PDFHash)
	if err != nil || len(pdfHashBytes) == 0 {
		return nil, "", errors.New("lifecycle assertion PDFHash must be a non-empty hex string")
	}

	// 2. Build assertion boxes and hash full assertion JUMBF box bytes for claim linkage.
	dataHashAssertionMap := map[string]any{
		"alg":  "sha256",
		"name": "pdf-asset",
		"hash": pdfHashBytes,
		"pad":  []byte{},
	}
	if dataHashExclusionLength > 0 {
		dataHashAssertionMap["exclusions"] = []map[string]int{{
			"start":  dataHashExclusionStart,
			"length": dataHashExclusionLength,
		}}
	}
	encMode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, "", fmt.Errorf("build canonical CBOR mode: %w", err)
	}
	dataHashAssertionCBOR, err := encMode.Marshal(dataHashAssertionMap)
	if err != nil {
		return nil, "", fmt.Errorf("marshal c2pa.hash.data assertion CBOR: %w", err)
	}
	dataHashAssertionBox := WriteSuperbox(c2paCBORAssertionUUID, "c2pa.hash.data", WriteCBORBox(dataHashAssertionCBOR))
	dataHashAssertionSum := sha256.Sum256(dataHashAssertionBox[8:])

	lifecycleAssertionBox := WriteSuperbox(c2paUUIDAssertionUUID, lifecycleAssertionLabel, WriteUUIDBox(lifecycleContentUUID, assertionJSON))
	lifecycleAssertionSum := sha256.Sum256(lifecycleAssertionBox[8:])

	// 3. Build a claim object limited to C2PA v1 claim fields.
	signatureURI := "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.signature"
	c := claim{
		Assertions: []assertionRef{
			{
				URL:  "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.assertions/c2pa.hash.data",
				Alg:  "sha256",
				Hash: dataHashAssertionSum[:],
			},
			{
				URL:  "self#jumbf=/c2pa/" + manifestLabel + "/c2pa.assertions/" + lifecycleAssertionLabel,
				Alg:  "sha256",
				Hash: lifecycleAssertionSum[:],
			},
		},
		ClaimGenerator: producer,
		Signature:      signatureURI,
		DCFormat:       "application/pdf",
		InstanceID:     "xmp:iid:" + uuid.New().String(),
		Alg:            "sha256",
	}
	claimCBOR, err := encMode.Marshal(c)
	if err != nil {
		return nil, "", fmt.Errorf("marshal claim CBOR: %w", err)
	}

	// 4. Build COSE_Sign1 protected header (alg: ES256 = -7).
	protected := cose.ProtectedHeader{}
	protected.SetAlgorithm(cose.AlgorithmES256)
	chainProvider, ok := signer.(CertificateChainProvider)
	if !ok {
		return nil, "", fmt.Errorf("signer does not expose certificate chain")
	}
	certChain, err := chainProvider.CertificateChain(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("get certificate chain: %w", err)
	}
	if len(certChain) == 0 {
		return nil, "", fmt.Errorf("certificate chain is empty")
	}
	leafCert, err := x509.ParseCertificate(certChain[0])
	if err != nil {
		return nil, "", fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}
	leafECPub, ok := leafCert.PublicKey.(*ecdsa.PublicKey)
	if !ok || leafECPub.Curve != elliptic.P256() {
		return nil, "", fmt.Errorf("x5chain leaf key must be ECDSA P-256 for ES256; got %T", leafCert.PublicKey)
	}
	protected["x5chain"] = certChain

	// 5. Build COSE_Sign1 and sign the Sig_structure with the remote signer.
	sig := &cose.Sign1Message{
		Headers: cose.Headers{
			Protected:   protected,
			Unprotected: cose.UnprotectedHeader{},
		},
		Payload: claimCBOR,
	}
	if err := sig.Sign(rand.Reader, nil, coseSignerAdapter{ctx: ctx, signer: signer}); err != nil {
		return nil, "", fmt.Errorf("sign COSE_Sign1: %w", err)
	}

	// 6. Optionally attach RFC 3161 timestamp (DCS-OR-C2PA-009).
	var tsBytes []byte
	if tsaCfg.URL != "" {
		var tsErr error
		tsBytes, tsErr = requestTimestamp(ctx, tsaCfg.URL, sig.Signature)
		if tsErr != nil {
			return nil, "", fmt.Errorf("request TSA timestamp: %w", tsErr)
		}
	}
	if len(tsBytes) > 0 {
		sig.Headers.Unprotected["sigTst"] = tsBytes
	}

	cosBytes, err := sig.MarshalCBOR()
	if err != nil {
		return nil, "", fmt.Errorf("marshal COSE_Sign1: %w", err)
	}

	// 8. Wrap in canonical C2PA JUMBF hierarchy.
	assertionStore := WriteSuperbox(c2paAssertionStoreUUID, "c2pa.assertions", dataHashAssertionBox, lifecycleAssertionBox)
	claimBox := WriteSuperbox(c2paClaimUUID, "c2pa.claim", WriteCBORBox(claimCBOR))
	signatureBox := WriteSuperbox(c2paSignatureUUID, "c2pa.signature", WriteCBORBox(cosBytes))

	manifestBox := WriteSuperbox(c2paManifestUUID, manifestLabel, assertionStore, claimBox, signatureBox)
	manifestBytes = WriteSuperbox(c2paBlockUUID, "c2pa", manifestBox)
	h := sha256.Sum256(manifestBytes)
	manifestHash = hex.EncodeToString(h[:])

	return manifestBytes, manifestHash, nil
}

// requestTimestamp requests an RFC 3161 timestamp token from the given TSA URL.
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
