package command

import "errors"

// ErrNotAParty indicates the caller (resolved server-side to a peer DID, see
// package doc on CauserDID) is not among the contract's registered
// negotiator/reviewer parties (FR-CWE-18, FR-CWE-07): negotiate/respond used
// to reject this with a bare "invalid permissions"/"invalid user" string
// mapped to a 500 by the HTTP layer's error dispatch — indistinguishable
// from a genuine server fault. This sentinel lets the HTTP layer map the
// same rejection to a proper 4xx client error instead.
var ErrNotAParty = errors.New("not a party to this contract")

// ErrConflictOfInterest indicates the caller attempted to accept a
// negotiation change_request they themselves authored (FR-CWE-07: a
// reviewer/negotiator may not approve their own redline proposal).
var ErrConflictOfInterest = errors.New("conflict of interest - cannot approve own proposal")
