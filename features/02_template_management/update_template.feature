@UC-02-04
Feature: Update Contract Template
  Template Creators update existing templates
  with full version history preserved.

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Creator"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" name to "Test Name"
    Then the result is a template with name "Test Name"

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Creator"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" description to "Test Description"
    Then the result is a template with description "Test Description"

  @clean_db
  Scenario: Update an existing template as reviewer
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status with name "Standard NDA" and description "Template Description"
    When I update template "Standard NDA" name to "Standard NDA"
    Then the result is a template with name "Standard NDA"

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status with name "Standard NDA" and description "Template Description"
    When I update template "Standard NDA" description to "Test Description"
    Then the result is a template with description "Test Description"

  @clean_db
  Scenario: Unauthorized role cannot update template
    Given I am authenticated with roles: "Template Approver"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" description to "Test Description"
    Then the request is denied with an authorization error
