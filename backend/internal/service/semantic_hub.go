package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	semantichubgen "digital-contracting-service/gen/semantic_hub"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub"

	"github.com/jmoiron/sqlx"
)

// SemanticHub service implementation (DCS-FR-TR-03, UC-02-08).
type semanticHubsrvc struct {
	DB   *sqlx.DB
	Repo semantichub.Repo
	auth.JWTAuthenticator
}

// NewSemanticHub returns the SemanticHub service implementation.
func NewSemanticHub(db *sqlx.DB, jwtAuth auth.JWTAuthenticator) semantichubgen.Service {
	return &semanticHubsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
	}
}

func (s *semanticHubsrvc) Register(ctx context.Context, p *semantichubgen.RegisterPayload) (res *semantichubgen.SemanticSchemaRegisterResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if p.Kind == "context" {
		// A context version must at least parse as a JSON-LD document with
		// an @context object — a broken active context would break every
		// subsequent document normalization.
		var doc struct {
			Context map[string]any `json:"@context"`
		}
		if err := json.Unmarshal([]byte(p.Content), &doc); err != nil || len(doc.Context) == 0 {
			return nil, semantichubgen.MakeBadRequest(
				fmt.Errorf("context schema content must be a JSON-LD document with a non-empty @context object"))
		}
	}
	if strings.TrimSpace(p.Content) == "" {
		return nil, semantichubgen.MakeBadRequest(errors.New("schema content must not be empty"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	activate := p.Activate != nil && *p.Activate
	version, err := s.Repo.Register(ctx, tx, p.Name, p.Kind, p.MediaType, p.Content, middleware.GetParticipantID(ctx), activate)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}

	return &semantichubgen.SemanticSchemaRegisterResponse{
		Name:    p.Name,
		Version: version,
		Kind:    p.Kind,
		Active:  activate,
	}, nil
}

func (s *semanticHubsrvc) Rollback(ctx context.Context, p *semantichubgen.RollbackPayload) (res *semantichubgen.SemanticSchemaRegisterResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.Repo.Activate(ctx, tx, p.Name, p.Kind, p.Version); err != nil {
		if errors.Is(err, semantichub.ErrSchemaNotFound) {
			return nil, semantichubgen.MakeNotFound(err)
		}
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}

	return &semantichubgen.SemanticSchemaRegisterResponse{
		Name:    p.Name,
		Version: p.Version,
		Kind:    p.Kind,
		Active:  true,
	}, nil
}

func (s *semanticHubsrvc) Retrieve(ctx context.Context, p *semantichubgen.RetrievePayload) (res *semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, p.Kind, p.Version)
	if err != nil {
		return nil, err
	}
	return toSchemaItem(schema), nil
}

func (s *semanticHubsrvc) Versions(ctx context.Context, p *semantichubgen.VersionsPayload) (res []*semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	schemas, err := s.Repo.Versions(ctx, tx, p.Name, p.Kind)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	out := make([]*semantichubgen.SemanticSchemaItem, 0, len(schemas))
	for i := range schemas {
		out = append(out, toSchemaItem(&schemas[i]))
	}
	return out, nil
}

func (s *semanticHubsrvc) ResolveContext(ctx context.Context, p *semantichubgen.ResolveContextPayload) (res any, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, "context", p.Version)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal([]byte(schema.Content), &doc); err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("stored context %s v%d is not valid JSON: %w", schema.Name, schema.Version, err))
	}
	return doc, nil
}

// Clauses serves the pre-digested clause catalog form-schema (Phase 3,
// ADR-10): the same shapes graph validateAgainstHubShapes concatenates into
// contract validation, so the template builder's palette and server-side
// enforcement never drift apart.
func (s *semanticHubsrvc) Clauses(ctx context.Context) (res *semantichubgen.ClauseCatalogResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, semantichub.ClauseCatalogName, "shapes", nil)
	if err != nil {
		return nil, err
	}
	entries, err := semantichub.ParseClauseCatalog(schema.Content)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("parse clause catalog v%d: %w", schema.Version, err))
	}

	clauses := make([]*semantichubgen.ClauseCatalogType, 0, len(entries))
	for _, entry := range entries {
		properties := make([]*semantichubgen.ClauseCatalogProperty, 0, len(entry.Properties))
		for _, p := range entry.Properties {
			prop := &semantichubgen.ClauseCatalogProperty{
				Path:         p.Path,
				In:           p.In,
				MinInclusive: p.MinInclusive,
				MaxInclusive: p.MaxInclusive,
			}
			if p.Datatype != "" {
				datatype := p.Datatype
				prop.Datatype = &datatype
			}
			prop.MinCount = p.MinCount
			prop.MaxCount = p.MaxCount
			properties = append(properties, prop)
		}
		clauses = append(clauses, &semantichubgen.ClauseCatalogType{
			Type:       entry.Type,
			Label:      entry.Label,
			Properties: properties,
		})
	}

	return &semantichubgen.ClauseCatalogResponse{
		Version: schema.Version,
		Clauses: clauses,
		Shapes:  schema.Content,
	}, nil
}

func (s *semanticHubsrvc) getSchema(ctx context.Context, name, kind string, version *int) (*semantichub.Schema, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	v := 0
	if version != nil {
		v = *version
	}
	schema, err := s.Repo.Get(ctx, tx, name, kind, v)
	if err != nil {
		if errors.Is(err, semantichub.ErrSchemaNotFound) {
			return nil, semantichubgen.MakeNotFound(err)
		}
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	return schema, nil
}

func toSchemaItem(s *semantichub.Schema) *semantichubgen.SemanticSchemaItem {
	return &semantichubgen.SemanticSchemaItem{
		Name:      s.Name,
		Version:   s.Version,
		Kind:      s.Kind,
		MediaType: s.MediaType,
		Content:   s.Content,
		Active:    s.Active,
		CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt,
	}
}
