package validation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ShapeSource is the enforcement-time source for the SHACL shapes and
// validation profile AuditContractContent checks produced documents
// against (DCS-FR-TR-03, ADR-8). Before this existed, enforcement always
// read docs/semantic-ontology/... straight off disk — the Semantic Hub
// (internal/semantichub) stored versioned schemas but nothing ever
// consulted it, so registering/activating/rolling back a schema version
// changed nothing about what got enforced. HubShapeSource
// (internal/semantichub) is the only implementation: this is a greenfield
// system with no deployed instances to keep a disk-file fallback for — the
// disk copies under docs/semantic-ontology/ exist solely as the Semantic
// Hub's startup seed (backend/internal/semantichub/assets, go:embed).
type ShapeSource interface {
	// ActiveShapes returns the SHACL shapes document (hub kind="shapes")
	// currently active, and its version.
	ActiveShapes(ctx context.Context) (content string, version int, err error)
	// ActiveProfile returns the validation profile document (hub
	// kind="profile") currently active, and its version.
	ActiveProfile(ctx context.Context) (content string, version int, err error)
	// ActiveContext returns the JSON-LD context (hub kind="context")
	// currently active, and its version.
	ActiveContext(ctx context.Context) (content string, version int, err error)
	// ShapesAt returns the SHACL shapes document pinned at a specific
	// version — used to revalidate a document against the hub version that
	// was active when it was produced (its sh:shapesGraph anchor), not
	// whatever is active now (ADR-8).
	ShapesAt(ctx context.Context, version int) (content string, err error)
	// ContextAt returns the JSON-LD context pinned at a specific version —
	// a document's "@context" carries the versioned hub context URL it was
	// authored under, and expansion during validation resolves exactly that
	// version (ADR-8), hub versions being immutable.
	ContextAt(ctx context.Context, version int) (content string, err error)
}

// activeShapeSource is the process-wide enforcement source, installed once
// at startup (cmd/dcs/main.go) after the database and Semantic Hub are
// available. Package-level rather than threaded as a parameter:
// AuditContractContent predates context plumbing here and every real call
// happens after startup seeding has run; SetShapeSource mirrors the
// existing SetSchemaAnchorRefs/SetCanonicalOntologyIRIs package-var pattern.
// Left nil until SetShapeSource runs — using it before startup wiring is a
// programming error and hard-fails (requireShapeSource) rather than
// silently falling back to anything.
var activeShapeSource ShapeSource

// SetShapeSource installs the process-wide enforcement source.
func SetShapeSource(s ShapeSource) {
	if s != nil {
		activeShapeSource = s
	}
}

// requireShapeSource hard-fails when the Semantic Hub enforcement source
// has not been configured, instead of silently validating against nothing.
func requireShapeSource() (ShapeSource, error) {
	if activeShapeSource == nil {
		return nil, errors.New("semantic hub shape source is not configured (SetShapeSource was never called)")
	}
	return activeShapeSource, nil
}

// pinnedVersionPattern extracts the ?version=N (or &version=N) query
// parameter semantichub.AnchorURL encodes into a hub-served schema URL.
var pinnedVersionPattern = regexp.MustCompile(`[?&]version=(\d+)`)

// pinnedHubShapesVersion reads the hub SHACL shapes version a produced
// document was anchored to at creation time via its sh:shapesGraph link
// (ADR-8: anchors are set once, at production time, and never
// re-normalized — so this is stable for the document's lifetime even after
// the hub's active version moves on). Returns 0 (no pin) for documents
// with no sh:shapesGraph or a non-hub-versioned one.
func pinnedHubShapesVersion(contract map[string]any) int {
	return anchorVersion(anchorIRI(contract["sh:shapesGraph"]))
}

// pinnedHubContextVersion reads the hub JSON-LD context version a produced
// document's "@context" is anchored to — the hub context URL is either the
// whole @context or the string entry of its array form
// (normalizeCanonicalContext).
func pinnedHubContextVersion(contract map[string]any) int {
	switch context := contract["@context"].(type) {
	case string:
		return anchorVersion(context)
	case []any:
		for _, entry := range context {
			if url, ok := entry.(string); ok {
				if v := anchorVersion(url); v > 0 {
					return v
				}
			}
		}
	}
	return 0
}

// anchorIRI reads the IRI out of a JSON-LD object reference ({"@id": ...})
// or a plain string.
func anchorIRI(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		iri, _ := typed["@id"].(string)
		return iri
	}
	return ""
}

func anchorVersion(iri string) int {
	match := pinnedVersionPattern.FindStringSubmatch(iri)
	if match == nil {
		return 0
	}
	version := 0
	if _, err := fmt.Sscanf(match[1], "%d", &version); err != nil {
		return 0
	}
	return version
}
