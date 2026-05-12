@UC-02-10
@skip
Feature: Template Approval Workflow
  Templates progress through submission, review, and approval
  before becoming available for contract generation.

  Scenario: Submit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Draft" status
    When I submit template "Standard NDA" for review
    Then the template status is "Submitted"
    And review and approval tasks are created

  Scenario: Review template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    When I submit template "Standard NDA" with flag "Approval"
    And all other review tasks are not in "Open, Verified" states
    Then the template status is "Reviewed"

  Scenario: Approve reviewed template
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I approve template "Standard NDA"
    Then the template status is "Approved"
    And the template is available for contract generation

  Scenario: Reject template with reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" with reason "Missing compliance clause"
    Then the template status is "Rejected"
    And my approval task is in "Rejected" status
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

