# Contract lifecycle termination and renewal (UC-06-02, POST /contract/
# terminate + POST /contract/renew, backend/design/contract_workflow_engine.go).
# KPI monitoring for ACTIVE contracts (UC-06-01) is covered by
# 05_contract_deployment/contract_deployment.feature (Workstream G,
# AC11/AC12) — not duplicated here.
#
# Renewal (DCS-FR-CWE-11/22, DCS-FR-CSA-15) creates a NEW, independently
# versioned contract instance rather than mutating the original's expiry date
# in place: SRS DCS-FR-CWE-11 ("Renewals MUST generate a new contract
# instance with reference links") and DCS-FR-CSA-15 ("creation of renewal or
# extension contracts linked to archived originals... retain references to
# the prior contract's version, ID, and signatures") both describe a linked
# sibling document, not an edit of the original. The original contract is
# left completely intact; the new instance starts in DRAFT and carries a
# dcs:renewsContract JSON-LD back-reference to the original's DID and
# version (see backend/internal/contractworkflowengine/command/renew.go).
# The scenarios below assert that honest model, not an in-place expiry-date
# mutation.

@UC-06-02 @DCS-FR-CWE-11 @DCS-FR-CWE-12
Feature: Contract termination

  @REQ-contract-termination-AC1 @UC-06-02
  Scenario: Contract Manager terminates an approved contract
    Given contract "Termination Contract" has reached contract state "APPROVED"
    When the contract manager terminates contract "Termination Contract" with reason "BDD termination test"
    Then get http 200:Success code
    And the contract "Termination Contract" is in state "TERMINATED"
    And the contract "Termination Contract" has an audit event of type "TERMINATE_CONTRACT"

  @REQ-contract-termination-AC2 @UC-06-02
  Scenario: A terminated contract cannot be terminated again
    Given contract "Double Termination Contract" has reached contract state "TERMINATED"
    When the contract manager terminates contract "Double Termination Contract" with reason "second attempt"
    Then the request is denied with a client error

  @REQ-contract-termination-AC3 @UC-06-02 @DCS-FR-CWE-11 @DCS-FR-CWE-22
  Scenario: Contract Manager renews a contract before its expiry notice period
    Given contract "Renewal Contract" has reached contract state "SIGNED"
    And contract "Renewal Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract manager renews contract "Renewal Contract" for a new term
    Then get http 200:Success code
    And the renewal of "Renewal Contract" is a new contract in state "DRAFT"
    And the renewal of "Renewal Contract" has its own term dates
    And the contract "Renewal Contract" is in state "ACTIVE"

  @DCS-FR-CSA-15 @DCS-FR-CWE-22 @UC-06-02
  Scenario: Renewal contract references the original contract's DID and version
    Given contract "Renewal Source Contract" has reached contract state "SIGNED"
    And contract "Renewal Source Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract manager renews contract "Renewal Source Contract" for a new term
    Then get http 200:Success code
    And the renewal of "Renewal Source Contract" references the original contract's DID and version
