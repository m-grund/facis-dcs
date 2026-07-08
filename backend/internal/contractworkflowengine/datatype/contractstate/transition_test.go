package contractstate

import (
	"errors"
	"testing"
)

// TestOfferAndWithdrawTransitions covers the C4 contract-state-machine-
// refactor ACs directly against the transition table (fast, no HTTP/DB
// needed) — the BDD scenarios in
// features/03_contract_creation/contract_state_machine_refactor.feature
// exercise the same rules end-to-end.
func TestOfferAndWithdrawTransitions(t *testing.T) {
	// AC1: DRAFT -> OFFERED via Offer.
	if err := ValidateTransition(Draft, EventOffer); err != nil {
		t.Fatalf("expected Offer to be allowed from Draft, got: %v", err)
	}
	if !IsAllowed(Draft, EventOffer, Offered) {
		t.Fatalf("expected Draft -Offer-> Offered to be a declared outcome")
	}

	// AC2: Withdraw must succeed from OFFERED/NEGOTIATION/SUBMITTED/REVIEWED.
	for _, from := range []ContractState{Offered, Negotiation, Submitted, Reviewed} {
		if err := ValidateTransition(from, EventWithdraw); err != nil {
			t.Fatalf("expected Withdraw to be allowed from %s, got: %v", from, err)
		}
		if !IsAllowed(from, EventWithdraw, Withdrawn) {
			t.Fatalf("expected %s -Withdraw-> Withdrawn to be a declared outcome", from)
		}
	}

	// AC3: Withdraw must be rejected once APPROVED.
	err := ValidateTransition(Approved, EventWithdraw)
	if err == nil {
		t.Fatalf("expected Withdraw from Approved to be rejected")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got: %v", err)
	}

	// AC4: Approve is rejected from Draft.
	err = ValidateTransition(Draft, EventApprove)
	if err == nil || !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected Approve from Draft to be rejected with ErrInvalidTransition, got: %v", err)
	}

	// AC5/AC6: the pre-existing Submit -> Reviewed -> Approve -> Sign path
	// still works under the new table.
	if err := ValidateTransition(Submitted, EventSubmit); err != nil {
		t.Fatalf("expected Submit to remain allowed from Submitted, got: %v", err)
	}
	if err := ValidateTransition(Reviewed, EventApprove); err != nil {
		t.Fatalf("expected Approve to be allowed from Reviewed, got: %v", err)
	}
	if err := ValidateTransition(Approved, EventSign); err != nil {
		t.Fatalf("expected Sign to be allowed from Approved, got: %v", err)
	}
	if !IsAllowed(Approved, EventSign, Signed) {
		t.Fatalf("expected Approved -Sign-> Signed to be a declared outcome")
	}
}

// TestSubmitAllowedFromOffered covers the C1-C3 two-instance-peer-trust
// gap found while writing AC8 of features/17_peer_trust: the documented
// sequence DRAFT -> OFFERED -> NEGOTIATION -> SUBMITTED -> ... requires an
// Offered -Submit-> Negotiation edge, analogous to the pre-existing
// Draft -Submit-> Negotiation edge.
func TestSubmitAllowedFromOffered(t *testing.T) {
	if err := ValidateTransition(Offered, EventSubmit); err != nil {
		t.Fatalf("expected Submit to be allowed from Offered, got: %v", err)
	}
	if !IsAllowed(Offered, EventSubmit, Negotiation) {
		t.Fatalf("expected Offered -Submit-> Negotiation to be a declared outcome")
	}
	if err := ValidateOutcome(Offered, EventSubmit, Negotiation); err != nil {
		t.Fatalf("expected Offered -Submit-> Negotiation to be a valid outcome, got: %v", err)
	}
	if err := ValidateOutcome(Offered, EventSubmit, Approved); err == nil {
		t.Fatalf("expected Offered -Submit-> Approved to be rejected as an undeclared outcome")
	}
}

func TestTerminateAllowedFromEveryNonTerminalState(t *testing.T) {
	nonTerminal := []ContractState{
		Draft, Offered, Rejected, Negotiation, Submitted, Reviewed, Approved,
		Signed, Active, Revoked, Withdrawn, Expired,
	}
	for _, from := range nonTerminal {
		if err := ValidateTransition(from, EventTerminate); err != nil {
			t.Fatalf("expected Terminate to be allowed from %s, got: %v", from, err)
		}
		if !IsAllowed(from, EventTerminate, Terminated) {
			t.Fatalf("expected %s -Terminate-> Terminated to be a declared outcome", from)
		}
	}

	if EventAllowed(Terminated, EventTerminate) {
		t.Fatalf("expected Terminate to be rejected once already Terminated")
	}
}

func TestValidateOutcomeRejectsUndeclaredTarget(t *testing.T) {
	// Submit from Draft may only reach Negotiation, never e.g. Approved.
	if err := ValidateOutcome(Draft, EventSubmit, Approved); err == nil {
		t.Fatalf("expected Draft -Submit-> Approved to be rejected as an undeclared outcome")
	}
	if err := ValidateOutcome(Draft, EventSubmit, Negotiation); err != nil {
		t.Fatalf("expected Draft -Submit-> Negotiation to be a declared outcome, got: %v", err)
	}
}

func TestAllContractStatesAreValid(t *testing.T) {
	for _, s := range []ContractState{
		Draft, Offered, Rejected, Withdrawn, Negotiation, Submitted, Reviewed,
		Approved, Signed, Active, Revoked, Terminated, Expired,
	} {
		if !s.IsValid() {
			t.Fatalf("expected %s to be a valid contract state", s)
		}
	}
}
