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

  @NFR-BR-08 @two-instance
  Scenario: A contract offered on instance A appears on its counterparty B
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as counterparty
    Then the contract appears on instance B in state OFFERED within a few seconds
