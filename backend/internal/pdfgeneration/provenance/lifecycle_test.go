package provenance

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
		"draft",
		"initial creation",
		"did:example:authority",
		"urn:dcs:vc:vcid",
		effectiveAt,
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))

	// DCS-OR-C2PA-003: all fields required.
	for _, field := range []string{
		"label", "contract_id", "file_hash",
		"status", "reason", "effective_at", "authority", "vc_id",
	} {
		assert.Contains(t, m, field, "field %q missing from lifecycle assertion JSON", field)
	}

	assert.Equal(t, lifecycleAssertionLabel, a.Label)
}

// TestMapCWEStateToC2PA_CWEUppercaseMappings verifies that every uppercase CWE state
// (as emitted by the CWE state machine) maps to the correct SRS C2PA state.
// This is the fix for Gap 4 (DCS-OR-C2PA-003 lifecycle vocabulary coverage).
func TestMapCWEStateToC2PA_CWEUppercaseMappings(t *testing.T) {
	cases := []struct {
		cwe  string
		want string
	}{
		{"DRAFT", "draft"},
		{"SUBMITTED", "active"},
		{"REVIEWED", "active"},
		{"APPROVED", "active"},
		{"NEGOTIATION", "amended"},
		{"REJECTED", "amended"},
		{"TERMINATED", "terminated"},
		{"EXPIRED", "expired"},
		{"SUSPENDED", "suspended"},
		{"REPLACED", "replaced"},
	}
	for _, tc := range cases {
		t.Run(tc.cwe, func(t *testing.T) {
			got, err := MapCWEStateToC2PA(tc.cwe)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "CWE state %q must map to SRS state %q", tc.cwe, tc.want)
		})
	}
}

func TestMapCWEStateToC2PA_UnknownStateFails(t *testing.T) {
	_, err := MapCWEStateToC2PA("UNKNOWN_FUTURE_STATE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported lifecycle state")
}

// TestMapCWEStateToC2PA_AllSRSStatesCovered verifies that the SRS-mandated states
// (DCS-OR-C2PA-003) are reachable from at least one CWE input.
func TestMapCWEStateToC2PA_AllSRSStatesCovered(t *testing.T) {
	required := map[string]bool{
		"draft": false, "active": false, "amended": false,
		"suspended": false, "terminated": false, "expired": false, "replaced": false,
	}
	// Map a representative CWE input for each SRS state.
	inputs := []string{"DRAFT", "APPROVED", "NEGOTIATION", "SUSPENDED", "TERMINATED", "EXPIRED", "REPLACED"}
	for _, in := range inputs {
		got, err := MapCWEStateToC2PA(in)
		require.NoError(t, err)
		required[got] = true
	}
	for state, covered := range required {
		assert.True(t, covered, "SRS state %q must be reachable from at least one CWE input", state)
	}
}

func TestLifecycleAssertion_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	a := NewLifecycleAssertion(
		"did:example:c1", "hash1",
		"active", "", "did:example:auth", "",
		time.Now().UTC(),
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	// reason and vc_id are omitempty — absent when empty.
	assert.NotContains(t, string(raw), `"reason"`)
	assert.NotContains(t, string(raw), `"vc_id"`)
}
