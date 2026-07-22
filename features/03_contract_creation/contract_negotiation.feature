@UC-03-02 @FR-CWE-08 @FR-CWE-14 @FR-CWE-18 @FR-CWE-07
Feature: Contract Negotiation
  Contract Managers and Contract Reviewers negotiate contract terms through
  commenting, version tracking, and structured negotiation workflows with
  redline proposals and full audit logs.

  @clean_db
  Scenario: Open draft contract for negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    When I open contract "Service Agreement" for negotiation
    Then the negotiation interface is displayed
    And I can view all contract clauses

  @clean_db
  Scenario: Add comment to contract clause
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Clarify payment terms" to clause "Payment Terms"
    Then the comment is added to the negotiation log
    And the comment is attributed to my identity
    And the comment includes a timestamp

  @clean_db
  Scenario: Propose redline edit to contract clause
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I propose a redline edit to clause "Liability"
    Then the proposed change is tracked
    And the original text is preserved
    And the redline proposal is visible to other negotiators

  @clean_db
  Scenario: Track version history during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has multiple negotiation edits
    When I view version history for contract "Service Agreement"
    Then I see all versions with timestamps
    And I see user attribution for each version
    And old versions remain accessible

  @clean_db
  Scenario: Approve proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I approve the redline proposal
    Then the change is applied to the contract
    And the approval is logged in the negotiation log
    And a new version is created

  @clean_db
  Scenario: Reject proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I reject the redline proposal with reason "Not acceptable"
    Then the proposal is marked as rejected
    And the rejection reason is logged
    And the original text is retained

  @clean_db
  Scenario: View negotiation log
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has completed multiple negotiation rounds
    When I view the negotiation log for contract "Service Agreement"
    Then I see all comments and proposals
    And I see approvals and rejections
    And I see the full audit trail

  @clean_db
  Scenario: Submit contract for review after negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" negotiation is complete
    When I submit contract "Service Agreement" for review
    Then the contract is routed to assigned reviewers
    And the contract status changes to "Under Review"
    And the submission is logged

  Scenario: Unauthorized role cannot negotiate contracts
    Given I am authenticated with roles: "Contract Observer"
    And contract "Service Agreement" is in "Draft" status
    When I attempt to add a comment to contract "Service Agreement"
    Then the request is denied with an authorization error

  # Rewritten to the DID-party semantics this backend actually implements
  # (see backend/internal/contractworkflowengine/db/contractrepository.go
  # Responsible{Reviewers,Negotiators,Approvers []string}, all peer DIDs, and
  # IsValidNegotiator in acceptnegotiation.go/negotiate.go/
  # rejectnegotiation.go): "party to the contract" = "this instance's own
  # peer DID is among the contract's registered negotiator DIDs" — there is
  # no organization/representative-of-party concept in the data model, only
  # DID membership. FR-CWE-18's intent (only parties may negotiate) is
  # preserved; "Acme Corp"/"TechVendor Inc" party-naming is not.
  @clean_db
  Scenario: Only parties to contract can negotiate terms
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Looks good" to clause "Liability"
    Then the comment is added to the negotiation log
    And the comment is attributed to my identity

  @clean_db
  Scenario: Contract Creator and assigned Reviewers can negotiate
    Given I am authenticated with roles: "Contract Creator"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Looks good" to clause "Liability"
    Then the comment is added to the negotiation log
    And negotiation actions are logged with reviewer identity

  @clean_db
  Scenario: Reviewer cannot approve own redline proposals
    Given I am authenticated with roles: "Contract Reviewer"
    And I have proposed a redline edit to clause "Liability"
    When I attempt to approve my own redline proposal
    Then the request is denied with a "Conflict of interest - cannot approve own proposal" error
    And another authorized reviewer must approve
    And the restriction is logged

  # SRS §3.1.1 Contract Negotiation UI lists "Save draft" among its controls,
  # distinct from "Propose change": a negotiator stages modifications privately
  # and proposes them later. A draft creates no negotiation change-request row,
  # moves no contract state, and is consumed when its author proposes it
  # (command/negotiate.go clears the author's draft row).
  @DCS-IR-CWE-03 @clean_db
  Scenario: A negotiator saves a private draft and proposes it later
    Given I am authenticated with roles: "Contract Creator"
    And contract "Staged Draft Contract" has reached contract state "NEGOTIATION"
    When the negotiator saves a negotiation draft for contract "Staged Draft Contract" renaming it to "Staged Rename"
    Then get http 200:Success code
    And the negotiation draft for contract "Staged Draft Contract" contains the staged name "Staged Rename"
    And the contract "Staged Draft Contract" has no recorded negotiation change requests
    And the contract "Staged Draft Contract" is in state "NEGOTIATION"
    When the negotiator proposes the staged draft for contract "Staged Draft Contract"
    Then get http 200:Success code
    And the negotiation draft for contract "Staged Draft Contract" is empty
    And the contract "Staged Draft Contract" has a recorded negotiation change request renaming it to "Staged Rename"

  @DCS-IR-CWE-03 @clean_db
  Scenario: A negotiation draft is private to its author and can be discarded
    Given I am authenticated with roles: "Contract Creator"
    And contract "Private Draft Contract" has reached contract state "NEGOTIATION"
    When the negotiator saves a negotiation draft for contract "Private Draft Contract" renaming it to "Only Mine"
    Then get http 200:Success code
    And the negotiation draft for contract "Private Draft Contract" is not visible to a user with roles "Contract Reviewer"
    When the negotiator discards the negotiation draft for contract "Private Draft Contract"
    Then get http 200:Success code
    And the negotiation draft for contract "Private Draft Contract" is empty
    And the contract "Private Draft Contract" has no recorded negotiation change requests
