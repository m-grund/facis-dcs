# Signature validate/audit/compliance (DCS-FR-SM-18, DCS-FR-SM-19,
# DCS-FR-SM-21, UC-04): POST /signature/validate, GET /signature/audit,
# POST /signature/compliance (backend/design/signature_management.go). All
# three are already implemented; only /signature/verify (contract integrity &
# envelope check, used as a setup step by other packs) and /signature/apply
# (the signing ceremony itself, covered by 22_real_signing_vertical) had
# scenario coverage before this file.

@UC-04 @DCS-FR-SM-18 @DCS-FR-SM-19 @DCS-FR-SM-21
Feature: Signature validation, audit, and compliance

  @clean_db @DCS-FR-SM-18
  Scenario: Contract Manager validates a signed contract's signature
    Given contract "Signature Validation Contract" has reached contract state "SIGNED"
    When the contract manager validates the signature for contract "Signature Validation Contract"
    Then get http 200:Success code
    And the signature validation for contract "Signature Validation Contract" reports no findings

  @clean_db @DCS-FR-SM-19
  Scenario: Signature audit log records the apply-signature action
    Given contract "Signature Audit Contract" has reached contract state "SIGNED"
    When the contract manager validates the signature for contract "Signature Audit Contract"
    Then get http 200:Success code
    And the signature audit log for contract "Signature Audit Contract" includes an action of type "APPLY_SIGNATURE"

  @clean_db @DCS-FR-SM-21
  Scenario: Contract Manager requests a compliance check for a signed contract
    Given contract "Signature Compliance Contract" has reached contract state "SIGNED"
    When the contract manager requests a compliance check for contract "Signature Compliance Contract"
    Then get http 200:Success code
    And the compliance check for contract "Signature Compliance Contract" returns no findings
