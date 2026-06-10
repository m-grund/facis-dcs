package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/piprate/json-gold/ld"
)

// NormalizePayload runs the JSON-LD → RDF pipeline:
//  1. Normalize  – applies URDNA2015 (Universal RDF Dataset Normalization
//                  Algorithm 2015) to the dataset, producing the deterministic
//                  N-Quads byte stream used for hashing and as the FileID seed.
//  2. Expand     – produces the JSON-LD expanded form where every property name
//                  is a full IRI, used by extractDocumentModel to extract the
//                  document model without relying on verbatim JSON key names.
//
// Returns:
//
//	nquads   – URDNA2015-canonical N-Quads (used for SHA-256 FileID hashing).
//	expanded – JSON-LD expanded form: []any of fully-qualified node maps.
//	err
func NormalizePayload(raw []byte) (nquads []byte, expanded []any, err error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}
	_ = root // used below for context extraction only

	proc := ld.NewJsonLdProcessor()

	// URDNA2015 canonical N-Quads.
	normalizeOpts := ld.NewJsonLdOptions("")
	normalizeOpts.Algorithm = "URDNA2015"
	normalizeOpts.Format = "application/n-quads"
	normResult, err := proc.Normalize(doc, normalizeOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("URDNA2015 normalization failed: %w", err)
	}
	normalized, ok := normResult.(string)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected normalization result type %T", normResult)
	}

	// JSON-LD expansion: resolves all compact IRIs and @vocab terms to full IRIs.
	expandOpts := ld.NewJsonLdOptions("")
	expandedResult, err := proc.Expand(doc, expandOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("JSON-LD expansion failed: %w", err)
	}

	return []byte(normalized), expandedResult, nil
}
