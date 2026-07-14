# Access Rights Revocation (UC-15-01). The revoke action itself
# (POST /signature/revoke) and its ContractSigner-level effects are exercised
# extensively by 22_real_signing_vertical/real_signing_vertical.feature; this
# file covers the piece that pack does not assert: that revoking a signature
# transitions the CONTRACT's own lifecycle state to REVOKED
# (backend/internal/signingmanagement/command/revoke.go,
# contractstate.EventRevoke) and that re-signing is required to restore it —
# the observable "rights withdrawn until re-signing" behavior Table 7 names.

@UC-15-01 @DCS-FR-SM-20
Feature: Signature revocation transitions the contract to REVOKED

  @UC-15-01
  Scenario: Revoking a contract's signature moves the contract to REVOKED
    Given contract "Revocation State Contract" has reached contract state "REVOKED"
    Then the contract "Revocation State Contract" is in state "REVOKED"

  @UC-15-01 @DCS-FR-CWE-06
  Scenario: A revoked contract can be re-approved to allow re-signing
    Given contract "Revocation Restore Contract" has reached contract state "REVOKED"
    And I am authenticated with roles: "Contract Approver"
    When I approve contract "Revocation Restore Contract"
    Then get http 200:Success code
    And the contract "Revocation Restore Contract" is in state "APPROVED"
