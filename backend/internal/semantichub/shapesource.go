package semantichub

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// HubShapeSource is the Semantic Hub-backed enforcement source
// (DCS-FR-TR-03, ADR-8): it structurally satisfies
// validation.ShapeSource without semantichub importing the validation
// package (main.go wires the two together), so registering/activating/
// rolling back a hub schema version changes what AuditContractContent
// actually enforces.
type HubShapeSource struct {
	DB *sqlx.DB
}

func (h HubShapeSource) ActiveShapes(ctx context.Context) (string, int, error) {
	return h.active(ctx, ShapesName, "shapes")
}

func (h HubShapeSource) ActiveProfile(ctx context.Context) (string, int, error) {
	return h.active(ctx, ProfileName, "profile")
}

func (h HubShapeSource) ActiveContext(ctx context.Context) (string, int, error) {
	return h.active(ctx, ContextName, "context")
}

// ShapesAt returns the SHACL shapes pinned at a specific version — used to
// revalidate a document against the hub version that was active when it was
// produced. Hub versions are immutable, so this stays resolvable forever.
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
	return s.Content, nil
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
// Used at startup to anchor produced documents' schemaRefs to each schema
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
