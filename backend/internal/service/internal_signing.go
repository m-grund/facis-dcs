package service

import (
	"context"
	"crypto"
	"encoding/base64"

	internalsigning "digital-contracting-service/gen/internal_signing"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/hsm"
)

type internalSigningsrvc struct {
	c2paSigner crypto.Signer
	auth.JWTAuthenticator
}

// NewInternalSigning builds the authenticated backend-internal signing service.
// c2paSigner is the PKCS#11 dcs-c2pa key used to sign COSE Sig_structure bytes;
// pdf-core holds no key material and delegates to this endpoint (DCS-IR-HI-01).
func NewInternalSigning(jwtAuth auth.JWTAuthenticator, c2paSigner crypto.Signer) internalsigning.Service {
	return &internalSigningsrvc{
		JWTAuthenticator: jwtAuth,
		c2paSigner:       c2paSigner,
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
