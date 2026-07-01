package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/piprate/json-gold/ld"
)

// NormalizePayload applies URDNA2015 (Universal RDF Dataset Normalization
// Algorithm 2015) to the JSON-LD payload, producing the deterministic N-Quads
// byte stream used for hashing and as the FileID seed.
//
// Returns:
//
//	nquads – URDNA2015-canonical N-Quads (used for SHA-256 FileID hashing).
//	err
func NormalizePayload(raw []byte) (nquads []byte, err error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}

	proc := ld.NewJsonLdProcessor()

	normalizeOpts := ld.NewJsonLdOptions("")
	normalizeOpts.Algorithm = "URDNA2015"
	normalizeOpts.Format = "application/n-quads"
	normResult, err := proc.Normalize(doc, normalizeOpts)
	if err != nil {
		return nil, fmt.Errorf("URDNA2015 normalization failed: %w", err)
	}
	normalized, ok := normResult.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected normalization result type %T", normResult)
	}
	return []byte(normalized), nil
}
