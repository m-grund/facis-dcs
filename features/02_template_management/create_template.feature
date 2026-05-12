@UC-02-01
Feature: Create Contract Template
  Template Creators create reusable contract templates
  that serve as the basis for contract generation.

  Background:
    Given I am authenticated with roles: "Template Creator"

  Scenario: Create a new contract template
    When I create a template "Standard NDA" in category "Legal"
    Then the template is created in "Draft" status
    And the template is assigned version "1.0"

  Scenario: Unauthorized role cannot create template
    Given I am authenticated with roles: "Template Reviewer"
    When I create a template "Standard NDA" in category "Legal"
    Then the request is denied with an authorization error
