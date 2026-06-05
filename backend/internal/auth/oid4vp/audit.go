package oid4vp

import "context"

// PresentationAuditEvent captures auth presentation outcomes for immutable audit logging.
type PresentationAuditEvent struct {
	PresentationState string
	Success           bool
	SubjectDID        string
	OrganizationID    string
	Roles             []string
	ErrorMessage      string
}

// RecordPresentationAudit is a no-op hook until auth audit logging is implemented.
func RecordPresentationAudit(ctx context.Context, evt PresentationAuditEvent) {
	_ = ctx
	_ = evt
	// TODO: persist to audit trail (actor DID, org, roles, timestamp, failure reason).
}
