# Requirement: two-instance-peer-trust
#
# Covers Workstream C1-C3 ("Two-instance inter-org demo", docs/anforderung.md)
# — only the ACs the analyst marked Pruefmittel = BDD:
#
#   AC2 — post_sync from a cryptographically valid but untrusted peer DID is
#         rejected and not applied.
#   AC3 — action (peer action) from a cryptographically valid but untrusted
#         peer DID is rejected and not executed.
#   AC4 — post_sync from a seeded, trusted peer DID is accepted and the
#         contract state is adopted locally.
#   AC6 — a raw peer DID can be entered/selected as Reviewer/Approver/
#         Negotiator at contract creation without being blocked by a
#         JWT-sub binding.
#   AC7 — a contract created+offered on instance A with B as negotiator +
#         approver appears on B as OFFERED within a few seconds.
#   AC8 — after negotiation/accept/submit/review/approve on both sides, state
#         APPROVED is replicated on both A and B.
#
# Deliberately OUT of scope for this pack (Pruefmittel != BDD, checked by the
# verifier against recorded manual/extern evidence instead):
#   - AC1 (manueller-Drill)
#   - AC5 (manueller-Drill)
#
# AC2/AC3 use a single-instance-testable simulation technique (a syntactically
# distinct did:web identity that resolves, hostname-wise, to THIS SAME
# instance's own dev key) — see steps/peer_trust/dcs_peer_trust_steps.py for
# why this is honest evidence for the trusted_peers allowlist specifically,
# not just "any" rejection. AC7/AC8 are @two-instance: they need a second real
# DCS process (Workstream C2, not built yet) and BDD_DCS_BASE_URL_A/_B rather
# than the single-instance BDD_DCS_BASE_URL — see the module docstring in the
# step file for the open points these surfaced (missing C2 runner; and a
# genuine Offered -> Negotiation gap in the C4 transition table found while
# writing AC8).

@NFR-BR-08
Feature: Two-instance peer trust — trusted_peers allowlist and cross-instance replication

  @REQ-two-instance-peer-trust-AC2 @NFR-BR-08
  Scenario: post_sync from a cryptographically valid but untrusted peer DID is rejected
    Given a cryptographically valid peer DID that is not listed in trusted_peers
    When that peer posts a full-state sync for a brand-new contract to this instance
    Then the post_sync request is rejected because the peer is not in trusted_peers

  @REQ-two-instance-peer-trust-AC3 @NFR-BR-08
  Scenario: A peer action from a cryptographically valid but untrusted peer DID is rejected
    Given a cryptographically valid peer DID that is not listed in trusted_peers
    And contract "Untrusted Peer Action Target" exists locally, created by this instance
    When that peer attempts to approve contract "Untrusted Peer Action Target" via the peer action endpoint
    Then the peer action request is rejected because the peer is not in trusted_peers
    And the contract "Untrusted Peer Action Target" was not modified by the untrusted peer action

  @REQ-two-instance-peer-trust-AC4 @NFR-BR-08
  Scenario: post_sync from a seeded, trusted peer DID is accepted and adopted locally
    Given a cryptographically valid peer DID that is listed in trusted_peers
    When that peer posts a full-state sync for a brand-new contract to this instance
    Then the contract data is accepted and stored locally with state "DRAFT"

  @REQ-two-instance-peer-trust-AC6
  Scenario: A raw peer DID can be entered as Reviewer, Approver, and Negotiator without a JWT-sub binding
    Given I am authenticated with roles: "Contract Creator"
    When the initiator creates a contract with a raw peer DID as reviewer, approver, and negotiator
    Then get http 200:Success code
    And the contract is created with that raw peer DID recorded as reviewer, approver, and negotiator

  @REQ-two-instance-peer-trust-AC7 @NFR-BR-08 @two-instance
  Scenario: A contract offered on instance A with B as negotiator and approver appears as OFFERED on B
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds

  @REQ-two-instance-peer-trust-AC8 @NFR-BR-08 @two-instance
  Scenario: Contract state APPROVED replicates to both instances after negotiation/submit/review/approve
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds
    When the parties complete negotiation acceptance, submit, review, and approval on both sides
    Then the contract state APPROVED is replicated on both instance A and instance B

  # DCS-FR-CWE-15 approval quorum: approve.go flips a contract to APPROVED
  # only once NO approval task remains OPEN, and each approve call flips only
  # the CALLING peer's task (UpdateState matches WHERE approver = CauserDID,
  # and CauserDID is always the executing instance's own peer DID). Proving a
  # PARTIAL quorum therefore needs two observably distinct approver peers —
  # this scenario supersedes the former single-instance @skip
  # "All required approvals gathered" in 03/contract_creation/
  # contract_approval.feature, which could never produce two distinct
  # approver identities on one instance.
  @REQ-two-instance-peer-trust-AC9 @DCS-FR-CWE-15 @DCS-FR-CWE-25 @UC-03-04 @two-instance
  Scenario: Approval quorum — one of two approver peers is not enough, both together complete it
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract requiring approval from both instances
    Then the contract appears on instance B in state OFFERED within a few seconds
    When instance A drives the contract to the approval stage
    And instance A's approver approves the contract
    Then the contract is still not APPROVED because instance B's required approval is open
    When instance B's approver approves the contract
    Then the contract state APPROVED is replicated on both instance A and instance B
    And both peers' approval decisions are recorded on the contract's approval tasks
