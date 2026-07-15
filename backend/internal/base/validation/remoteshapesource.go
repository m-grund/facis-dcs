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
// public Semantic Hub endpoints (Phase 4, DCS-to-DCS: DCS-FR-TR-03).
// GET /semantic/schema/retrieve is public and unauthenticated (same as
// resolve_context) precisely so that a counterparty instance receiving a
// contract can resolve dcs:schemaRefs back to the ORIGINATOR's hub and
// re-run validation against the exact shapes/profile/context it was
// produced under — not the receiver's own local hub, which may be running
// a different active version. This is what a produced document's
// dcs:schemaRefs anchor (semantichub.AnchorURL) exists to make possible.
type RemoteShapeSource struct {
	// BaseURL is the originator instance's public origin (e.g.
	// "https://dcs-a.example.org" or, in the BDD two-instance deployment,
	// "http://dcs-a.localhost:18080") — schemaRefs anchors are host-relative
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

// VerifyAgainstOriginatorHub (Phase 4, DCS-to-DCS) validates a received
// document against the ORIGINATOR's Semantic Hub rather than the local
// instance's — resolving dcs:schemaRefs back to originatorBaseURL. An
// external verifier (or a counterparty DCS instance receiving a synced
// contract) uses this to confirm the document validates against the exact
// shapes it was actually produced under, independent of what the local
// instance's own hub currently has active.
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
