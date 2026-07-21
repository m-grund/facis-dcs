package validation

import (
	"encoding/json"
	"strings"
)

// RequiredSignatureFields returns the contract's declared signature-field
// names (dcs:SignatureField nodes' signatoryName, the AcroForm field name
// pdf-core renders and /sign targets — see pdf-core/compiler/dcs_schema.go).
// An empty result means the contract declares no explicit signature fields
// and follows the single-signature flow (DCS-FR-SM-07/-17: contracts that
// require multiple signatures declare one field per signatory).
func RequiredSignatureFields(contractData []byte) []string {
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return nil
	}
	// The declaration may carry the prefixed or the JSON-LD-compacted term,
	// depending on the contract-data form in hand.
	raw, ok := doc["dcs:signatureFields"].([]any)
	if !ok {
		raw, _ = doc["signatureFields"].([]any)
	}
	fields := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := node["dcs:signatoryName"].(string)
		if name == "" {
			name, _ = node["signatoryName"].(string)
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		fields = append(fields, name)
	}
	return fields
}
