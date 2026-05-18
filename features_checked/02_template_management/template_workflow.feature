@UC-02-10
Feature: Template Approval Workflow
  Templates progress through submission, review, and approval
  before becoming available for contract generation.

  @clean_db
  Scenario: Submit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Draft" status
    When I submit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db
  Scenario: Approve submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with flag=approval
    Then the template status is "Reviewed"

  @clean_db
  Scenario: Reject submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with flag=draft
    Then the template status is "Rejected"

  @clean_db
  Scenario: Reject reviewed template without reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" without reason
    And I retrieve template "Standard NDA" by did
    Then the template status is "Reviewed"

  @clean_db
  Scenario: Reject reviewed template with reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" with reason "Missing compliance clause"
    Then the template status is "Rejected"
    And the rejection reason is recorded

  @clean_db
  Scenario: Resubmit reviewed template
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I resubmit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db
  Scenario: Resubmit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Rejected" status
    When I submit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db
  Scenario: Unauthorized role cannot approve template
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Reviewed" status
    When I approve template "Standard NDA"
    Then the request is denied with an authorization error

