package base

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
)

func TestDecodeSignedAuditLogEntryReturnsEmbeddedEntry(t *testing.T) {
	did := "did:example:contract:1"
	entry := datatype.AuditLogEntry{
		ID:        42,
		Component: "ContractWorkflowEngine",
		EventType: "CREATE_CONTRACT",
		EventData: json.RawMessage(`{"created_by":"alice"}`),
		DID:       &did,
		CreatedAt: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	}
	payload, err := json.Marshal(datatype.SignedAuditLogEntry{
		ID:            7,
		AuditLogEntry: entry,
		TsaSignature:  "test-signature",
	})
	if err != nil {
		t.Fatalf("marshal signed audit log entry: %v", err)
	}

	got, err := decodeSignedAuditLogEntry(payload)
	if err != nil {
		t.Fatalf("decodeSignedAuditLogEntry returned error: %v", err)
	}
	if got.ID != entry.ID || got.Component != entry.Component || got.EventType != entry.EventType {
		t.Fatalf("decoded wrong entry: %+v", got)
	}
	if got.DID == nil || *got.DID != did {
		t.Fatalf("decoded DID mismatch: %+v", got.DID)
	}
	if !got.CreatedAt.Equal(entry.CreatedAt) {
		t.Fatalf("decoded CreatedAt mismatch: %s", got.CreatedAt)
	}
}
