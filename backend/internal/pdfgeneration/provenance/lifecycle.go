package provenance

import (
	"fmt"
	"strings"
	"time"
)

// LifecycleAssertion is the dcs.contract.lifecycle assertion carried in each C2PA
// manifest (DCS-OR-C2PA-003). It records the contract's state at the time the
// manifest was created so verifiers can reconstruct the full lifecycle history.
type LifecycleAssertion struct {
	// Label identifies this assertion type.
	Label string `json:"label"`

	// ContractID is the contract's DID.
	ContractID string `json:"contract_id"`

	// FileHash is the SHA-256 of the protected PDF artifact bytes (hex-encoded)
	// immediately before the current manifest append operation. This is the
	// SRS-required binding field used in both lifecycle assertions and
	// VC credentialSubject.file_hash.
	FileHash string `json:"file_hash"`

	// Status is the contract lifecycle state at assertion time
	// (draft, active, amended, suspended, terminated, expired, replaced).
	Status string `json:"status"`

	// Reason is the human-readable reason for the state transition (may be empty).
	Reason string `json:"reason,omitempty"`

	// EffectiveAt is when this lifecycle state became effective.
	EffectiveAt time.Time `json:"effective_at"`

	// Authority is the DID of the entity asserting this lifecycle event.
	Authority string `json:"authority"`

	// VCId is the identifier of the W3C VC that records this lifecycle event
	// (DCS-OR-C2PA-004). Empty until the VC is issued.
	VCId string `json:"vc_id,omitempty"`
}

const lifecycleAssertionLabel = "org.facis.dcs.contract.lifecycle"

// NewLifecycleAssertion constructs a LifecycleAssertion with the required fields.
func NewLifecycleAssertion(contractID, fileHash, status, reason, authority, vcID string, effectiveAt time.Time) LifecycleAssertion {
	return LifecycleAssertion{
		Label:       lifecycleAssertionLabel,
		ContractID:  contractID,
		FileHash:    fileHash,
		Status:      status,
		Reason:      reason,
		EffectiveAt: effectiveAt,
		Authority:   authority,
		VCId:        vcID,
	}
}

// MapCWEStateToC2PA maps a CWE contract state to the canonical C2PA lifecycle
// vocabulary defined in DCS-OR-C2PA-003. Unsupported states return an error.
func MapCWEStateToC2PA(cweState string) (string, error) {
	switch strings.ToUpper(cweState) {
	case "DRAFT":
		return "draft", nil
	case "SUBMITTED", "REVIEWED", "APPROVED":
		// Reviewed/submitted/approved are intermediate steps toward an active
		// contract; map to "active" as the closest SRS equivalent.
		return "active", nil
	case "NEGOTIATION", "REJECTED":
		// Negotiation and rejection are amendment/review cycles before the
		// contract becomes active; treated as "amended" (under negotiation)
		// or "active" (re-submitted after rejection).
		// Use "amended" because the content may have changed.
		return "amended", nil
	case "TERMINATED":
		return "terminated", nil
	case "EXPIRED":
		return "expired", nil
	case "SUSPENDED":
		return "suspended", nil
	case "REPLACED":
		return "replaced", nil
	default:
		return "", fmt.Errorf("unsupported lifecycle state %q (allowed: DRAFT,SUBMITTED,REVIEWED,APPROVED,NEGOTIATION,REJECTED,TERMINATED,EXPIRED,SUSPENDED,REPLACED)", cweState)
	}
}
