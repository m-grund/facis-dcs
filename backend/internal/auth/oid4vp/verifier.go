package oid4vp

import (
	"encoding/json"
	"fmt"
)

// PresentationContext carries OpenID4VP request parameters required for verification.
type PresentationContext struct {
	Nonce    string
	ClientID string
}

// VerifiedLoginClaims holds subject and roles extracted from a verified VP.
type VerifiedLoginClaims struct {
	SubjectDID     string
	OrganizationID string
	Roles          []string
	RawClaims      json.RawMessage
}

// Verifier validates a wallet presentation and returns login claims.
type Verifier interface {
	Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error)
}

// NewVerifier returns the active VP verifier (embedded JWKS trust anchor; OCM-W swappable later).
func NewVerifier(trust *TrustConfig) Verifier {
	if trust == nil {
		return &unconfiguredVerifier{}
	}
	return NewTrustVerifier(trust)
}

type unconfiguredVerifier struct{}

func (u *unconfiguredVerifier) Verify(_ string, _ PresentationContext) (*VerifiedLoginClaims, error) {
	return nil, fmt.Errorf("oid4vp trust config is not loaded (set OID4VP_TRUST_DATA_PATH)")
}
