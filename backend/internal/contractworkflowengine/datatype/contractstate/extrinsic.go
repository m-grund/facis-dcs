package contractstate

import "strings"

// ExtrinsicLifecycle is the peer-facing, INFERRED negotiation lifecycle a
// contract presents across the DCS-to-DCS boundary (ADR-13): the two instances
// derive the same value from the shared artifact and their own intrinsic state.
// It is distinct from both the intrinsic (local RBAC) state and the C2PA banner
// (which stays SRS draft/active): the negotiation phases live here.
type ExtrinsicLifecycle string

const (
	// Proposed: a version is on the table and being negotiated (offer +
	// counteroffers). This is the whole pre-settlement negotiation.
	Proposed ExtrinsicLifecycle = "proposed"
	// Agreed: both parties settled/consolidated on the same version. The
	// signing gate opens here.
	Agreed ExtrinsicLifecycle = "agreed"
	// Executed: all declared parties have signed.
	Executed ExtrinsicLifecycle = "executed"
)

// InferExtrinsic projects the extrinsic lifecycle from a contract's intrinsic
// state. The pre-settlement formation states all read as "proposed"; internal
// approval on both sides is the settlement, so APPROVED reads as "agreed"; a
// signed contract is "executed". Off-ramps pass through as their lowercase
// state so a caller can still tell why a contract left the happy path.
func InferExtrinsic(intrinsicState string) ExtrinsicLifecycle {
	switch strings.ToUpper(intrinsicState) {
	case Draft.String(), Offered.String(), Negotiation.String(), Submitted.String(), Reviewed.String():
		return Proposed
	case Approved.String():
		return Agreed
	case Signed.String(), Active.String():
		return Executed
	default:
		return ExtrinsicLifecycle(strings.ToLower(intrinsicState))
	}
}
