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
	var doc struct {
		SignatureFields []struct {
			SignatoryName string `json:"signatoryName"`
		} `json:"signatureFields"`
	}
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return nil
	}
	fields := make([]string, 0, len(doc.SignatureFields))
	seen := map[string]bool{}
	for _, sf := range doc.SignatureFields {
		name := strings.TrimSpace(sf.SignatoryName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		fields = append(fields, name)
	}
	return fields
}
