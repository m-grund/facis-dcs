@UC-02-04
Feature: Update Contract Template
  Template Creators update existing templates
  with full version history preserved.

  Background:
    Given I am authenticated with roles: "Template Creator"

  Scenario: Update an existing template
    Given template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA"
    Then a new version "1.1" is created
    And the previous version remains accessible

  Scenario: Unauthorized role cannot update template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" with version "1.0" exists
    When I update template "Standard NDA"
    Then the request is denied with an authorization error
