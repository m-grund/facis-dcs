// Package semantichub is the Semantic Hub (DCS-FR-TR-03, UC-02-08): a
// versioned repository for the machine-readable schemas the DCS produces
// documents against — JSON-LD contexts, SHACL shapes, and validation
// profiles. It is seeded at startup with the FACIS DCS v1 profile (the
// assets/ copies of docs/semantic-ontology, the authoring source), serves
// every version over /semantic/..., and exposes the ACTIVE context's
// ontology IRIs so the normalization layer can anchor and enforce them on
// every produced JSON-LD artifact.
package semantichub

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed assets/facis-dcs-context.jsonld
var genesisContext []byte

//go:embed assets/facis-dcs-shapes.ttl
var genesisShapes []byte

//go:embed assets/facis.sla.basic.v1.yaml
var genesisProfile []byte

//go:embed assets/facis-dcs-clause-catalog.ttl
var genesisClauseCatalog []byte

//go:embed assets/facis-dcs-ontology.ttl
var genesisOntology []byte

//go:embed assets/facis-sla-ontology.ttl
var genesisSLAOntology []byte

//go:embed assets/dcs-odrl-profile.ttl
var genesisODRLProfile []byte

// Canonical hub schema names. ContextName is the JSON-LD context every DCS
// document resolves its prefixes against. ClauseCatalogName is a second,
// independently-versioned kind="shapes" entry (Phase 3, ADR-10): typed
// clause NodeShapes the template builder's palette (GET /semantic/clauses)
// and contract validation (validateAgainstHubShapes) both read.
const (
	ContextName       = "facis-dcs"
	ShapesName        = "facis-dcs"
	ProfileName       = "facis.sla.basic"
	OntologyName      = "facis-dcs"
	SLAOntologyName   = "facis-sla"
	ODRLProfileName   = "dcs-odrl-profile"
	ClauseCatalogName = "clause-catalog"
)

// Schema is one stored, versioned hub entry.
type Schema struct {
	Name      string `db:"name"`
	Version   int    `db:"version"`
	Kind      string `db:"kind"`
	MediaType string `db:"media_type"`
	Content   string `db:"content"`
	Active    bool   `db:"active"`
	CreatedBy string `db:"created_by"`
	CreatedAt string `db:"created_at"`
}

// ErrSchemaNotFound is returned when no matching schema (name/version) exists.
var ErrSchemaNotFound = errors.New("semantic hub: schema not found")

// Repo is the hub's Postgres access layer.
type Repo struct{}

// Register stores content as the next version of name and, when activate is
// set, makes it the active version. Returns the assigned version.
func (Repo) Register(ctx context.Context, tx *sqlx.Tx, name, kind, mediaType, content, createdBy string, activate bool) (int, error) {
	var version int
	// Explicit casts: $1/$2 appear both as inserted VALUES and inside the
	// version subselect, and Postgres refuses to deduce one type for a
	// parameter used in two positions (42P08) without them.
	err := tx.QueryRowContext(ctx, `
        INSERT INTO semantic_schemas (name, version, kind, media_type, content, active, created_by)
        VALUES ($1::varchar, COALESCE((SELECT MAX(version) FROM semantic_schemas WHERE name = $1::varchar AND kind = $2::varchar), 0) + 1, $2::varchar, $3, $4, FALSE, $5)
        RETURNING version
    `, name, kind, mediaType, content, createdBy).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("semantic hub: register %s: %w", name, err)
	}
	if activate {
		if err := activateVersion(ctx, tx, name, kind, version); err != nil {
			return 0, err
		}
	}
	return version, nil
}

// Activate makes an existing version the active one (UC-02-08 rollback).
func (Repo) Activate(ctx context.Context, tx *sqlx.Tx, name, kind string, version int) error {
	var exists bool
	if err := tx.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM semantic_schemas WHERE name = $1 AND kind = $2 AND version = $3)`, name, kind, version); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s/%s version %d", ErrSchemaNotFound, name, kind, version)
	}
	return activateVersion(ctx, tx, name, kind, version)
}

func activateVersion(ctx context.Context, tx *sqlx.Tx, name, kind string, version int) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE semantic_schemas SET active = FALSE WHERE name = $1 AND kind = $2 AND active`, name, kind); err != nil {
		return fmt.Errorf("semantic hub: deactivate %s/%s: %w", name, kind, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE semantic_schemas SET active = TRUE WHERE name = $1 AND kind = $2 AND version = $3`, name, kind, version); err != nil {
		return fmt.Errorf("semantic hub: activate %s/%s v%d: %w", name, kind, version, err)
	}
	return nil
}

// Get returns a specific version, or the ACTIVE version when version is 0.
func (Repo) Get(ctx context.Context, tx *sqlx.Tx, name, kind string, version int) (*Schema, error) {
	var s Schema
	var err error
	if version > 0 {
		err = tx.GetContext(ctx, &s, `
            SELECT name, version, kind, media_type, content, active, created_by, created_at::text
            FROM semantic_schemas WHERE name = $1 AND kind = $2 AND version = $3`, name, kind, version)
	} else {
		err = tx.GetContext(ctx, &s, `
            SELECT name, version, kind, media_type, content, active, created_by, created_at::text
            FROM semantic_schemas WHERE name = $1 AND kind = $2 AND active`, name, kind)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s/%s", ErrSchemaNotFound, name, kind)
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListEntry summarizes one (name, kind) hub entry for the management UI.
type ListEntry struct {
	Name          string `db:"name"`
	Kind          string `db:"kind"`
	MediaType     string `db:"media_type"`
	ActiveVersion int    `db:"active_version"`
	LatestVersion int    `db:"latest_version"`
	UpdatedAt     string `db:"updated_at"`
}

// List returns every distinct (name, kind) entry with its active/latest
// version summary, ordered by kind then name.
func (Repo) List(ctx context.Context, tx *sqlx.Tx) ([]ListEntry, error) {
	var out []ListEntry
	err := tx.SelectContext(ctx, &out, `
        SELECT s.name, s.kind,
               MAX(s.version) AS latest_version,
               COALESCE(MAX(s.version) FILTER (WHERE s.active), 0) AS active_version,
               MAX(s.created_at)::text AS updated_at,
               (ARRAY_AGG(s.media_type ORDER BY s.version DESC))[1] AS media_type
        FROM semantic_schemas s
        GROUP BY s.name, s.kind
        ORDER BY s.kind, s.name`)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Versions lists all stored versions of (name, kind), oldest first.
func (Repo) Versions(ctx context.Context, tx *sqlx.Tx, name, kind string) ([]Schema, error) {
	var out []Schema
	err := tx.SelectContext(ctx, &out, `
        SELECT name, version, kind, media_type, content, active, created_by, created_at::text
        FROM semantic_schemas WHERE name = $1 AND kind = $2 ORDER BY version`, name, kind)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Seed idempotently installs the genesis FACIS DCS v1 profile: the embedded
// context, shapes, and validation profile become version 1 (active) unless
// the hub already holds them. Fatal on failure — the hub is a required
// dependency of document normalization.
func Seed(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	genesis := []struct {
		name, kind, mediaType string
		content               []byte
	}{
		{ContextName, "context", "application/ld+json", genesisContext},
		{ShapesName, "shapes", "text/turtle", genesisShapes},
		{ProfileName, "profile", "application/yaml", genesisProfile},
		{ClauseCatalogName, "shapes", "text/turtle", genesisClauseCatalog},
		{OntologyName, "ontology", "text/turtle", genesisOntology},
		{SLAOntologyName, "ontology", "text/turtle", genesisSLAOntology},
		{ODRLProfileName, "ontology", "text/turtle", genesisODRLProfile},
	}
	for _, g := range genesis {
		var exists bool
		if err := tx.GetContext(ctx, &exists,
			`SELECT EXISTS(SELECT 1 FROM semantic_schemas WHERE name = $1 AND kind = $2)`, g.name, g.kind); err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := (Repo{}).Register(ctx, tx, g.name, g.kind, g.mediaType, string(g.content), "system:genesis", true); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ActiveOntologyIRIs returns the prefix -> IRI map declared by the ACTIVE
// context's @context object (only string-valued prefix entries). The
// normalization layer enforces these on every produced document.
func ActiveOntologyIRIs(ctx context.Context, db *sqlx.DB) (map[string]string, int, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, ContextName, "context", 0)
	if err != nil {
		return nil, 0, err
	}
	var doc struct {
		Context map[string]any `json:"@context"`
	}
	if err := json.Unmarshal([]byte(s.Content), &doc); err != nil {
		return nil, 0, fmt.Errorf("semantic hub: parse active context: %w", err)
	}
	iris := map[string]string{}
	for prefix, v := range doc.Context {
		if iri, ok := v.(string); ok && !strings.HasPrefix(prefix, "@") && strings.Contains(iri, "://") {
			iris[prefix] = iri
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return iris, s.Version, nil
}

// AnchorURL builds the hub-served, versioned URL a produced document's
// schema anchors to. Mirrors provenance.RemoteManifestURL's DCS_PUBLIC_URL
// convention: without a configured public URL the reference stays
// host-relative (still resolvable against the serving instance).
func AnchorURL(kind, name string, version int) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("DCS_PUBLIC_URL")), "/")
	return fmt.Sprintf("%s/semantic/%s/%s?version=%d", base, kind, name, version)
}
