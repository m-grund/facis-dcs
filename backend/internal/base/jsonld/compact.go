// Package jsonld re-compacts JSON-LD documents against the FACIS DCS context.
package jsonld

import (
	_ "embed"
	"encoding/json"
	"fmt"

	ld "github.com/piprate/json-gold/ld"
)

//go:embed facis-context.jsonld
var facisContextBytes []byte

//go:embed pdfcore-context.jsonld
var pdfCoreContextBytes []byte

// CompactToFacis re-compacts a JSON-LD document against the FACIS DCS context,
// restoring the dcs:/dcst:/dcterms: prefixes and term aliases (parameterValue,
// derivedFromTemplate, …) the DCS stores and queries contract data by.
//
// pdf-core canonicalizes the payload against its LinkML-generated context, whose
// @vocab emits bare ontology terms (parentContract, metadata, policies), and
// embeds that canonical form in the contract PDF. A peer that rebuilds its copy
// from an extracted payload would otherwise hold those bare terms where the
// originator holds compact terms (dcs:parentContract) — diverging the two copies
// and breaking every contract_data->'dcs:…' JSON path. Re-compacting on receipt
// restores the originator's representation (ADR-13).
//
// The expansion inlines pdf-core's context so json-gold never fetches the
// payload's remote @context URL. The input's own @context value is preserved on
// the output.
func CompactToFacis(doc []byte) ([]byte, error) {
	var input map[string]any
	if err := json.Unmarshal(doc, &input); err != nil {
		return nil, fmt.Errorf("parse json-ld: %w", err)
	}
	originalContext, hasContext := input["@context"]

	pdfCoreCtx, err := contextObject(pdfCoreContextBytes)
	if err != nil {
		return nil, fmt.Errorf("load pdf-core context: %w", err)
	}
	facisCtx, err := contextObject(facisContextBytes)
	if err != nil {
		return nil, fmt.Errorf("load facis context: %w", err)
	}

	// Inline pdf-core's context so expansion resolves the payload's terms without
	// an HTTP fetch of the remote @context URL it was serialized with.
	input["@context"] = pdfCoreCtx

	proc := ld.NewJsonLdProcessor()
	options := ld.NewJsonLdOptions("")
	expanded, err := proc.Expand(input, options)
	if err != nil {
		return nil, fmt.Errorf("expand payload: %w", err)
	}
	compacted, err := proc.Compact(expanded, facisCtx, options)
	if err != nil {
		return nil, fmt.Errorf("compact to facis context: %w", err)
	}

	if hasContext {
		compacted["@context"] = originalContext
	}
	return json.Marshal(compacted)
}

// contextObject unmarshals a JSON-LD context document and returns its @context
// value for use as a json-gold expand/compact argument.
func contextObject(b []byte) (any, error) {
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse context document: %w", err)
	}
	ctx, ok := doc["@context"]
	if !ok {
		return nil, fmt.Errorf("context document has no @context key")
	}
	return ctx, nil
}
