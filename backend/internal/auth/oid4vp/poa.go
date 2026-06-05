package oid4vp

import "time"

// PoAVCT is the DCS Proof of Authority credential type (version 1).
const PoAVCT = "urn:dcs:poa:v1"

// PoAExampleIssuer is the illustrative issuer from Context.md (pre-issuance PoA example).
const PoAExampleIssuer = "https://issuer.example.org"

// StubPoACredentialTTL is how long stub iat/exp claims remain valid in development.
const StubPoACredentialTTL = 365 * 24 * time.Hour

// BuildStubPoACredentialClaims creates claims for a stub PoA credential, using provided subject/org/roles and current time for iat/exp.
func BuildStubPoACredentialClaims(subject, organization string, roles []string, now time.Time) map[string]any {
	return map[string]any{
		"iss":          PoAExampleIssuer,
		"sub":          subject,
		"vct":          PoAVCT,
		"organization": organization,
		"roles":        roles,
		"iat":          now.Unix(),
		"exp":          now.Add(StubPoACredentialTTL).Unix(),
	}
}

const poaCredentialQueryID = "dcs_poa_credential"

// DefaultDCQLQuery requests a dc+sd-jwt PoA credential for OpenID4VP presentation.
// Override the full query via OID4VP_DCQL_QUERY when needed.
func DefaultDCQLQuery() map[string]any {
	return map[string]any{
		"credentials": []any{
			map[string]any{
				"id":     poaCredentialQueryID,
				"format": "dc+sd-jwt",
				"meta": map[string]any{
					"vct_values": []string{PoAVCT},
				},
				"require_cryptographic_holder_binding": true,
				"claims": []any{
					map[string]any{"path": []string{"organization"}},
					map[string]any{"path": []string{"roles"}},
				},
			},
		},
	}
}
