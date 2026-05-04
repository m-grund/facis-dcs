package datatype

import (
	"encoding/json"
	"time"
)

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
