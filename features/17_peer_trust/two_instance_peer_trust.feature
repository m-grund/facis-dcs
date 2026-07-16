# Two-instance peer trust — trusted_peers allowlist and cross-instance
# replication (SRS: NFR-BR-08, DCS-FR-CWE-01/-15).
#
# The untrusted-peer scenarios use a single-instance-testable simulation
# technique: a syntactically distinct did:web identity that resolves,
# hostname-wise, to THIS SAME instance's own dev key — see
# steps/peer_trust/dcs_peer_trust_steps.py for why this is honest evidence
# for the trusted_peers allowlist specifically, not just "any" rejection.
#
# The @two-instance scenarios need a second real DCS process and
# BDD_DCS_BASE_URL_A/_B rather than the single-instance BDD_DCS_BASE_URL
# (locally via dev-stack2.sh, in CI via the dcs-a/dcs-b Helm releases).

@NFR-BR-08
Feature: Two-instance peer trust — trusted_peers allowlist and cross-instance replication

  @NFR-BR-08
  Scenario: post_sync from a cryptographically valid but untrusted peer DID is rejected
    Given a cryptographically valid peer DID that is not listed in trusted_peers
    When that peer posts a full-state sync for a brand-new contract to this instance
    Then the post_sync request is rejected because the peer is not in trusted_peers

  @NFR-BR-08
  Scenario: A peer action from a cryptographically valid but untrusted peer DID is rejected
    Given a cryptographically valid peer DID that is not listed in trusted_peers
    And contract "Untrusted Peer Action Target" exists locally, created by this instance
    When that peer attempts to approve contract "Untrusted Peer Action Target" via the peer action endpoint
    Then the peer action request is rejected because the peer is not in trusted_peers
    And the contract "Untrusted Peer Action Target" was not modified by the untrusted peer action

  @NFR-BR-08
  Scenario: post_sync from a seeded, trusted peer DID is accepted and adopted locally
    Given a cryptographically valid peer DID that is listed in trusted_peers
    When that peer posts a full-state sync for a brand-new contract to this instance
    Then the contract data is accepted and stored locally with state "DRAFT"

  # DCS-FR-SM-02 (JAdES): every peer broadcast must carry the SENDER's JAdES
  # baseline-B signature over the canonical contract representation. The
  # challenge-response secret only authenticates the session; this scenario
  # holds session auth and trust listing VALID and breaks only the JAdES
  # payload binding, so the rejection can only come from the receiver's
  # content-signature check.
  @NFR-BR-08 @DCS-FR-SM-02
  Scenario: post_sync whose JAdES signature covers a different contract document is rejected
    Given a cryptographically valid peer DID that is listed in trusted_peers
    When that peer posts a full-state sync whose JAdES signature covers a different contract document
    Then the post_sync request is rejected because the JAdES payload does not match

  Scenario: A raw peer DID can be entered as Reviewer, Approver, and Negotiator without a JWT-sub binding
    Given I am authenticated with roles: "Contract Creator"
    When the initiator creates a contract with a raw peer DID as reviewer, approver, and negotiator
    Then get http 200:Success code
    And the contract is created with that raw peer DID recorded as reviewer, approver, and negotiator

  @NFR-BR-08 @two-instance
  Scenario: A contract offered on instance A with B as negotiator and approver appears as OFFERED on B
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds

  # The stored JAdES artifact (verified at sync time, persisted for
  # independent re-verification) is the contract's cross-instance provenance:
  # instance B can prove WHO sent it the contract content it holds.
  @NFR-BR-08 @DCS-FR-SM-02 @two-instance
  Scenario: A contract synced from instance A carries instance A's verifiable JAdES provenance on B
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds
    And instance B stores a JAdES sync-provenance artifact for that contract signed by instance A

  # Phase 4 (DCS-FR-TR-03, DCS-to-DCS): on receiving a synced contract,
  # instance B resolves its sh:shapesGraph anchor back to instance A's PUBLIC
  # Semantic Hub and re-validates against those exact shapes
  # (validation.VerifyAgainstOriginatorHub, called from post_sync) — not
  # its own local hub, which may run a different active version. This
  # proves the reachability precondition that makes that possible: the
  # sh:shapesGraph anchor instance B received from A really does resolve
  # against instance A's own hub, from outside instance A.
  @NFR-BR-08 @DCS-FR-TR-03 @two-instance
  Scenario: A contract synced from instance A carries a sh:shapesGraph anchor resolvable against instance A's own Semantic Hub
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds
    And the contract's sh:shapesGraph anchor, as stored on instance B, resolves against instance A's Semantic Hub

  @NFR-BR-08 @two-instance
  Scenario: Contract state APPROVED replicates to both instances after negotiation/submit/review/approve
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds
    When the parties complete negotiation acceptance, submit, review, and approval on both sides
    Then the contract state APPROVED is replicated on both instance A and instance B

  # DCS-NFR-BR-06 Revocation & Termination Propagation: revoking a signature
  # MUST take immediate effect — including across instances. Revocation is a
  # SignatureManagement-sourced event (signingmanagement/command/revoke.go),
  # so the dcs-to-dcs synchronizer must broadcast it exactly like the
  # workflow-engine state changes it already replicates; the peer adopts the
  # full contract state (REVOKED) through the same verified post_sync path.
  # SIGNED replication is deliberately not asserted in between: auto-deploy
  # can race SIGNED to ACTIVE, and EventRevoke is valid from either state.
  @DCS-NFR-BR-06 @two-instance
  Scenario: Signature revocation on instance A propagates REVOKED to instance B
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds
    When the parties complete negotiation acceptance, submit, review, and approval on both sides
    Then the contract state APPROVED is replicated on both instance A and instance B
    When instance A applies a ceremony-backed signature to the contract
    And instance A revokes the applied signature of the cross-instance contract
    Then the contract state "REVOKED" is replicated on both instance A and instance B

  # DCS-FR-CWE-15 approval quorum: approve.go flips a contract to APPROVED
  # only once NO approval task remains OPEN, and each approve call flips only
  # the CALLING peer's task (UpdateState matches WHERE approver = CauserDID,
  # and CauserDID is always the executing instance's own peer DID). Proving a
  # PARTIAL quorum therefore needs two observably distinct approver peers,
  # which only the two-instance setup can produce.
  @DCS-FR-CWE-15 @DCS-FR-CWE-25 @UC-03-04 @two-instance
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
