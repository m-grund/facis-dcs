package pdfgeneration

import (
	"encoding/json"
	"fmt"
	"strings"
)

// dcsCoreVocabIRI is the base IRI for all pdf-core terms.
// Set once at startup via SetVocabIRI before any MarshalJSONLD call.
var dcsCoreVocabIRI string

// SetVocabIRI configures the vocab IRI. Must be called at startup (cmd/dcs/main.go).
func SetVocabIRI(iri string) {
	dcsCoreVocabIRI = iri
}

// MarshalJSONLD converts a raw JSON-LD data blob and entity name into a
// fully term-expanded JSON object for pdf-core. All compact property names
// become absolute IRIs using the dcs-pdf-core vocab; @context is dropped.
// DCS is authoritative about the term mapping — all document structure terms
// live in the dcs-pdf-core ontology, so no prefix table is needed.
func MarshalJSONLD(data []byte, name *string) ([]byte, error) {
	if dcsCoreVocabIRI == "" {
		return nil, fmt.Errorf("pdfgeneration: vocab IRI not configured; call SetVocabIRI at startup")
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal JSON-LD data: %w", err)
	}

	if name != nil && *name != "" {
		raw["title"] = *name
	}

	return json.Marshal(expandObject(raw))
}

func expandObject(obj map[string]any) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		switch k {
		case "@context":
			// Drop — full IRI keys are self-contained.
		case "@type":
			out[k] = expandTypeValues(v)
		case "@id", "@value", "@language", "@graph", "@list", "@set":
			out[k] = v
		default:
			out[expandTerm(k)] = expandValue(v)
		}
	}
	return out
}

func expandValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return expandObject(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = expandValue(item)
		}
		return out
	default:
		return v
	}
}

func expandTypeValues(v any) any {
	switch val := v.(type) {
	case string:
		return expandTerm(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			if s, ok := item.(string); ok {
				out[i] = expandTerm(s)
			} else {
				out[i] = item
			}
		}
		return out
	default:
		return v
	}
}

func expandTerm(term string) string {
	if strings.HasPrefix(term, "@") {
		return term
	}
	if strings.Contains(term, "://") {
		return term // already absolute IRI
	}
	return dcsCoreVocabIRI + term
}
