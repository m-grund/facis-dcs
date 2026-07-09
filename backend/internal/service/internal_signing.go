package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/base64"

	internalsigning "digital-contracting-service/gen/internal_signing"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/hsm"
)

type internalSigningsrvc struct {
	c2paSigner  crypto.Signer
	padesSigner crypto.Signer
	auth.JWTAuthenticator
}

// NewInternalSigning builds the authenticated backend-internal signing service.
// c2paSigner is the PKCS#11 dcs-c2pa key used to sign COSE Sig_structure bytes;
// padesSigner is the PKCS#11 dcs-pades key used to sign CMS SignedAttributes
// digests for PAdES contract signatures. pdf-core holds no key material and
// delegates both to this endpoint (DCS-IR-HI-01).
func NewInternalSigning(jwtAuth auth.JWTAuthenticator, c2paSigner, padesSigner crypto.Signer) internalsigning.Service {
	return &internalSigningsrvc{
		JWTAuthenticator: jwtAuth,
		c2paSigner:       c2paSigner,
		padesSigner:      padesSigner,
	}
}

func (s *internalSigningsrvc) C2paSign(_ context.Context, req *internalsigning.C2PASignRequest) (*internalsigning.C2PASignResponse, error) {
	sigStructure, err := base64.StdEncoding.DecodeString(req.SigStructure)
	if err != nil {
		return nil, internalsigning.MakeBadRequest(err)
	}

	signature, err := hsm.SignES256(s.c2paSigner, sigStructure)
	if err != nil {
		return nil, internalsigning.MakeInternalError(err)
	}

	return &internalsigning.C2PASignResponse{
		Signature: base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// PadesSign signs a pre-computed SHA-256 digest of the CMS SignedAttributes and
// returns the ASN.1 DER ECDSA signature. Unlike C2paSign the PAdES CMS embeds
// the DER form directly, so no r||s conversion is applied.
func (s *internalSigningsrvc) PadesSign(_ context.Context, req *internalsigning.PAdESSignRequest) (*internalsigning.PAdESSignResponse, error) {
	digest, err := base64.StdEncoding.DecodeString(req.Digest)
	if err != nil {
		return nil, internalsigning.MakeBadRequest(err)
	}

	der, err := s.padesSigner.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		return nil, internalsigning.MakeInternalError(err)
	}

	return &internalsigning.PAdESSignResponse{
		Signature: base64.StdEncoding.EncodeToString(der),
	}, nil
}
