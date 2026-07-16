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

// validateAgainstHubShapes checks a decoded JSON-LD document against the
// Semantic Hub's SHACL shapes: the version pinned by the document's
// sh:shapesGraph anchor when present, otherwise the currently-active one.
// Returns the findings and the shapes version they were produced against.
func validateAgainstHubShapes(ctx context.Context, contract map[string]any) ([]PolicyFinding, int, error) {
	source, err := requireShapeSource()
	if err != nil {
		return nil, 0, err
	}
	return validateAgainstShapeSource(ctx, contract, source)
}

// validateAgainstShapeSource is validateAgainstHubShapes generalized over an
// explicit ShapeSource — used directly (bypassing the process-wide
// activeShapeSource) by VerifyAgainstOriginatorHub (Phase 4, DCS-to-DCS),
// so a one-off remote-hub validation never mutates shared process state
// under concurrent request handling.
func validateAgainstShapeSource(ctx context.Context, contract map[string]any, source ShapeSource) ([]PolicyFinding, int, error) {
	var err error
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

	var contextContent string
	if pinnedContext := pinnedHubContextVersion(contract); pinnedContext > 0 {
		contextContent, err = source.ContextAt(ctx, pinnedContext)
		if err != nil {
			return nil, 0, fmt.Errorf("load pinned JSON-LD context v%d: %w", pinnedContext, err)
		}
	} else {
		contextContent, _, err = source.ActiveContext(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("load active JSON-LD context: %w", err)
		}
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
// PolicyFinding shape every other audit source in this package produces.
// SHACL reports only non-conformant results — a conformant document yields
// no findings.
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

// hermeticContextLoader returns a JSON-LD document loader that serves the
// given hub context content in-process; any non-hub context IRI fails
// instead of triggering a network fetch during validation.
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
	if strings.Contains(u, "/semantic/context/") || strings.Contains(u, schemaRefJSONLDContext) || u == SchemaJSONLDContextV1 {
		return &ld.RemoteDocument{DocumentURL: u, Document: l.document}, nil
	}
	return nil, fmt.Errorf("SHACL validation: JSON-LD context %q is not the offline hub cache; network fetch during validation is disallowed", u)
}
