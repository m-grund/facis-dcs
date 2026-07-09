package request

import (
	"crypto"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// HSMSigner signs OpenID4VP authorization request JWTs (JAR) with an ECDSA
// P-256 key held in the PKCS#11 token. The signing key's HSM label is used as
// the JWT "kid" and its public key is embedded in the header "jwk" so a
// verifier can check the signature against the request object itself.
type HSMSigner struct {
	kid    string
	signer crypto.Signer
	jwk    map[string]any
	// signES256 converts a signing input into a 64-byte r||s ES256 signature.
	signES256 func(crypto.Signer, []byte) ([]byte, error)
}

// NewHSMSigner builds a JAR signer over the given HSM key. label is both the
// PKCS#11 CKA_LABEL used as the JWT kid and the identifier a verifier resolves.
func NewHSMSigner(label string, signer crypto.Signer, publicJWK map[string]any, signES256 func(crypto.Signer, []byte) ([]byte, error)) (*HSMSigner, error) {
	if label == "" {
		return nil, fmt.Errorf("hsm key label is required for JAR signing")
	}
	if signer == nil {
		return nil, fmt.Errorf("hsm signer is required for JAR signing")
	}
	if signES256 == nil {
		return nil, fmt.Errorf("es256 signing function is required for JAR signing")
	}
	return &HSMSigner{
		kid:       label,
		signer:    signer,
		jwk:       publicJWK,
		signES256: signES256,
	}, nil
}

// SignAuthorizationRequestJWT returns a compact oauth-authz-req+jwt signed by
// the HSM key, with the public JWK embedded in the header.
func (s *HSMSigner) SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error) {
	if s == nil {
		return "", fmt.Errorf("hsm request signer is not configured")
	}
	return signES256JWT(s.kid, claims, s.jwk, func(signingInput string) ([]byte, error) {
		return s.signES256(s.signer, []byte(signingInput))
	})
}
