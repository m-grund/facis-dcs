@UC-02-03
Feature: Generate Contract from Template
  Contract Creators generate contract instances
  by populating approved templates with data.
  
  # contract/create only accepts templates in state REGISTERED or PUBLISHED
  # (backend ReadContractTemplateDataByID) — APPROVED alone is not enough.
  Scenario: Generate contract from approved template
    Given I am authenticated with roles: "Contract Creator"
    And template "Standard NDA" is in "Registered" status
    When I generate a contract from template "Standard NDA"
    Then a contract is created

  Scenario: Unauthorized role cannot generate contract
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Registered" status
    When I generate a contract from template "Standard NDA"
    Then the request is denied with an authorization error
