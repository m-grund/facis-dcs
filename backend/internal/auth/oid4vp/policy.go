package oid4vp

import (
	"encoding/json"
	"fmt"
)

// CheckCredentialRevocation validates credentialStatus / bitstring status list when present.
// TODO: resolve and evaluate W3C Bitstring Status List v1.0; reject revoked credentials.
// Until OCM-W / status service is available, presentations pass this check.
func CheckCredentialRevocation(rawClaims json.RawMessage) error {
	var claims map[string]any
	if len(rawClaims) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawClaims, &claims); err != nil {
		return fmt.Errorf("parse credential claims for revocation check: %w", err)
	}
	_, hasStatus := claims["credentialStatus"]
	_ = hasStatus
	// Deferred: return error when status list reports revoked.
	return nil
}

// EvaluateLoginPolicy decides granted roles from disclosed claims.
// TODO: evaluate Rego policy from OID4VP_POLICY_PATH (issuer trust, expiry, disclosure rules).
// Until OPA is wired, all disclosed roles from the verified VP are granted.
func EvaluateLoginPolicy(verified *VerifiedLoginClaims) ([]string, error) {
	if verified == nil {
		return nil, fmt.Errorf("verified claims are required")
	}
	if len(verified.Roles) == 0 {
		return nil, fmt.Errorf("no roles disclosed in presentation")
	}
	return verified.Roles, nil
}

// CheckDisclosedClaimsMeetDCQL confirms the VP satisfies the active DCQL query.
// TODO: enforce selective disclosure and DCQL claim requirements beyond vct/format checks.
// Until full SD-JWT / OCM-W support, verified credential claims are accepted when cryptographic checks pass.
func CheckDisclosedClaimsMeetDCQL(_ json.RawMessage) error {
	return nil
}
