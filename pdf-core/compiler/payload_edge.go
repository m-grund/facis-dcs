package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	// Build the compaction context: start from any prefix bindings in the
	// original payload (so user-declared namespaces like odrl are preserved in
	// the canonical form), then overlay the stable bindings so core vocabulary
	// terms are always normalised regardless of what the submitter called them.
	var rawDoc map[string]any
	json.Unmarshal(raw, &rawDoc) //nolint:errcheck // already validated above
	compactCtx := map[string]any{}
	if rawCtx, ok := rawDoc["@context"].(map[string]any); ok {
		for k, v := range rawCtx {
			if _, isStr := v.(string); isStr && !strings.HasPrefix(k, "@") {
				compactCtx[k] = v
			}
		}
	}
	stableCtx := map[string]any{
		"@vocab":       dcsCoreIRI,
		"dcs-pdf-core": dcsCoreIRI,
		"dcterms":      "http://purl.org/dc/terms/",
		"schema":       "https://schema.org/",
		"prov":         "http://www.w3.org/ns/prov#",
	}
	for k, v := range stableCtx {
		compactCtx[k] = v
	}
	compacted, err := proc.Compact(expanded, compactCtx, ld.NewJsonLdOptions(""))
	if err != nil {
		return nil, fmt.Errorf("JSON-LD compaction failed: %w", err)
	}
	if _, hasType := compacted["@type"]; !hasType {
		compacted["@type"] = "dcs-pdf-core:Document"
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
	// First ensure the payload is valid JSON-LD and can be expanded.
	_, _, err := NormalizePayload(raw)
	if err != nil {
		return err
	}

	shapePath := filepath.Join("ontology", "generated", "dcs-pdf-core.shacl.ttl")
	if _, statErr := os.Stat(shapePath); statErr != nil {
		return fmt.Errorf("SHACL shapes not found at %s: %w", shapePath, statErr)
	}
	shapesGraph, err := shacl.LoadTurtleFile(shapePath)
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
