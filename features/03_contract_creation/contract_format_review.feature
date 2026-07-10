@UC-03-05 @FR-CWE-04
Feature: Machine-Readable and Human-Readable Contract Review
  Contract Creators, Contract Reviewers, and Contract Managers review
  contracts in both machine-readable and human-readable formats. The system
  ensures synchronization and highlights any inconsistencies.

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: View contract in machine-readable format
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists
    When I view contract "Service Agreement" in machine-readable format
    Then the JSON-LD or XML representation is displayed
    And the structure is valid

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: View contract in human-readable format
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists
    When I view contract "Service Agreement" in human-readable format
    Then the PDF or document view is displayed
    And the content is readable

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: Synchronized view of both formats
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" exists
    When I request synchronized view of contract "Service Agreement"
    Then both machine-readable and human-readable views are rendered
    And both formats are derived from the same source
    And both formats have matching content hashes

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: System highlights inconsistencies between formats
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" has a formatting error
    When I review both formats of contract "Service Agreement"
    Then the system highlights inconsistencies
    And the specific discrepancies are identified

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: Export both formats with same version tag
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" with version "2.0" exists
    When I export contract "Service Agreement" in both formats
    Then the machine-readable export has version tag "2.0"
    And the human-readable export has version tag "2.0"
    And both exports are consistent

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: Validate machine-readable structure
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" has machine-readable representation
    When I validate the machine-readable structure
    Then the schema validation passes
    And required fields are present
    And data types are correct

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: Fix inconsistency and re-validate
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a detected inconsistency
    When I fix the inconsistency
    And I re-validate contract "Service Agreement"
    Then no inconsistencies are highlighted
    And both formats are synchronized

  # @skip: step definitions not implemented yet (undefined steps would fail the run)
  @skip
  Scenario: Unauthorized role cannot access format review
    Given I am authenticated with roles: "Contract Observer"
    And contract "Service Agreement" exists
    When I attempt to access the synchronized view of contract "Service Agreement"
    Then the request is denied with an authorization error

  @DCS-FR-CWE-04
  Scenario: Export contract as PDF and verify MR/HR hash match
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists in "Under Review" state
    When I export contract "Service Agreement" as PDF
    Then the response is a valid PDF document
    And the PDF contains an embedded JSON-LD attachment named "contract.jsonld"
    And the embedded JSON-LD matches the contract source

  @DCS-FR-CWE-04 @DCS-FR-CWE-05
  Scenario: Verify MR/HR content hash consistency
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF
    When I verify the MR/HR hash consistency for contract "Service Agreement"
    Then the verification result shows match is true
    And the response includes jsonld_hash and base_pdf_hash

  @DCS-FR-CWE-04 @skip
  Scenario: Tampered PDF fails hash verification
    # This scenario requires injecting a tampered PDF into IPFS, which is
    # covered by the Go unit tests in verify/verifier_test.go.
    # Integration-level tampering detection is not exercised here.
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF with a tampered base layer
    When I verify the MR/HR hash consistency for contract "Service Agreement"
    Then the verification result shows match is false
