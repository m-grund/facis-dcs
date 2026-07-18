package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// DSSSigner produces the PAdES signature through a remote EU DSS via the CSC/
// rQES two-call flow (DCS-IR-SI-10), instead of pdf-core's in-process PKCS#11
// path. The signature value over the DSS data-to-be-signed still comes from the
// backend HSM here (the demonstrator's stand-in for the wallet-unlocked remote
// QTSP); flipping SIGNER_BACKEND to "dss" and pointing the HashSigner at a real
// QTSP is the production switch — the ceremony, evidence embedding, and AcroForm
// field placement are otherwise identical to the pdf-core path.
type DSSSigner struct {
	dss     *dss.Client
	pdfCore *pdfcore.Client
	// hashSigner is the backend HSM signer for the PAdES key: it signs the DSS
	// data-to-be-signed. In production this is replaced by the wallet-unlocked
	// QTSP without touching the rest of the flow.
	padesSigner    crypto.Signer
	signingCertB64 string   // base64 DER of the signing (leaf) certificate
	chainB64       []string // base64 DER of the remaining chain, leaf-excluded
	signatureLevel string   // e.g. "PAdES_BASELINE_B" / "PAdES_BASELINE_T"
}

// NewDSSSigner builds a DSSSigner. x5chainPEM is the PAdES signer's certificate
// chain (leaf first), the same chain pdf-core embeds; the leaf becomes the DSS
// signing certificate and the remainder the certificate chain.
func NewDSSSigner(dssClient *dss.Client, pdfCore *pdfcore.Client, padesSigner crypto.Signer, x5chainPEM, signatureLevel string) (*DSSSigner, error) {
	leaf, chain, err := parseCertChainPEM(x5chainPEM)
	if err != nil {
		return nil, fmt.Errorf("dss signer: %w", err)
	}
	if signatureLevel == "" {
		signatureLevel = "PAdES_BASELINE_B"
	}
	return &DSSSigner{
		dss:            dssClient,
		pdfCore:        pdfCore,
		padesSigner:    padesSigner,
		signingCertB64: leaf,
		chainB64:       chain,
		signatureLevel: signatureLevel,
	}, nil
}

// SignPDF implements ContractSigner: embed the evidence via pdf-core's attach-
// only seam (so the DSS signature's /ByteRange covers it — embed-first-sign-
// second, DCS-FR-SM-08), then have the DSS place the PAdES signature in the
// named AcroForm field, backed by the HSM HashSigner.
func (s *DSSSigner) SignPDF(ctx context.Context, pdf []byte, fieldName, signatoryName string, evidence []byte) ([]byte, error) {
	document := pdf
	if len(evidence) > 0 {
		embedded, err := s.pdfCore.EmbedEvidence(ctx, pdf, evidence)
		if err != nil {
			return nil, fmt.Errorf("dss signer: embed evidence: %w", err)
		}
		document = embedded
	}

	hashSigner := func(_ context.Context, dataToSign []byte) ([]byte, string, error) {
		digest := sha256.Sum256(dataToSign)
		der, err := s.padesSigner.Sign(rand.Reader, digest[:], crypto.SHA256)
		if err != nil {
			return nil, "", fmt.Errorf("hsm sign data-to-be-signed: %w", err)
		}
		return der, "ECDSA_SHA256", nil
	}

	return s.dss.Sign(ctx, document, signatoryName, dss.SignParams{
		Format:             dss.FormatPAdES,
		SignatureLevel:     s.signatureLevel,
		SigningCertificate: s.signingCertB64,
		CertificateChain:   s.chainB64,
		DigestAlgorithm:    "SHA256",
		SignatureFieldID:   fieldName,
	}, hashSigner)
}

// parseCertChainPEM splits a PEM certificate chain into the base64 DER leaf and
// the base64 DER of the remaining certificates (leaf-excluded), the shape the
// DSS REST parameters expect.
func parseCertChainPEM(pemData string) (leaf string, chain []string, err error) {
	rest := []byte(pemData)
	var ders []string
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		ders = append(ders, base64.StdEncoding.EncodeToString(block.Bytes))
	}
	if len(ders) == 0 {
		return "", nil, fmt.Errorf("no CERTIFICATE block in x5chain PEM")
	}
	return ders[0], ders[1:], nil
}
