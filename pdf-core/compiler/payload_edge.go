package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/piprate/json-gold/ld"
	"github.com/tggo/goRDFlib/shacl"
)

// PayloadSHACLValidationError represents a non-conformant SHACL report.
type PayloadSHACLValidationError struct {
	Report string
}

func (e *PayloadSHACLValidationError) Error() string {
	report := strings.TrimSpace(e.Report)
	if report == "" {
		return "payload failed SHACL validation"
	}
	return "payload failed SHACL validation: " + report
}

var shaclShapes []byte

// SetSHACLBytes stores the SHACL shapes graph used by ValidatePayloadSHACL.
// Must be called before any validation is attempted.
func SetSHACLBytes(b []byte) {
	shaclShapes = b
}

// dcsListProperties are the DCS ontology properties whose SHACL shapes use
// RDF list path traversal (sh:path (prop rdf:rest* rdf:first)). Their values
// must be RDF lists so SHACL member-type/class constraints fire on each element.
//
// Exclusions:
//   - dcs:layout: SHACL uses sh:path dcs:layout + sh:class dcs:LayoutNode
//     (plain triple). Wrapping as RDF list makes the list HEAD the triple object,
//     failing the class constraint.
//   - dcs:children: values are string literals (not IRI nodes) in practice;
//     wrapping as RDF list exposes them to sh:nodeKind sh:IRI via the list path,
//     which would fail. Plain-triple path is vacuously valid for literals.
//   - dcs:signatureFields: no list-path SHACL shape.
//
// json-gold does not propagate ExpandContext annotations to compact-IRI
// properties (e.g. dcs:blocks), so we normalise the expanded form explicitly.
var dcsListProperties = map[string]bool{
	dcsOntologyIRI + "blocks":  true,
	dcsOntologyIRI + "content": true,
}

// CanonicalizePayload disambiguates JSON-LD at the application edge by running
// an expand+compact pass with a stable context so semantically equivalent
// payload flavors serialize to the same canonical JSON representation.
func CanonicalizePayload(raw []byte) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}

	proc := ld.NewJsonLdProcessor()
	expanded, err := proc.Expand(doc, ld.NewJsonLdOptions(""))
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion failed: %w", err)
	}

	// Normalise list properties to RDF list form. json-gold matches
	// @container:@list during compaction only against {"@list":[...]} values,
	// so we must ensure all list-typed properties carry that structure after
	// expansion regardless of whether the submitter used prefix notation or
	// @list syntax.
	normalizeExpandedListProps(expanded)

	// stableCtx is the authoritative compaction context for canonical payloads.
	// - No dcs: prefix: when both a prefix and a term map to the same IRI,
	//   json-gold prefers the prefix during compaction. Omitting the prefix
	//   forces the short term name to win.
	// - @container:@list: preserves RDF list structure in the canonical form so
	//   SHACL member-type constraints remain effective on round-tripped payloads.
	// - Full absolute IRI for @id: compact IRI (dcs:X) is ambiguous when the
	//   prefix is absent from the context.
	stableCtx := map[string]any{
		"@vocab":          dcsOntologyIRI,
		"dcterms":         "http://purl.org/dc/terms/",
		"schema":          "https://schema.org/",
		"prov":            "http://www.w3.org/ns/prov#",
		// @container:@list — SHACL traverses members via rdf:rest*/rdf:first
		"blocks":  map[string]any{"@id": dcsOntologyIRI + "blocks",  "@container": "@list"},
		"content": map[string]any{"@id": dcsOntologyIRI + "content", "@container": "@list"},
		// @container:@set — plain-triple SHACL shapes; @list would break class/nodeKind constraints
		"children":        map[string]any{"@id": dcsOntologyIRI + "children",        "@container": "@set"},
		"layout":          map[string]any{"@id": dcsOntologyIRI + "layout",          "@container": "@set"},
		"signatureFields": map[string]any{"@id": dcsOntologyIRI + "signatureFields", "@container": "@set"},
	}
	compacted, err := proc.Compact(expanded, stableCtx, ld.NewJsonLdOptions(""))
	if err != nil {
		return nil, fmt.Errorf("JSON-LD compaction failed: %w", err)
	}

	b, err := json.Marshal(compacted)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON-LD: %w", err)
	}
	return b, nil
}

// ValidatePayloadSHACL validates JSON-LD against LinkML-generated SHACL using
// a backend Go SHACL library.
func ValidatePayloadSHACL(raw []byte) error {
	if len(shaclShapes) == 0 {
		return fmt.Errorf("SHACL shapes not initialised; call compiler.SetSHACLBytes before validating")
	}

	// First ensure the payload is valid JSON-LD and can be expanded.
	_, err := NormalizePayload(raw)
	if err != nil {
		return err
	}

	shapesGraph, err := shacl.LoadTurtleString(string(shaclShapes), "")
	if err != nil {
		return fmt.Errorf("load SHACL shapes: %w", err)
	}
	dataGraph, err := shacl.LoadJsonLDString(string(raw), "")
	if err != nil {
		return fmt.Errorf("load JSON-LD data graph: %w", err)
	}
	report := shacl.Validate(dataGraph, shapesGraph)
	if report.Conforms {
		return nil
	}
	return &PayloadSHACLValidationError{Report: formatSHACLReport(report)}
}

func formatSHACLReport(report shacl.ValidationReport) string {
	if len(report.Results) == 0 {
		return "payload does not conform to SHACL shapes"
	}
	results := flattenValidationResults(report.Results)
	sort.Slice(results, func(i, j int) bool {
		return results[i].ResultPath.String() < results[j].ResultPath.String()
	})
	parts := make([]string, 0, len(results))
	for _, r := range results {
		focus := strings.TrimSpace(r.FocusNode.String())
		path := strings.TrimSpace(r.ResultPath.String())
		component := strings.TrimSpace(r.SourceConstraintComponent.String())
		message := ""
		if len(r.ResultMessages) > 0 {
			message = strings.TrimSpace(r.ResultMessages[0].Value())
		}
		if message == "" {
			message = strings.TrimSpace(r.Value.String())
		}
		if message == "" {
			message = "constraint violation"
		}
		parts = append(parts, fmt.Sprintf("focus=%s path=%s component=%s message=%s", focus, path, component, message))
	}
	return strings.Join(parts, "; ")
}

func flattenValidationResults(results []shacl.ValidationResult) []shacl.ValidationResult {
	flat := make([]shacl.ValidationResult, 0, len(results))
	var walk func(shacl.ValidationResult)
	walk = func(r shacl.ValidationResult) {
		flat = append(flat, r)
		for _, d := range r.Details {
			walk(d)
		}
	}
	for _, r := range results {
		walk(r)
	}
	return flat
}

// normalizeExpandedListProps brings the expanded form into a consistent shape
// before compaction:
//
//   - dcsListProperties (blocks, content): plain arrays are wrapped as
//     {"@list":[...]} so the @container:@list compaction term matches and
//     SHACL list-path constraints (rdf:rest*/rdf:first) can fire.
//   - All other array properties: explicit {"@list":[...]} wrappers are stripped
//     to plain arrays so @container:@set compaction produces clean JSON arrays
//     regardless of whether the submitter used @list syntax or not.
func normalizeExpandedListProps(expanded []any) {
	for _, node := range expanded {
		if m, ok := node.(map[string]any); ok {
			normalizeNodeListProps(m)
		}
	}
}

func normalizeNodeListProps(node map[string]any) {
	for k, v := range node {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		if dcsListProperties[k] {
			// Already an RDF list — recurse into inner items and leave wrapper
			if len(arr) == 1 {
				if listObj, ok := arr[0].(map[string]any); ok {
					if inner, has := listObj["@list"]; has {
						if innerArr, ok := inner.([]any); ok {
							for _, item := range innerArr {
								if m, ok := item.(map[string]any); ok {
									normalizeNodeListProps(m)
								}
							}
						}
						continue
					}
				}
			}
			// Plain array — recurse into items then wrap
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					normalizeNodeListProps(m)
				}
			}
			node[k] = []any{map[string]any{"@list": arr}}
		} else {
			// Non-list property: strip any @list wrapper to a plain array
			if len(arr) == 1 {
				if listObj, ok := arr[0].(map[string]any); ok {
					if inner, has := listObj["@list"]; has {
						if innerArr, ok := inner.([]any); ok {
							arr = innerArr
							node[k] = arr
						}
					}
				}
			}
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					normalizeNodeListProps(m)
				}
			}
		}
	}
}
