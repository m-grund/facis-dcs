package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PIDVCT is the German EUDI PID credential type.
const PIDVCT = "urn:eudi:pid:de:1"

// PlaygroundPIDVCT is returned by the EUDIPLO playground issuer when credentialId is "pid".
const PlaygroundPIDVCT = "urn:eudi:eaa:loyalty-card:1"

const PIDCredentialQueryID = "eudi_pid_credential"

// DefaultPIDDCQLQuery requests a dc+sd-jwt PID credential for identity presentation.
// Override the full query via OID4VP_PID_DCQL_QUERY when needed.
func DefaultPIDDCQLQuery() map[string]any {
	return map[string]any{
		"credentials": []any{
			map[string]any{
				"id":     PIDCredentialQueryID,
				"format": "dc+sd-jwt",
				"meta": map[string]any{
					"vct_values": []string{PIDVCT, PlaygroundPIDVCT},
				},
				"require_cryptographic_holder_binding": true,
			},
		},
	}
}

func LoadPIDDCQLQuery(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultPIDDCQLQuery(), nil
	}

	var q any
	err := json.Unmarshal([]byte(raw), &q)
	if err != nil {
		return nil, fmt.Errorf("invalid OID4VP_PID_DCQL_QUERY JSON: %w", err)
	}

	return q, nil
}
