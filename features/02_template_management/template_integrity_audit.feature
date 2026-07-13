# Template integrity verification and audit log (DCS-FR-TR-20, DCS-FR-TR-21,
# DCS-FR-TR-05): POST /template/verify and GET /template/audit
# (backend/design/template_repository.go). Verify itself is already exercised
# as a setup step elsewhere (template_workflow.feature's "template is
# verified" Given), but no scenario asserts the actual claim of DCS-FR-TR-20
# ("integrity confirmed") or that the verify action lands in the template's
# own audit trail (DCS-FR-TR-21/DCS-FR-TR-05) - this file adds both.

@DCS-FR-TR-20 @DCS-FR-TR-21 @DCS-FR-TR-05 @UC-02
Feature: Template integrity verification and audit log

  @clean_db @DCS-FR-TR-20
  Scenario: Verifying an approved template confirms its integrity
    Given I am authenticated with roles: "Template Manager"
    And template "Integrity Verify Template" is in "Approved" status
    When I verify template "Integrity Verify Template"
    Then get http 200:Success code
    And the template verification reports no findings

  @clean_db @DCS-FR-TR-21 @DCS-FR-TR-05
  Scenario: Template audit log records a verify action
    Given I am authenticated with roles: "Template Manager"
    And template "Audit Log Template" is in "Approved" status
    When I verify template "Audit Log Template"
    Then get http 200:Success code
    And the template audit log for "Audit Log Template" includes an action of type "VERIFY_CONTRACT_TEMPLATE"
