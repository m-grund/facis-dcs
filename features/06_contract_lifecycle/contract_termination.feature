# Contract lifecycle termination (UC-06-02, POST /contract/terminate,
# backend/design/contract_workflow_engine.go). KPI monitoring for ACTIVE
# contracts (UC-06-01) is covered by 05_contract_deployment/
# contract_deployment.feature (Workstream G, AC11/AC12) — not duplicated here.
#
# Renewal (the other half of UC-06-02) is NOT covered: no /contract/renew (or
# equivalent) endpoint exists in backend/design/*.go — grep confirms only
# create/update/submit/negotiate/respond/review/retrieve/search/approve/
# reject/store/terminate/audit/templates/deploy/deployment-callback methods
# on the ContractWorkflowEngine service. Renewal is therefore a genuine gap,
# not a broken test — see the @skip scenario below.

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

  # @skip: FR-CWE-11/12 renewal path has no backend endpoint (SRS §3.1.1 lists
  # no POST /contract/renew or equivalent; grep of backend/design confirms
  # only terminate exists among lifecycle-ending actions). Deviation-register
  # candidate: renewal is v1-undelivered, not merely untested.
  @skip @UC-06-02 @DCS-FR-CWE-22
  Scenario: Renew a contract before its expiry notice period
    Given contract "Renewal Contract" has reached contract state "ACTIVE"
    When the contract manager renews contract "Renewal Contract" for a new term
    Then the contract "Renewal Contract" has an extended expiry date
