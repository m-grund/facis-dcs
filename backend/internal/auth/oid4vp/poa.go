package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PoAVCT is the DCS Proof of Authority credential type (version 1).
const PoAVCT = "urn:dcs:poa:v1"

// CredentialJWTTyp is the JWT typ for issued dc+sd-jwt PoA credentials.
const CredentialJWTTyp = "dc+sd-jwt"

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

func LoadDCQLQuery(raw string) (any, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return DefaultDCQLQuery(), nil
	}

	var q any

	err := json.Unmarshal([]byte(raw), &q)
	if err != nil {
		return nil, fmt.Errorf("invalid OID4VP_DCQL_QUERY JSON: %w", err)
	}

	return q, nil
}
