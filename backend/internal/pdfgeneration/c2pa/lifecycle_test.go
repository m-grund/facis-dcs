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
		"1.0.1",
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

// TestMapCWEStateToC2PA_SRSVocabularyPassthrough verifies that states already in the
// SRS C2PA vocabulary (DCS-OR-C2PA-003) are returned unchanged (case-preserved).
func TestMapCWEStateToC2PA_SRSVocabularyPassthrough(t *testing.T) {
	srsStates := []string{"draft", "active", "amended", "suspended", "terminated", "expired", "replaced"}
	for _, s := range srsStates {
		t.Run(s, func(t *testing.T) {
			assert.Equal(t, s, MapCWEStateToC2PA(s), "SRS vocabulary state must pass through unchanged")
		})
	}
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
			got := MapCWEStateToC2PA(tc.cwe)
			assert.Equal(t, tc.want, got, "CWE state %q must map to SRS state %q", tc.cwe, tc.want)
		})
	}
}

// TestMapCWEStateToC2PA_UnknownStateReturnsEmpty verifies that unsupported
// states are not silently coerced.
func TestMapCWEStateToC2PA_UnknownStateReturnsEmpty(t *testing.T) {
	unknowns := []string{"UNKNOWN_FUTURE_STATE", "", "pending", "ARCHIVING"}
	for _, s := range unknowns {
		t.Run(s, func(t *testing.T) {
			assert.Equal(t, "", MapCWEStateToC2PA(s),
				"unknown CWE state must not be silently mapped")
		})
	}
}

func TestMapCWEStateToC2PAStrict_UnknownStateFails(t *testing.T) {
	_, err := MapCWEStateToC2PAStrict("UNKNOWN_FUTURE_STATE")
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
		required[MapCWEStateToC2PA(in)] = true
	}
	for state, covered := range required {
		assert.True(t, covered, "SRS state %q must be reachable from at least one CWE input", state)
	}
}

func TestLifecycleAssertion_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	a := NewLifecycleAssertion(
		"did:example:c1", "hash1", "pdfhash1", "1.0.1",
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
