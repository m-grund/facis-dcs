package c2pa

import "time"

// LifecycleAssertion is the dcs.contract.lifecycle assertion carried in each C2PA
// manifest (DCS-OR-C2PA-003). It records the contract's state at the time the
// manifest was created so verifiers can reconstruct the full lifecycle history
// by following the prev_manifest_hash chain.
type LifecycleAssertion struct {
	// Label identifies this assertion type.
	Label string `json:"label"`

	// ContractID is the contract's DID.
	ContractID string `json:"contract_id"`

	// FileHash is the SHA-256 of the JSON-LD source bytes (hex-encoded).
	// This binds the manifest to a specific version of the machine-readable content.
	FileHash string `json:"file_hash"`

	// PDFHash is the SHA-256 of the base PDF bytes (hex-encoded) at the time this
	// assertion was created. Together with FileHash it binds the manifest to both
	// representations (DCS-FR-SM-11).
	PDFHash string `json:"pdf_hash"`

	// RendererVersion identifies the renderer build that produced the PDF. A renderer
	// upgrade produces different PDF bytes for the same JSON-LD, so the version must
	// be recorded to allow future bytewise-match verification to select the correct
	// renderer.
	RendererVersion string `json:"renderer_version"`

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

	// PrevManifestHash is the SHA-256 of the previous JUMBF manifest box bytes
	// (hex-encoded), forming a chain of custody. Empty for the first assertion.
	PrevManifestHash string `json:"prev_manifest_hash,omitempty"`
}

const lifecycleAssertionLabel = "org.facis.dcs.contract.lifecycle"

// NewLifecycleAssertion constructs a LifecycleAssertion with the required fields.
func NewLifecycleAssertion(contractID, fileHash, pdfHash, rendererVersion, status, reason, authority, vcID, prevManifestHash string, effectiveAt time.Time) LifecycleAssertion {
	return LifecycleAssertion{
		Label:            lifecycleAssertionLabel,
		ContractID:       contractID,
		FileHash:         fileHash,
		PDFHash:          pdfHash,
		RendererVersion:  rendererVersion,
		Status:           status,
		Reason:           reason,
		EffectiveAt:      effectiveAt,
		Authority:        authority,
		VCId:             vcID,
		PrevManifestHash: prevManifestHash,
	}
}
