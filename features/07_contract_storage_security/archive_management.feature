# Contract Storage & Archive (UC-07, backend/design/contract_storage_archive.go).
# Archive-entry creation itself (at SIGNED, not APPROVED) and its evidence
# content are covered by 05_contract_deployment/contract_deployment.feature
# (Workstream G, AC1/AC9) — this file covers retrieval, search, RBAC scope,
# and the audit trail of the /archive/* endpoints themselves, which G's pack
# does not exercise.

@UC-07 @DCS-IR-CSA-01 @DCS-IR-CSA-05
Feature: Contract storage and archive retrieval

  @REQ-archive-management-AC1 @UC-07-01 @DCS-IR-CSA-01
  Scenario: Archive Manager retrieves the full archive list
    Given contract "Archive Retrieve Contract" has reached contract state "SIGNED"
    When the Archive Manager retrieves the archive
    Then get http 200:Success code
    And the archive retrieval result includes contract "Archive Retrieve Contract"

  @REQ-archive-management-AC2 @UC-07-01 @DCS-IR-CSA-01
  Scenario: Archive search filters by contract state
    Given contract "Archive Search Contract" has reached contract state "SIGNED"
    When the Archive Manager searches the archive with state filter "SIGNED"
    Then get http 200:Success code
    And the archive search result includes contract "Archive Search Contract"

  @REQ-archive-management-AC3 @UC-07-02 @DCS-IR-CSA-05
  Scenario: A role outside the archive scope cannot retrieve the archive
    Given I am authenticated with roles: "Template Creator"
    When I attempt to retrieve the archive with my current role
    Then the request is denied with a client error

  # @skip: GET /archive/audit is an unimplemented stub
  # (backend/internal/service/contract_storage_archive.go's Audit handler
  # only logs and returns (nil, nil) — verified by an actual run against the
  # dev stack: the endpoint returns 200 with a null body, never real audit
  # entries). The contract-level and process-level audit trails
  # (POST /contract/audit, POST /pac/audit) ARE implemented and exercised
  # elsewhere (contract_state_machine_steps.py's audit-event step,
  # process_audit_and_compliance.feature) — only this archive-specific audit
  # endpoint is a stub.
  @skip @REQ-archive-management-AC4 @UC-07-03 @DCS-IR-CSA-04
  Scenario: Auditor retrieves the archive audit log
    Given contract "Archive Audit Contract" has reached contract state "SIGNED"
    When the Auditor retrieves the archive audit log
    Then get http 200:Success code
    And the archive audit log is a non-empty list
