package validation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ShapeSource is the enforcement-time source for the SHACL shapes,
// validation profile, and JSON-LD context AuditContractContent checks
// produced documents against. HubShapeSource (internal/semantichub) is the
// production implementation.
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
	// ShapesAt returns the SHACL shapes document at a specific version —
	// the version a document's sh:shapesGraph anchor pins.
	ShapesAt(ctx context.Context, version int) (content string, err error)
	// ContextAt returns the JSON-LD context at a specific version — the
	// version a document's "@context" hub URL pins.
	ContextAt(ctx context.Context, version int) (content string, err error)
}

// activeShapeSource is the process-wide enforcement source, installed at
// startup (cmd/dcs/main.go); nil until SetShapeSource runs.
var activeShapeSource ShapeSource

// SetShapeSource installs the process-wide enforcement source.
func SetShapeSource(s ShapeSource) {
	if s != nil {
		activeShapeSource = s
	}
}

func requireShapeSource() (ShapeSource, error) {
	if activeShapeSource == nil {
		return nil, errors.New("semantic hub shape source is not configured (SetShapeSource was never called)")
	}
	return activeShapeSource, nil
}

// pinnedVersionPattern extracts the ?version=N (or &version=N) query
// parameter semantichub.AnchorURL encodes into a hub-served schema URL.
var pinnedVersionPattern = regexp.MustCompile(`[?&]version=(\d+)`)

// pinnedHubShapesVersion reads the hub SHACL shapes version pinned by the
// document's sh:shapesGraph anchor; 0 when there is no versioned anchor.
func pinnedHubShapesVersion(contract map[string]any) int {
	return anchorVersion(anchorIRI(contract["sh:shapesGraph"]))
}

// pinnedHubContextVersion reads the hub context version pinned by the
// document's "@context" — the hub URL is either the whole @context or a
// string entry of its array form.
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
