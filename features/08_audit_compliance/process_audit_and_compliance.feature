# Process Audit & Compliance Management (UC-08,
# backend/design/process_audit_and_compliance.go, /pac/...). C2PA provenance
# export is covered separately by c2pa_provenance_export.feature. This file
# covers the four /pac/* endpoints themselves.

@UC-08 @DCS-IR-PACM-01
Feature: Process audit and compliance management

  @UC-08-02 @DCS-IR-PACM-01
  Scenario: Auditor triggers an audit on the CONTRACT_WORKFLOW_ENGINE scope and sees the create event
    Given contract "PAC Audit Contract" is in "Draft" status
    When the Auditor triggers a process audit with scope "CONTRACT_WORKFLOW_ENGINE"
    Then get http 200:Success code
    And the process audit response includes an audit trail entry for contract "PAC Audit Contract"

  @UC-08-02 @DCS-IR-PACM-01
  Scenario: A role outside Auditor/Compliance Officer cannot trigger a process audit
    Given I am authenticated with roles: "Template Creator"
    When I attempt to trigger a process audit with scope "CONTRACT_WORKFLOW_ENGINE"
    Then the request is denied with a client error

  @UC-08-01 @DCS-IR-PACM-02
  Scenario: Auditor generates an audit report
    Given contract "PAC Report Contract" is in "Draft" status
    When the Auditor requests an audit report for scope "CONTRACT_WORKFLOW_ENGINE" in format "json"
    Then get http 200:Success code

  @UC-08-02 @DCS-IR-PACM-03
  Scenario: Compliance Officer runs continuous monitoring
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    # The sweep event itself carries no resource DID and is anchored only to
    # the global chain, not the per-component PAC read path — the auditable
    # per-contract artifact (PAC_COMPLIANCE_RISK) is asserted by the
    # "Compliance monitoring detects risk during approval" scenario in
    # 03_contract_creation/contract_approval.feature.
    And the monitoring response reports a checked_at timestamp and a risks list

  @UC-08-02 @DCS-IR-PACM-04
  Scenario: Compliance Officer submits a non-compliance incident report
    When the Compliance Officer submits a non-compliance incident report
    Then get http 200:Success code
