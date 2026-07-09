// Package signer produces PAdES contract signatures (DCS-IR-SI-10,
// DCS-FR-SM-16). The backend keeps all private-key material inside its PKCS#11
// token (DCS-IR-HI-01); a ContractSigner never handles raw key material. It
// hands a PDF to pdf-core, which builds the CMS to-be-signed bytes and calls
// the backend's authenticated /internal/pades/sign endpoint for the ECDSA
// operation.
package signer

import (
	"context"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

// ContractSigner embeds signing evidence into a PDF and applies a PAdES
// signature over it (embed-first-sign-second: the evidence is covered by the
// PAdES ByteRange).
type ContractSigner interface {
	// SignPDF embeds evidence (when non-empty) as a PDF attachment, then returns
	// a PAdES-signed copy of pdf with the signature placed in the AcroForm
	// signature field named fieldName and signatoryName bound as the CMS signer
	// name. The signed PDF bytes are returned; no IPFS/DB side effects occur.
	SignPDF(ctx context.Context, pdf []byte, fieldName, signatoryName string, evidence []byte) ([]byte, error)
}

// PDFCoreSigner delegates PAdES signing to the pdf-core microservice.
type PDFCoreSigner struct {
	PDFCore *pdfcore.Client
}

// NewPDFCoreSigner returns a ContractSigner backed by pdf-core.
func NewPDFCoreSigner(pdfCore *pdfcore.Client) *PDFCoreSigner {
	return &PDFCoreSigner{PDFCore: pdfCore}
}

// SignPDF implements ContractSigner by calling pdf-core POST /sign.
func (s *PDFCoreSigner) SignPDF(ctx context.Context, pdf []byte, fieldName, signatoryName string, evidence []byte) ([]byte, error) {
	signed, _, err := s.PDFCore.Sign(ctx, pdf, fieldName, signatoryName, evidence)
	return signed, err
}
