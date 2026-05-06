@UC-02-05 @UC-02-11
@skip
Feature: Archive Contract Templates
  Template Managers deprecate outdated templates to prevent new contract generation
  and delete deprecated templates that are no longer needed.

  # UC-02-05: Deprecate Contract Template

  Scenario: Deprecate an active template
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Approved" status
    When I deprecate template "Old NDA"
    Then the template status is "Deprecated"
    And new contracts cannot be generated from this template

  Scenario: Unauthorized role cannot deprecate template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Approved" status
    When I attempt to deprecate template "Old NDA"
    Then the request is denied with an authorization error

  # UC-02-11: Delete Contract Template

  Scenario: Delete reviewed template
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Reviewed" status
    When I delete template "Old NDA"
    Then the template status is "Deleted"
    And the archivation is recorded in the audit log
 
  Scenario: Cannot delete deprecated template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Deprecated" status
    When I attempt to archive template "Standard NDA"
    Then the request is denied
    And I receive error "invalid contract template state"
 
  Scenario: Unauthorized role cannot archive template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Submitted" status
    When I attempt to delete template "Old NDA"
    Then the request is denied with an authorization error