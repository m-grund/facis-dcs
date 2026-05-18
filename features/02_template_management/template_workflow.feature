@UC-02-10
Feature: Template Approval Workflow
  Templates progress through submission, review, and approval
  before becoming available for contract generation.

  Scenario: Submit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Draft" status
    When I submit template "Standard NDA"
    Then the template status is "Submitted"

  Scenario: Approve submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with approval flag
    Then the template status is "Reviewed"

  Scenario: Reject submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with draft flag
    Then the template status is "Rejected"

  Scenario: Reject reviewed template without reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" without reason
    Then the template status is "Reviewed"

  Scenario: Reject reviewed template with reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" with reason "Missing compliance clause"
    Then the template status is "Rejected"
    And the rejection reason is recorded

  Scenario: Resubmit reviewed template
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I resubmit template "Standard NDA"
    Then the template status is "Submitted"
    And all tasks are in "Open" status

  Scenario: Submit to Draft template with comment
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    When I submit template "Standard NDA" with flag "Draft" and comment "Missing compliance clause"
    Then the template status is "Rejected"
    And all tasks are in "Open" status
    And the comment is recorded

  Scenario: Resubmit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Rejected" status
    When I submit template "Standard NDA" for review
    Then the template status is "Submitted"
    And all tasks are in "Open" status

  Scenario: Unauthorized role cannot approve template
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Reviewed" status
    When I approve template "Standard NDA"
    Then the request is denied with an authorization error

