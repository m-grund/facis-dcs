package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// RemoteShapeSource is a ShapeSource backed by another DCS instance's
// public Semantic Hub endpoints, so a received document can be validated
// against the originator's hub rather than the local one.
type RemoteShapeSource struct {
	// BaseURL is the originator instance's public origin (e.g.
	// "https://dcs-a.example.org" or, in the BDD two-instance deployment,
	// "http://dcs-a.localhost:18080") — hub anchors are host-relative
	// when the originator has no DCS_PUBLIC_URL configured
	// (semantichub.AnchorURL), so the caller supplies the origin the
	// contract was actually received from.
	BaseURL string
	// ShapesName/ProfileName/ContextName name the remote hub's schema
	// entries (mirrors semantichub.ShapesName etc. — the DCS ontology is
	// shared, so these are the same well-known names on every instance).
	ShapesName, ProfileName, ContextName string
	HTTPClient                           *http.Client
}

const remoteShapeSourceTimeout = 15 * time.Second

func (r RemoteShapeSource) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: remoteShapeSourceTimeout}
}

func (r RemoteShapeSource) ActiveShapes(ctx context.Context) (string, int, error) {
	return r.retrieve(ctx, r.ShapesName, "shapes", 0)
}

func (r RemoteShapeSource) ActiveProfile(ctx context.Context) (string, int, error) {
	return r.retrieve(ctx, r.ProfileName, "profile", 0)
}

func (r RemoteShapeSource) ActiveContext(ctx context.Context) (string, int, error) {
	return r.retrieve(ctx, r.ContextName, "context", 0)
}

func (r RemoteShapeSource) ShapesAt(ctx context.Context, version int) (string, error) {
	content, _, err := r.retrieve(ctx, r.ShapesName, "shapes", version)
	return content, err
}

func (r RemoteShapeSource) ContextAt(ctx context.Context, version int) (string, error) {
	content, _, err := r.retrieve(ctx, r.ContextName, "context", version)
	return content, err
}

func (r RemoteShapeSource) ContextByIRI(ctx context.Context, iri string) (string, error) {
	content, _, err := r.retrieve(ctx, iri, "context", 0)
	return content, err
}

type remoteSchemaItem struct {
	Version int    `json:"version"`
	Content string `json:"content"`
}

func (r RemoteShapeSource) retrieve(ctx context.Context, name, kind string, version int) (string, int, error) {
	u, err := url.Parse(r.BaseURL + "/semantic/schema/retrieve")
	if err != nil {
		return "", 0, fmt.Errorf("remote shape source: invalid base URL %q: %w", r.BaseURL, err)
	}
	q := u.Query()
	q.Set("name", name)
	q.Set("kind", kind)
	if version > 0 {
		q.Set("version", strconv.Itoa(version))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("remote shape source: fetch %s/%s from %s: %w", kind, name, r.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("remote shape source: %s/%s from %s: HTTP %d", kind, name, r.BaseURL, resp.StatusCode)
	}
	var item remoteSchemaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return "", 0, fmt.Errorf("remote shape source: decode %s/%s response from %s: %w", kind, name, r.BaseURL, err)
	}
	return item.Content, item.Version, nil
}

// VerifyAgainstOriginatorHub validates a received document against the
// originator's Semantic Hub at originatorBaseURL rather than the local
// instance's.
func VerifyAgainstOriginatorHub(ctx context.Context, contractDocument any, originatorBaseURL string) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	remote := RemoteShapeSource{
		BaseURL:     originatorBaseURL,
		ShapesName:  "facis-dcs",
		ProfileName: "facis.sla.basic",
		ContextName: "facis-dcs",
	}
	findings, _, err := validateAgainstShapeSource(ctx, contract, remote)
	return findings, err
}
