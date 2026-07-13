# Contract Storage & Archive (UC-07, backend/design/contract_storage_archive.go).
# Archive-entry creation itself (at SIGNED, not APPROVED) and its evidence
# content are covered by 05_contract_deployment/contract_deployment.feature
# — this file covers retrieval, search, RBAC scope, and the audit trail of
# the /archive/* endpoints themselves, which the deployment pack does not
# exercise.

@UC-07 @DCS-IR-CSA-01 @DCS-IR-CSA-05
Feature: Contract storage and archive retrieval

  @UC-07-01 @DCS-IR-CSA-01
  Scenario: Archive Manager retrieves the full archive list
    Given contract "Archive Retrieve Contract" has reached contract state "SIGNED"
    When the Archive Manager retrieves the archive
    Then get http 200:Success code
    And the archive retrieval result includes contract "Archive Retrieve Contract"

  @UC-07-01 @DCS-IR-CSA-01
  Scenario: Archive search filters by contract state
    Given contract "Archive Search Contract" has reached contract state "SIGNED"
    When the Archive Manager searches the archive with state filter "SIGNED"
    Then get http 200:Success code
    And the archive search result includes contract "Archive Search Contract"

  @UC-07-02 @DCS-IR-CSA-05
  Scenario: A role outside the archive scope cannot retrieve the archive
    Given I am authenticated with roles: "Template Creator"
    When I attempt to retrieve the archive with my current role
    Then the request is denied with a client error

  @UC-07-03 @DCS-IR-CSA-04
  Scenario: Auditor retrieves the archive audit log
    Given contract "Archive Audit Contract" has reached contract state "SIGNED"
    When the Auditor retrieves the archive audit log
    Then get http 200:Success code
    And the archive audit log is a non-empty list

  @UC-07-03 @DCS-FR-CSA-17
  Scenario: Archive Manager deletes an archived contract with a logged justification
    Given contract "Archive Deletion Contract" has reached contract state "SIGNED"
    When the Archive Manager deletes the archived contract "Archive Deletion Contract" with justification "no longer needed for compliance retention"
    Then get http 200:Success code
    And the archive deletion of contract "Archive Deletion Contract" is recorded in the archive audit log

  @UC-07-03 @DCS-FR-CSA-17
  Scenario: A role outside the archive scope cannot delete an archived contract
    Given contract "Unauthorized Archive Deletion Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Template Creator"
    When I attempt to delete the archived contract "Unauthorized Archive Deletion Contract" with my current role
    Then the request is denied with a client error

  # DCS-IR-CSA-06: read-only users (Contract Observer) can view archived
  # records but MUST NOT be able to modify or delete entries. The design
  # scopes /archive/retrieve+search to Archive Manager AND Contract Observer,
  # while /archive/store and /archive/delete are Archive Manager only
  # (backend/design/contract_storage_archive.go) — this scenario asserts
  # both halves of that contract against the running service.
  @UC-07-03 @DCS-IR-CSA-06
  Scenario: A read-only Observer can view the archive but cannot delete from it
    Given contract "Observer Readonly Archive Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Contract Observer"
    When I attempt to retrieve the archive with my current role
    Then get http 200:Success code
    And the archive retrieval result includes contract "Observer Readonly Archive Contract"
    When I attempt to delete the archived contract "Observer Readonly Archive Contract" with my current role
    Then the request is denied with a client error
