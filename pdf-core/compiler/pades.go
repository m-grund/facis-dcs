package compiler

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
)

const (
	// envPAdESSigningEndpoint is the backend's authenticated internal PAdES
	// signing endpoint (POST /internal/pades/sign). pdf-core delegates the
	// ECDSA operation over the CMS SignedAttributes digest to it (DCS-IR-HI-01);
	// unlike the C2PA endpoint it returns an ASN.1 DER signature.
	envPAdESSigningEndpoint = "DCS_PDF_CORE_PADES_SIGNING_ENDPOINT"
	// envPAdESX5ChainPEM / envPAdESX5ChainPEMFile carry the PAdES leaf
	// certificate (and chain) whose public key matches the backend's dcs-pades
	// token key. The leaf is embedded as the CMS signing certificate and the
	// remainder as the x5chain.
	envPAdESX5ChainPEM     = "DCS_PDF_CORE_PADES_X5CHAIN_PEM"
	envPAdESX5ChainPEMFile = "DCS_PDF_CORE_PADES_X5CHAIN_PEM_FILE"
	// envPAdESTSAURL is the RFC 3161 TSA endpoint pdf-core requests a timestamp
	// token from (PAdES-B-T). When unset or unreachable the signature falls back
	// to PAdES-B-B (no timestamp) rather than hard-failing.
	envPAdESTSAURL = "DCS_PDF_CORE_TSA_URL"
)

type padesMaterial struct {
	signer    crypto.Signer
	leaf      *x509.Certificate
	chain     []*x509.Certificate
	tsaURL    string
	available bool
	err       error
}

var (
	padesMaterialOnce sync.Once
	padesMaterial_    padesMaterial
)

func loadPAdESMaterial() padesMaterial {
	padesMaterialOnce.Do(func() {
		padesMaterial_ = resolvePAdESMaterial(os.Getenv, os.ReadFile)
	})
	return padesMaterial_
}

func resolvePAdESMaterial(getenv func(string) string, readFile func(string) ([]byte, error)) padesMaterial {
	endpoint := strings.TrimSpace(getenv(envPAdESSigningEndpoint))
	if endpoint == "" {
		return padesMaterial{err: fmt.Errorf("%s is required: pdf-core signs PAdES via the backend's internal signing endpoint", envPAdESSigningEndpoint)}
	}

	inline := strings.TrimSpace(getenv(envPAdESX5ChainPEM))
	file := strings.TrimSpace(getenv(envPAdESX5ChainPEMFile))
	chainPEM, provided, err := resolveSigningConfigValue(readFile, inline, file, envPAdESX5ChainPEM, envPAdESX5ChainPEMFile)
	if err != nil {
		return padesMaterial{err: err}
	}
	if !provided {
		return padesMaterial{err: fmt.Errorf("x5chain must be provided; set %s or %s", envPAdESX5ChainPEM, envPAdESX5ChainPEMFile)}
	}
	certsDER, err := parseCertificateChainPEM([]byte(chainPEM))
	if err != nil {
		return padesMaterial{err: err}
	}
	if len(certsDER) == 0 {
		return padesMaterial{err: fmt.Errorf("PAdES x5chain is empty")}
	}
	chain := make([]*x509.Certificate, 0, len(certsDER))
	for _, der := range certsDER {
		cert, perr := x509.ParseCertificate(der)
		if perr != nil {
			return padesMaterial{err: fmt.Errorf("parse PAdES certificate: %w", perr)}
		}
		chain = append(chain, cert)
	}

	return padesMaterial{
		signer:    newPAdESCallbackSigner(endpoint, chain[0].PublicKey),
		leaf:      chain[0],
		chain:     chain,
		tsaURL:    strings.TrimSpace(getenv(envPAdESTSAURL)),
		available: true,
	}
}

// SignPAdES applies an ETSI PAdES signature to pdf, placing it in the AcroForm
// signature field and binding signatoryName as the CMS signer name. The ECDSA
// operation is delegated to the backend (DCS-IR-HI-01). When a TSA is configured
// and reachable a PAdES-B-T RFC 3161 timestamp is embedded; otherwise the
// signature falls back to PAdES-B-B (no timestamp).
func SignPAdES(ctx context.Context, pdfBytes []byte, fieldName, signatoryName string) ([]byte, error) {
	material := loadPAdESMaterial()
	if material.err != nil {
		return nil, material.err
	}

	signer := material.signer
	if hs, ok := signer.(*padesCallbackSigner); ok {
		signer = hs.withContext(ctx)
	}

	signData := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.ApprovalSignature,
			DocMDPPerm: sign.AllowFillingExistingFormFieldsAndSignaturesPerms,
			Info: sign.SignDataSignatureInfo{
				Name: signatoryName,
				Date: time.Now().UTC(),
			},
		},
		Signer:            signer,
		DigestAlgorithm:   crypto.SHA256,
		Certificate:       material.leaf,
		CertificateChains: [][]*x509.Certificate{material.chain},
	}
	if material.tsaURL != "" {
		signData.TSA = sign.TSA{URL: material.tsaURL}
	}

	signed, err := signPAdESBytes(pdfBytes, signData)
	if err != nil && material.tsaURL != "" {
		// PAdES-B-B fallback: the TSA is configured but unreachable/failed. Retry
		// without a timestamp rather than hard-failing (documented deviation).
		signData.TSA = sign.TSA{}
		if fallbackSigned, fbErr := signPAdESBytes(pdfBytes, signData); fbErr == nil {
			return fallbackSigned, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return signed, nil
}

func signPAdESBytes(pdfBytes []byte, signData sign.SignData) ([]byte, error) {
	rdr, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return nil, fmt.Errorf("pades: parse pdf: %w", err)
	}
	var out bytes.Buffer
	if err := sign.Sign(bytes.NewReader(pdfBytes), &out, rdr, int64(len(pdfBytes)), signData); err != nil {
		return nil, fmt.Errorf("pades: sign: %w", err)
	}
	return relabelSubFilterCAdES(out.Bytes()), nil
}

// pdfsignAdbeSubFilter and padesCAdESSubFilter are equal-length PDF name tokens.
// digitorus/pdfsign always writes the adbe.pkcs7.detached SubFilter; the PAdES
// baseline (ETSI EN 319 142-1) requires ETSI.CAdES.detached for the CAdES-based
// detached signature the CMS SignedData already carries (it embeds the ESS
// SigningCertificateV2 attribute). The two names are the same byte length, so
// the label is corrected without shifting any /ByteRange or xref offsets.
var (
	pdfsignAdbeSubFilter = []byte("/adbe.pkcs7.detached")
	padesCAdESSubFilter  = []byte("/ETSI.CAdES.detached")
)

func relabelSubFilterCAdES(pdfBytes []byte) []byte {
	return bytes.Replace(pdfBytes, pdfsignAdbeSubFilter, padesCAdESSubFilter, 1)
}

// padesCallbackSigner is a crypto.Signer that delegates the ECDSA operation over
// the CMS SignedAttributes digest to the backend's authenticated internal PAdES
// signing endpoint (POST /internal/pades/sign). It returns the ASN.1 DER
// signature the CMS SignedData embeds directly.
type padesCallbackSigner struct {
	endpoint string
	pub      crypto.PublicKey
	client   *http.Client
	ctx      context.Context
}

func newPAdESCallbackSigner(endpoint string, pub crypto.PublicKey) *padesCallbackSigner {
	return &padesCallbackSigner{
		endpoint: strings.TrimRight(endpoint, "/"),
		pub:      pub,
		client:   &http.Client{},
		ctx:      context.Background(),
	}
}

func (s *padesCallbackSigner) withContext(ctx context.Context) *padesCallbackSigner {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *padesCallbackSigner) Public() crypto.PublicKey { return s.pub }

func (s *padesCallbackSigner) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	body, err := json.Marshal(map[string]string{
		"digest": base64.StdEncoding.EncodeToString(digest),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal pades sign request: %w", err)
	}
	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create pades sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := bearerTokenFromContext(s.ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call pades signing endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pades signing endpoint returned status %d", resp.StatusCode)
	}
	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode pades sign response: %w", err)
	}
	der, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode pades signature base64: %w", err)
	}
	return der, nil
}
