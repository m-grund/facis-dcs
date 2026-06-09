package datatype

type SignedAuditLogEntry struct {
	ID            int64         `json:"id"`
	AuditLogEntry AuditLogEntry `json:"audit_log_entry"`
	TsaSignature  string        `json:"tsa_signature"`
}
