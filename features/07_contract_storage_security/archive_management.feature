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

  # DCS-FR-CSA-13: full-text search across archived contract CONTENT (not just
  # name/description metadata). The contracts table keeps a stored tsvector
  # over the entire contract JSON-LD (search_vector, GIN-indexed) and
  # /archive/search?contract_data=... queries it via plainto_tsquery — this
  # scenario drives that path with a term that exists in the contract's data
  # payload, and asserts a nonsense term yields no hit for the same contract.
  # "Confidentiality" is a word inside the standard template's clause text
  # (embedded in the contract JSON-LD, therefore in the stored search_vector),
  # NOT a metadata field — so a hit proves the tsvector covers document
  # CONTENT. The nonsense term proves it is a real full-text match, not a
  # match-everything.
  @UC-07-01 @DCS-FR-CSA-13
  Scenario: Archive full-text search finds a contract by terms inside its content
    Given contract "Fulltext Corpus Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager searches the archive with full-text query "Confidentiality"
    Then get http 200:Success code
    And the archive search result includes contract "Fulltext Corpus Archive Contract"
    When the Archive Manager searches the archive with full-text query "xyzzyplugh nonexistent"
    Then get http 200:Success code
    And the archive search result does not include contract "Fulltext Corpus Archive Contract"

  # DCS-FR-CSA-11: each archived contract can carry a summary (manual or
  # system-generated) and user-assigned tags for thematic categorization and
  # discovery. Annotation is Archive Manager-scoped, mutates ONLY the
  # annotation columns (the archive entry's snapshot/evidence stay immutable,
  # enforced by DB trigger), and is recorded in the archive audit log.
  @UC-07-01 @DCS-FR-CSA-11
  Scenario: Archive Manager annotates an archived contract with a manual summary and tags
    Given contract "Annotated Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager annotates the archived contract "Annotated Archive Contract" with summary "Pilot supply agreement archived for the BDD suite" and tags "pilot-agreement,bdd-supply"
    Then get http 200:Success code
    And the archive entry for contract "Annotated Archive Contract" carries summary "Pilot supply agreement archived for the BDD suite" and tags "pilot-agreement,bdd-supply"
    And the archive annotation of contract "Annotated Archive Contract" is recorded in the archive audit log

  @UC-07-01 @DCS-FR-CSA-11
  Scenario: Searching the archive by tag returns only contracts carrying that tag
    Given contract "Tagged Archive Contract" has reached contract state "SIGNED"
    And contract "Untagged Archive Contract" has reached contract state "SIGNED"
    And the Archive Manager annotates the archived contract "Tagged Archive Contract" with summary "tagged for discovery" and tags "quarterly-review-bdd"
    When the Archive Manager searches the archive by tag "quarterly-review-bdd"
    Then get http 200:Success code
    And the archive search result includes contract "Tagged Archive Contract"
    And the archive search result does not include contract "Untagged Archive Contract"

  # The "automatic ... generation of a summary" half of DCS-FR-CSA-11: when no
  # summary is supplied, the system derives one from the archived contract's
  # own metadata (name, version, state, creator).
  @UC-07-01 @DCS-FR-CSA-11
  Scenario: Annotating without a summary generates one from the contract metadata
    Given contract "Auto Summary Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager annotates the archived contract "Auto Summary Archive Contract" with tags "auto-summary-bdd" and no summary
    Then get http 200:Success code
    And the archive entry for contract "Auto Summary Archive Contract" carries a generated summary derived from its version and state

  @UC-07-02 @DCS-FR-CSA-11
  Scenario: A read-only role cannot annotate an archived contract
    Given contract "Unauthorized Annotation Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Contract Observer"
    When I attempt to annotate the archived contract "Unauthorized Annotation Contract" with my current role
    Then the request is denied with a client error
