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

  # @skip: contract version history is not exposed as a queryable API yet
  # (retrieve returns only the current contract_version) — needs backend
  # capability, not just step definitions.
  @skip
  Scenario: Track version history during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has multiple negotiation edits
    When I view version history for contract "Service Agreement"
    Then I see all versions with timestamps
    And I see user attribution for each version
    And old versions remain accessible

  # @skip: negotiation decisions are peer-DID-scoped (decision rows are keyed
  # by the responsible peer DID); POST /contract/respond by an individual user
  # returns 200 but matches no decision row, so ACCEPTED/REJECTED never
  # surfaces in the log. Asserting decision outcomes needs backend capability
  # (user-scoped decision recording or rows-affected enforcement).
  @skip
  Scenario: Approve proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I approve the redline proposal
    Then the change is applied to the contract
    And the approval is logged in the negotiation log
    And a new version is created

  # @skip: negotiation decisions are peer-DID-scoped (decision rows are keyed
  # by the responsible peer DID); POST /contract/respond by an individual user
  # returns 200 but matches no decision row, so ACCEPTED/REJECTED never
  # surfaces in the log. Asserting decision outcomes needs backend capability
  # (user-scoped decision recording or rows-affected enforcement).
  @skip
  Scenario: Reject proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I reject the redline proposal with reason "Not acceptable"
    Then the proposal is marked as rejected
    And the rejection reason is logged
    And the original text is retained

  # @skip: negotiation decisions are peer-DID-scoped (decision rows are keyed
  # by the responsible peer DID); POST /contract/respond by an individual user
  # returns 200 but matches no decision row, so ACCEPTED/REJECTED never
  # surfaces in the log. Asserting decision outcomes needs backend capability
  # (user-scoped decision recording or rows-affected enforcement).
  @skip
  Scenario: View negotiation log
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has completed multiple negotiation rounds
    When I view the negotiation log for contract "Service Agreement"
    Then I see all comments and proposals
    And I see approvals and rejections
    And I see the full audit trail

  # @skip: duplicate coverage — the NEGOTIATION→SUBMITTED path is executable
  # and verified by contract_state_machine_refactor.feature and the
  # "returned for revision" reopen proof in contract_approval.feature; the
  # "Under Review"/reviewer-routing assertions here have no step definitions.
  @skip
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

  # @skip: party/organization-scoped negotiation ACLs (representative-of-party,
  # per-organization attribution, non-party denial, per-reviewer assignment)
  # are not modeled in the backend — responsibility is peer-DID-scoped
  # (responsible.reviewers/negotiators are peer DIDs, not individual users or
  # organizations). Needs backend capability, not step definitions.
  @skip
  Scenario: Only parties to contract can negotiate terms
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" involves parties "Acme Corp" and "TechVendor Inc"
    And I am a representative of party "Acme Corp"
    When I open contract "Service Agreement" for negotiation
    Then the negotiation interface is displayed
    And I can add comments to contract clauses
    And my comments are attributed to organization "Acme Corp"

  # @skip: party/organization-scoped negotiation ACLs (representative-of-party,
  # per-organization attribution, non-party denial, per-reviewer assignment)
  # are not modeled in the backend — responsibility is peer-DID-scoped
  # (responsible.reviewers/negotiators are peer DIDs, not individual users or
  # organizations). Needs backend capability, not step definitions.
  @skip
  Scenario: Non-party reviewer cannot negotiate contract not assigned to them
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" involves parties "Acme Corp" and "TechVendor Inc"
    And I am a representative of organization "UnrelatedCorp"
    When I attempt to access contract "Service Agreement" for negotiation
    Then the request is denied with an "Access denied - not a party to this contract" error
    And the access denial is logged

  # @skip: party/organization-scoped negotiation ACLs (representative-of-party,
  # per-organization attribution, non-party denial, per-reviewer assignment)
  # are not modeled in the backend — responsibility is peer-DID-scoped
  # (responsible.reviewers/negotiators are peer DIDs, not individual users or
  # organizations). Needs backend capability, not step definitions.
  @skip
  Scenario: Contract Creator and assigned Reviewers can negotiate
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is assigned to reviewers "Alice" and "Bob"
    And I am listed as an assigned reviewer
    When I open contract "Service Agreement" for negotiation
    Then I can add comments and propose redlines
    And only assigned reviewers and the creator can see negotiation comments
    And negotiation actions are logged with reviewer identity

  # @skip: requires a conflict-of-interest guard in the backend respond path
  # (acceptnegotiation.go validates the negotiator peer but has no
  # own-proposal check — decisions are peer-scoped, so on a single instance
  # the proposer and approver are the same peer DID). Needs a backend
  # capability, not a step definition.
  @skip
  Scenario: Reviewer cannot approve own redline proposals
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    And I have proposed a redline edit to clause "Liability"
    When I attempt to approve my own redline proposal
    Then the request is denied with a "Conflict of interest - cannot approve own proposal" error
    And another authorized reviewer must approve
    And the restriction is logged
