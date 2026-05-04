package event

import (
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"time"
)

// CreateEvent is emitted when a new contract is created.
type CreateEvent struct {
	DID          string         `json:"did"`
	TemplateDID  string         `json:"template_did"`
	CreatedBy    string         `json:"created_by"`
	Name         *string        `json:"name"`
	Description  *string        `json:"description"`
	ContractData *datatype.JSON `json:"contract_data"`
	OccurredAt   time.Time      `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e CreateEvent) EventType() string {
	return eventtype.Create.String()
}

// GetDID implements the Event interface.
func (e CreateEvent) GetDID() string {
	return e.DID
}

// UpdateEvent is emitted when contract data is updated.
type UpdateEvent struct {
	DID                string         `json:"did"`
	UpdatedBy          string         `json:"updated_by"`
	OldContractVersion *int           `json:"old_contract_version,omitempty"`
	NewContractVersion *int           `json:"new_contract_version,omitempty"`
	OldName            *string        `json:"old_name,omitempty"`
	NewName            *string        `json:"new_name,omitempty"`
	OldDescription     *string        `json:"old_description,omitempty"`
	NewDescription     *string        `json:"new_description,omitempty"`
	OldContractData    *datatype.JSON `json:"old_contract_data,omitempty"`
	NewContractData    *datatype.JSON `json:"new_contract_data,omitempty"`
	OccurredAt         time.Time      `json:"occurred_at"`
	OldExpirationDate  *time.Time     `json:"old_expiration_date,omitempty"`
	NewExpirationDate  *time.Time     `json:"new_expiration_date,omitempty"`
}

// EventType implements the Event interface.
func (e UpdateEvent) EventType() string {
	return eventtype.Update.String()
}

// GetDID implements the Event interface.
func (e UpdateEvent) GetDID() string {
	return e.DID
}

// SubmitEvent is emitted when a contract is submitted
type SubmitEvent struct {
	DID             string                 `json:"did"`
	PreviousState   string                 `json:"previous_state"`
	NewState        string                 `json:"new_state"`
	SubmittedBy     string                 `json:"submitted_by"`
	OccurredAt      time.Time              `json:"occurred_at"`
	ContractVersion *int                   `json:"contract_version,omitempty"`
	ActionFlag      *actionflag.ActionFlag `json:"action_flag,omitempty"`
	Comments        []string               `json:"comments"`
}

// EventType implements the Event interface.
func (e SubmitEvent) EventType() string {
	return eventtype.Submit.String()
}

// GetDID implements the Event interface.
func (e SubmitEvent) GetDID() string {
	return e.DID
}

// RetrieveByIDEvent is emitted when contract data is retrieved.
type RetrieveByIDEvent struct {
	DID         string    `json:"did"`
	RetrievedBy string    `json:"retrieved_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RetrieveByIDEvent) EventType() string {
	return eventtype.RetrieveByID.String()
}

// GetDID implements the Event interface.
func (e RetrieveByIDEvent) GetDID() string {
	return e.DID
}

// RetrieveAllEvent is emitted when contract data is retrieved.
type RetrieveAllEvent struct {
	RetrievedBy string    `json:"retrieved_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RetrieveAllEvent) EventType() string {
	return eventtype.RetrieveAll.String()
}

// GetDID implements the Event interface.
func (e RetrieveAllEvent) GetDID() string {
	return "*"
}

// VerifyEvent is emitted when a template is verified.
type VerifyEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	VerifiedBy      string    `json:"verified_by"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e VerifyEvent) EventType() string {
	return eventtype.Verify.String()
}

// GetDID implements the Event interface.
func (e VerifyEvent) GetDID() string {
	return e.DID
}

// NegotiationEvent is emitted when a template is verified.
type NegotiationEvent struct {
	DID             string         `json:"did"`
	ContractVersion *int           `json:"contract_version,omitempty"`
	ChangeRequest   *datatype.JSON `json:"change_request,omitempty"`
	NegotiatedBy    string         `json:"negotiated_by"`
	OccurredAt      time.Time      `json:"occurred_at"`
	Negotiators     []string       `json:"negotiators"`
}

// EventType implements the Event interface.
func (e NegotiationEvent) EventType() string {
	return eventtype.Negotiation.String()
}

// GetDID implements the Event interface.
func (e NegotiationEvent) GetDID() string {
	return e.DID
}

// AcceptNegotiationEvent is emitted when a template is verified.
type AcceptNegotiationEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	AcceptedBy      string    `json:"accepted_by"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e AcceptNegotiationEvent) EventType() string {
	return eventtype.AcceptRespond.String()
}

// GetDID implements the Event interface.
func (e AcceptNegotiationEvent) GetDID() string {
	return e.DID
}

// RejectNegotiationEvent is emitted when a template is verified.
type RejectNegotiationEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	RejectedBy      string    `json:"rejected_by"`
	RejectionReason *string   `json:"rejection_reason,omitempty"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RejectNegotiationEvent) EventType() string {
	return eventtype.RejectRespond.String()
}

// GetDID implements the Event interface.
func (e RejectNegotiationEvent) GetDID() string {
	return e.DID
}

// ApproveEvent is emitted when a contract is approved.
type ApproveEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	ApprovedBy      string    `json:"approved_by"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e ApproveEvent) EventType() string {
	return eventtype.Approve.String()
}

// GetDID implements the Event interface.
func (e ApproveEvent) GetDID() string {
	return e.DID
}

// RejectEvent is emitted when a contract is rejected.
type RejectEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	RejectedBy      string    `json:"rejected_by"`
	Reason          string    `json:"reason"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RejectEvent) EventType() string {
	return eventtype.Reject.String()
}

// GetDID implements the Event interface.
func (e RejectEvent) GetDID() string {
	return e.DID
}

// TerminateEvent is emitted when a contract is terminated.
type TerminateEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	Reason          string    `json:"reason"`
	TerminatedBy    string    `json:"terminated_by"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e TerminateEvent) EventType() string {
	return eventtype.Terminate.String()
}

// GetDID implements the Event interface.
func (e TerminateEvent) GetDID() string {
	return e.DID
}

// RecordEvidenceEvent is emitted when an evidence is recorded
type RecordEvidenceEvent struct {
	DID             string    `json:"did"`
	ContractVersion *int      `json:"contract_version,omitempty"`
	RecordedBy      string    `json:"recorded_by"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RecordEvidenceEvent) EventType() string {
	return eventtype.RecordEvidence.String()
}

// GetDID implements the Event interface.
func (e RecordEvidenceEvent) GetDID() string {
	return e.DID
}

// AuditEvent is emitted when the contract is audited
type AuditEvent struct {
	DID           string                      `json:"did"`
	AuditedBy     string                      `json:"audited_by"`
	OccurredAt    time.Time                   `json:"occurred_at"`
	ComponentType componenttype.ComponentType `json:"component_type"`
}

// EventType implements the Event interface.
func (e AuditEvent) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvent) GetDID() string {
	return e.DID
}

// ReviewEvent is emitted when contract is reviewed.
type ReviewEvent struct {
	DID        string    `json:"did"`
	ReviewedBy string    `json:"reviewed_by"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e ReviewEvent) EventType() string {
	return eventtype.Review.String()
}

// GetDID implements the Event interface.
func (e ReviewEvent) GetDID() string {
	return e.DID
}

// IncreaseContractVersionEvent is emitted when change requests for contract merged
type IncreaseContractVersionEvent struct {
	DID                string    `json:"did"`
	OldContractVersion *int      `json:"old_contract_version,omitempty"`
	NewContractVersion *int      `json:"new_contract_version,omitempty"`
	SubmittedBy        string    `json:"submitted_by"`
	OccurredAt         time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e IncreaseContractVersionEvent) EventType() string {
	return eventtype.IncreaseContractVersion.String()
}

// GetDID implements the Event interface.
func (e IncreaseContractVersionEvent) GetDID() string {
	return e.DID
}
