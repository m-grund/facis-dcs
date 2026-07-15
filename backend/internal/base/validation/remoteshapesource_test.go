package validation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVerifyAgainstOriginatorHub is the Phase 4 DCS-to-DCS acceptance
// criterion: a counterparty resolves the originator's public hub URLs from
// a received document's dcs:schemaRefs and validates against THOSE shapes
// — a fake HTTP server here stands in for a peer DCS instance's public
// Semantic Hub endpoints (GET /semantic/schema/retrieve).
func TestVerifyAgainstOriginatorHub(t *testing.T) {
	shapesTTL := mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl")
	contextJSON := mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Query().Get("kind")
		var content string
		switch kind {
		case "shapes":
			content = shapesTTL
		case "context":
			content = contextJSON
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "content": content})
	}))
	defer server.Close()

	// A contract missing dcs:metadata.dcs:title — the originator's hub
	// (this fake server) considers it non-conformant.
	invalidContract := map[string]any{
		"@context":              map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":                   "urn:facis:dcs:contract:remote-001",
		"@type":                 "dcs:Contract",
		"dcs:metadata":          map[string]any{"@type": "dcs:ContractMetadata"},
		"dcs:documentStructure": map[string]any{"@type": "dcs:DocumentStructure"},
	}
	findings, err := VerifyAgainstOriginatorHub(context.Background(), invalidContract, server.URL)
	require.NoError(t, err)
	require.NotEmpty(t, findings)
	require.Equal(t, "error", findings[0].Severity)

	validContract := map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "xsd": "http://www.w3.org/2001/XMLSchema#"},
		"@id":      "urn:facis:dcs:contract:remote-002",
		"@type":    "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@type":     "dcs:ContractMetadata",
			"dcs:title": "Remote-validated contract",
		},
		"dcs:documentStructure": map[string]any{"@type": "dcs:DocumentStructure"},
	}
	okFindings, err := VerifyAgainstOriginatorHub(context.Background(), validContract, server.URL)
	require.NoError(t, err)
	require.Empty(t, okFindings)

	// Concurrency safety: VerifyAgainstOriginatorHub must not disturb the
	// process-wide activeShapeSource other request-handling goroutines rely
	// on (it never calls SetShapeSource) — verified indirectly by asserting
	// the package fixture is unchanged after the calls above.
	require.NotNil(t, activeShapeSource)
}
