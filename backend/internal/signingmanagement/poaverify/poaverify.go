// Package poaverify re-verifies the holder-bound Proof-of-Authority (PoA) SD-JWT
// VC + KB-JWT a signer presents for a signing ceremony (SRS "Signer
// Authorization & PoA Credential Chain Verification", UC-04-02/03, UC-14). The
// same PoA the signer authenticated with at login authorizes the signature:
// being holder-bound (cnf.jwk signs the KB-JWT, and the credential subject is
// the did:jwk of that holder key) it is traceable to the natural person, so no
// separate PID presentation is taken at signing. Verify proves the presentation
// is internally consistent and that the credential is a PoA.
package poaverify

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// Audience is the fixed OID4VP audience/client_id bound into the KB-JWT of a
// PoA presentation for a signing ceremony.
const Audience = "dcs-signature-ceremony"

// Verify validates the presentation and returns the signer DID (the holder-bound
// credential subject — the natural person) and the KB-JWT sd_hash.
func Verify(vpToken string) (signerDID, sdHash string, err error) {
	presentation, err := sdjwt.ParsePresentation(vpToken)
	if err != nil {
		return "", "", err
	}

	issuerClaims := jwt.MapClaims{}
	if _, _, perr := jwt.NewParser().ParseUnverified(presentation.IssuerJWT, &issuerClaims); perr != nil {
		return "", "", fmt.Errorf("parse issuer jwt: %w", perr)
	}

	if vct, _ := issuerClaims["vct"].(string); strings.TrimSpace(vct) != oid4vp.PoAVCT {
		return "", "", fmt.Errorf("signing credential is not a PoA (vct %q, want %q)", vct, oid4vp.PoAVCT)
	}

	cnfJWK, err := sdjwt.CNFJWKFromClaims(issuerClaims)
	if err != nil {
		return "", "", err
	}
	sub, _ := issuerClaims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", "", fmt.Errorf("credential missing sub")
	}
	expectedSub, err := sdjwt.DIDJWKFromPublicJWK(cnfJWK)
	if err != nil {
		return "", "", fmt.Errorf("credential cnf.jwk: %w", err)
	}
	if sub != expectedSub {
		return "", "", fmt.Errorf("credential sub does not match cnf.jwk holder binding")
	}

	// The ceremony webhook does not carry the wallet nonce, so KB verification
	// checks the holder signature, sd_hash and audience against the nonce
	// actually present in the KB-JWT (self-consistency), not a server nonce.
	kbNonce, err := kbJWTNonce(presentation.KBJWT)
	if err != nil {
		return "", "", err
	}
	if err := sdjwt.VerifyKB(presentation.KBJWT, presentation.SDHash, cnfJWK, sub, kbNonce, Audience); err != nil {
		return "", "", err
	}

	return sub, presentation.SDHash, nil
}

func kbJWTNonce(kbJWT string) (string, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(kbJWT, &claims); err != nil {
		return "", fmt.Errorf("parse kb jwt: %w", err)
	}
	nonce, _ := claims["nonce"].(string)
	if strings.TrimSpace(nonce) == "" {
		return "", fmt.Errorf("kb jwt missing nonce")
	}
	return nonce, nil
}
