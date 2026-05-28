package c2pa

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleAssertion_AllFieldsPresent(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	a := NewLifecycleAssertion(
		"did:example:contract123",
		"abc123hash",
		"pdfhash",
		"1.0.0",
		"draft",
		"initial creation",
		"did:example:authority",
		"urn:dcs:vc:vcid",
		"prevhash",
		effectiveAt,
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))

	// DCS-OR-C2PA-003: all fields required.
	for _, field := range []string{
		"label", "contract_id", "file_hash", "pdf_hash", "renderer_version",
		"status", "reason", "effective_at", "authority", "vc_id", "prev_manifest_hash",
	} {
		assert.Contains(t, m, field, "field %q missing from lifecycle assertion JSON", field)
	}

	assert.Equal(t, lifecycleAssertionLabel, a.Label)
}

func TestLifecycleAssertion_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	a := NewLifecycleAssertion(
		"did:example:c1", "hash1", "pdfhash1", "1.0.0",
		"active", "", "did:example:auth", "", "",
		time.Now(),
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	// reason, vc_id, prev_manifest_hash are omitempty — absent when empty.
	assert.NotContains(t, string(raw), `"reason"`)
	assert.NotContains(t, string(raw), `"vc_id"`)
	assert.NotContains(t, string(raw), `"prev_manifest_hash"`)
}
