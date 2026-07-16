package semantichub

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// HubShapeSource is the Semantic Hub-backed enforcement source; it
// structurally satisfies validation.ShapeSource (main.go wires the two
// together).
type HubShapeSource struct {
	DB *sqlx.DB
}

// ActiveShapes returns the canonical contract shapes concatenated with the
// clause catalog's active version into one shapes graph.
func (h HubShapeSource) ActiveShapes(ctx context.Context) (string, int, error) {
	content, version, err := h.active(ctx, ShapesName, "shapes")
	if err != nil {
		return "", 0, err
	}
	merged, err := h.withClauseCatalog(ctx, content)
	if err != nil {
		return "", 0, err
	}
	return merged, version, nil
}

func (h HubShapeSource) ActiveProfile(ctx context.Context) (string, int, error) {
	return h.active(ctx, ProfileName, "profile")
}

func (h HubShapeSource) ActiveContext(ctx context.Context) (string, int, error) {
	return h.active(ctx, ContextName, "context")
}

// ShapesAt returns the SHACL shapes at a specific version, concatenated
// with the clause catalog's active version.
func (h HubShapeSource) ShapesAt(ctx context.Context, version int) (string, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, ShapesName, "shapes", version)
	if err != nil {
		return "", fmt.Errorf("semantic hub: pinned shapes v%d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return h.withClauseCatalog(ctx, s.Content)
}

// ContextAt returns the JSON-LD context at a specific version.
func (h HubShapeSource) ContextAt(ctx context.Context, version int) (string, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, ContextName, "context", version)
	if err != nil {
		return "", fmt.Errorf("semantic hub: pinned context v%d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return s.Content, nil
}

// withClauseCatalog appends the clause catalog's active shapes to a
// canonical shapes document; both declare identical @prefix headers, so
// the concatenation parses as one Turtle graph.
func (h HubShapeSource) withClauseCatalog(ctx context.Context, canonicalShapesTTL string) (string, error) {
	catalog, _, err := h.active(ctx, ClauseCatalogName, "shapes")
	if err != nil {
		return "", fmt.Errorf("clause catalog: %w", err)
	}
	return canonicalShapesTTL + "\n\n" + catalog, nil
}

func (h HubShapeSource) active(ctx context.Context, name, kind string) (string, int, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, name, kind, 0)
	if err != nil {
		return "", 0, fmt.Errorf("semantic hub: active %s: %w", kind, err)
	}
	if err := tx.Commit(); err != nil {
		return "", 0, err
	}
	return s.Content, s.Version, nil
}

// ActiveVersion returns the version number of the ACTIVE (name, kind) entry.
// Used at startup to anchor produced documents' schema anchors to each schema
// kind's OWN active version — context, shapes, and profile version numbers
// diverge as soon as any one of them is registered/rolled back independently
// of the others (ADR-8).
func ActiveVersion(ctx context.Context, db *sqlx.DB, name, kind string) (int, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, name, kind, 0)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return s.Version, nil
}
