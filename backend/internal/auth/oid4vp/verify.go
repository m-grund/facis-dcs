package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/base/datatype/userrole"
)

// PresentationContext carries OpenID4VP request parameters required for verification.
type PresentationContext struct {
	Nonce    string
	ClientID string
}

// VerifiedLoginClaims holds subject and roles extracted from a verified VP.
type VerifiedLoginClaims struct {
	SubjectDID     string
	ParticipantDID string
	Roles          []string
	GrantedRoles   []string
	RawClaims      json.RawMessage
}

// Verifier validates a wallet presentation and returns login claims.
type Verifier interface {
	Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error)
}

// NewVerifier returns a VP verifier backed by the given issuer trust configuration.
func NewVerifier(cfg *TrustConfig) Verifier {
	if cfg == nil {
		return unconfiguredVerifier{}
	}
	return verifier{trust: cfg}
}

type verifier struct {
	trust *TrustConfig
}

type unconfiguredVerifier struct{}

func (unconfiguredVerifier) Verify(_ string, _ PresentationContext) (*VerifiedLoginClaims, error) {
	return nil, fmt.Errorf("oid4vp trust config is not loaded (set OID4VP_TRUST_DATA_PATH)")
}

func (v verifier) Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error) {
	// Policy verification steps, in order of execution:
	// 1. trust list + wallet binding
	// 2. status list
	// 3. login roles
	verified, err := verifyTrustAndWallet(vpToken, ctx, v.trust)
	if err != nil {
		return nil, err
	}

	err = checkStatusList(verified.RawClaims)
	if err != nil {
		return nil, err
	}

	granted, err := evaluateLoginRoles(verified.Roles)
	if err != nil {
		return nil, err
	}
	verified.GrantedRoles = granted

	return verified, nil
}

func verifyTrustAndWallet(vpToken string, ctx PresentationContext, trust *TrustConfig) (*VerifiedLoginClaims, error) {
	if trust == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	presentation, err := sdjwt.ParsePresentation(vpToken)
	if err != nil {
		return nil, err
	}

	issuerClaims, err := sdjwt.VerifyCredential(presentation.IssuerJWT, presentation.Disclosures, trust)
	if err != nil {
		return nil, err
	}

	cnfJWK, err := sdjwt.CNFJWKFromClaims(issuerClaims)
	if err != nil {
		return nil, err
	}

	sub, _ := issuerClaims["sub"].(string)
	sub = strings.TrimSpace(sub)

	if sub == "" {
		return nil, fmt.Errorf("credential missing sub")
	}

	expectedSub, err := sdjwt.DIDJWKFromPublicJWK(cnfJWK)
	if err != nil {
		return nil, fmt.Errorf("credential cnf.jwk: %w", err)
	}

	if sub != expectedSub {
		return nil, fmt.Errorf("credential sub does not match cnf.jwk holder binding")
	}

	err = sdjwt.VerifyKB(presentation.KBJWT, presentation.SDHash, cnfJWK, sub, ctx.Nonce, ctx.ClientID)
	if err != nil {
		return nil, err
	}

	roles, err := sdjwt.RolesFromClaims(issuerClaims)
	if err != nil {
		return nil, err
	}

	organization, err := sdjwt.OrganizationFromClaims(issuerClaims)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(issuerClaims)
	if err != nil {
		return nil, err
	}

	return &VerifiedLoginClaims{
		SubjectDID:     sub,
		ParticipantDID: organization,
		Roles:          roles,
		RawClaims:      raw,
	}, nil
}

// checkStatusList validates the credential against the W3C Bitstring Status List.
// TODO: resolve and evaluate W3C Bitstring Status List v1.0; reject revoked credentials.
func checkStatusList(rawClaims json.RawMessage) error {
	var claims map[string]any
	if len(rawClaims) == 0 {
		return nil
	}

	err := json.Unmarshal(rawClaims, &claims)
	if err != nil {
		return fmt.Errorf("parse credential claims for status list check: %w", err)
	}

	_, hasStatus := claims["credentialStatus"]
	_ = hasStatus

	return nil
}

// evaluateLoginRoles applies login authorization policy to disclosed roles.
// TODO: evaluate Rego policy from OID4VP_POLICY_PATH.
func evaluateLoginRoles(disclosedRoles []string) ([]string, error) {
	if len(disclosedRoles) == 0 {
		return nil, fmt.Errorf("no roles disclosed in presentation")
	}

	granted := make([]string, 0, len(disclosedRoles))
	for _, role := range disclosedRoles {
		ur, err := userrole.NewUserRole(role)
		if err != nil {
			return nil, fmt.Errorf("invalid disclosed role %q: %w", role, err)
		}
		granted = append(granted, ur.String())
	}

	return granted, nil
}
