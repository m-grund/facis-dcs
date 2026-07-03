// Package datatype holds base-level, domain-agnostic value types shared by
// every domain (audit log entries, outbox events, JSON wrappers, pagination).
package datatype

import (
	"encoding/json"
	"time"
)

// AuditLogEntry is one entry of the tamper-evident audit trail. ResLogPredCID
// and GlobalLogPredCID chain this entry to the previous IPFS-anchored entry
// for the same resource and globally, respectively — retroactively editing
// an entry breaks the chain and is therefore detectable. See
// base/event.OutboxProcessor, which builds and anchors these entries.
type AuditLogEntry struct {
	ID               int64           `json:"id"`
	Component        string          `json:"component"`
	EventType        string          `json:"event_type"`
	EventData        json.RawMessage `json:"event_data"`
	DID              *string         `json:"did"`
	CreatedAt        time.Time       `json:"created_at"`
	ResLogPredCID    *string         `json:"res_log_pred_cid"`
	GlobalLogPredCID *string         `json:"global_log_pred_cid"`
}
