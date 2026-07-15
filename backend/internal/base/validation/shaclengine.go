package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piprate/json-gold/ld"
	"github.com/tggo/goRDFlib/jsonld"
	"github.com/tggo/goRDFlib/shacl"
)

// validateAgainstHubShapes checks contract (an already-decoded JSON-LD
// document map) against the Semantic Hub's SHACL shapes using goRDFlib
// (github.com/tggo/goRDFlib/shacl), a conformant SHACL-core processor
// verified against the W3C SHACL/SHACL-1.2 test suites (ADR-9) — replacing
// the hand-rolled structural-subset matcher this package used to carry.
//
// The shapes version is the one pinned in the document's own
// dcs:schemaRefs.dcs:shaclShapes anchor when present (ADR-8 revalidation),
// otherwise the hub's currently-active version (new document validation).
// Returns the findings and the shapes version they were produced against.
func validateAgainstHubShapes(ctx context.Context, contract map[string]any) ([]PolicyFinding, int, error) {
	source, err := requireShapeSource()
	if err != nil {
		return nil, 0, err
	}

	var shapesTTL string
	var shapesVersion int
	if pinned := pinnedHubShapesVersion(contract); pinned > 0 {
		shapesTTL, err = source.ShapesAt(ctx, pinned)
		shapesVersion = pinned
	} else {
		shapesTTL, shapesVersion, err = source.ActiveShapes(ctx)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load SHACL shapes: %w", err)
	}

	contextContent, _, err := source.ActiveContext(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("load active JSON-LD context: %w", err)
	}
	loader, err := hermeticContextLoader(contextContent)
	if err != nil {
		return nil, 0, err
	}

	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return nil, 0, fmt.Errorf("encode contract document: %w", err)
	}

	dataGraph, err := shacl.LoadJsonLDString(string(contractJSON), "urn:dcs:contract", jsonld.WithDocumentLoader(loader))
	if err != nil {
		return nil, 0, fmt.Errorf("parse contract document as JSON-LD: %w", err)
	}
	shapesGraph, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:hub:shapes")
	if err != nil {
		return nil, 0, fmt.Errorf("parse SHACL shapes (hub version %d): %w", shapesVersion, err)
	}

	report := shacl.Validate(dataGraph, shapesGraph)
	return mapShaclReport(report, shapesVersion), shapesVersion, nil
}

// mapShaclReport translates a goRDFlib sh:ValidationReport into the
// PolicyFinding shape every other audit source in this package produces
// (task 2.3), so downstream consumers — the PACM contract-content audit
// trail, the signature/compliance viewer — are unaffected by which engine
// ran. SHACL itself only reports non-conformant results (sh:Violation/
// sh:Warning/sh:Info) — there is no per-property "conforms" finding to
// synthesize, unlike the deleted subset matcher's noisier "X conforms" info
// entries.
func mapShaclReport(report shacl.ValidationReport, shapesVersion int) []PolicyFinding {
	findings := make([]PolicyFinding, 0, len(report.Results))
	for _, result := range report.Results {
		findings = append(findings, shaclResultFinding(result, shapesVersion))
	}
	return findings
}

func shaclResultFinding(result shacl.ValidationResult, shapesVersion int) PolicyFinding {
	// SourceShape is frequently a blank node (every inline sh:property [...]
	// shape is anonymous) — not a stable identifier across parses/versions.
	// ResultPath (a real predicate IRI whenever the violation is a property
	// constraint) is: prefer it for the rule ID, falling back to the shape
	// IRI only for node-level violations (sh:targetClass/sh:nodeKind
	// mismatches), which name a real, non-blank NodeShape.
	shapeName := shaclLocalName(termValue(result.SourceShape))
	componentName := shaclLocalName(termValue(result.SourceConstraintComponent))
	pathName := shaclLocalName(termValue(result.ResultPath))
	focusNode := termValue(result.FocusNode)

	ruleID := pathName
	if ruleID == "" {
		ruleID = shapeName
	}
	if componentName != "" {
		ruleID += "-" + componentName
	}

	message := joinResultMessages(result.ResultMessages)
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("%s: constraint %s violated at %s", shapeName, componentName, focusNode)
		if pathName != "" {
			message = fmt.Sprintf("%s: %s must satisfy %s (focus node %s)", shapeName, pathName, componentName, focusNode)
		}
	} else if focusNode != "" {
		message = fmt.Sprintf("%s (focus node %s)", message, focusNode)
	}

	finding := contractFinding(ruleID, shapeName, shaclResultSeverity(result), message, pathName, pathName, termValue(result.SourceShape))
	finding.ActualValue = shaclFindingValue(result.Value)
	finding.Operator = componentName
	finding.ShapesVersion = shapesVersion
	return finding
}

func shaclResultSeverity(result shacl.ValidationResult) string {
	switch termValue(result.ResultSeverity) {
	case shacl.SHWarning.Value():
		return "warning"
	case shacl.SHInfo.Value():
		return "info"
	case "":
		return "error"
	default:
		// sh:Violation and any custom/debug/trace severity goRDFlib passes
		// through (e.g. SHACL 1.2's sh:Debug/sh:Trace) — treat anything not
		// explicitly Warning/Info as blocking, matching Validate's own
		// sh:conforms computation.
		return "error"
	}
}

func shaclFindingValue(t shacl.Term) any {
	v := termValue(t)
	if v == "" {
		return nil
	}
	return v
}

func joinResultMessages(messages []shacl.Term) string {
	parts := make([]string, 0, len(messages))
	for _, m := range messages {
		if v := termValue(m); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "; ")
}

// termValue safely reads a goRDFlib Term's string value — result terms
// (FocusNode, SourceShape, ResultPath, ...) are nil-valued zero Terms when
// the constraint evaluator had nothing to report for that field.
func termValue(t shacl.Term) string {
	return strings.TrimSpace(t.Value())
}

// shaclLocalName extracts the fragment/last-segment local name from a full
// IRI (e.g. "https://w3id.org/facis/dcs/ontology/v1#ContractShape" ->
// "ContractShape", "http://www.w3.org/ns/shacl#MinCountConstraintComponent"
// -> "MinCountConstraintComponent") for compact, readable rule IDs/titles.
func shaclLocalName(iri string) string {
	if iri == "" {
		return ""
	}
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}

// hermeticContextLoader returns a JSON-LD document loader that resolves
// ONLY the Semantic Hub's own active JSON-LD context, entirely in-process —
// never a network fetch during validation (commit 1fa4a097 established
// hermetic runtime deps; SHACL validation must not regress that). Any other
// context IRI a document references hard-fails with a clear error rather
// than silently degrading to a remote lookup.
func hermeticContextLoader(activeContextJSON string) (ld.DocumentLoader, error) {
	doc, err := ld.DocumentFromReader(strings.NewReader(activeContextJSON))
	if err != nil {
		return nil, fmt.Errorf("parse active JSON-LD context: %w", err)
	}
	return staticContextLoader{document: doc}, nil
}

type staticContextLoader struct {
	document any
}

func (l staticContextLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	if strings.Contains(u, schemaRefJSONLDContext) || (schemaRefJSONLDContext != "" && u == schemaRefJSONLDContext) {
		return &ld.RemoteDocument{DocumentURL: u, Document: l.document}, nil
	}
	return nil, fmt.Errorf("SHACL validation: JSON-LD context %q is not the offline hub cache; network fetch during validation is disallowed", u)
}
