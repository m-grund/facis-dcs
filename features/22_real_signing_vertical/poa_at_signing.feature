# Power of Attorney at signing (UC-14, DCS-FR-SM-03/-04/-26): a natural person
# signs on behalf of an organization, presenting a Power of Attorney at the
# signing ceremony that authorizes them to act for the party — the participating
# DCS instance DID — they sign as. A missing or wrong-party PoA blocks signing
# (UC-14). The Signature Compliance Viewer re-checks every party node in the
# (possibly peer-synced) contract and raises an audit finding for any party, own
# or a counterparty's, that signed without a Power of Attorney for that party.

@UC-04 @DCS-FR-SM-03 @DCS-FR-SM-04 @DCS-FR-SM-26 @ADR-12
Feature: Power of Attorney at signing

  @DCS-FR-SM-03 @UC-14
  Scenario: A signatory with a Power of Attorney for their party signs and is compliant
    Given contract "PoA Happy Contract" is APPROVED and has completed a signing ceremony for signatory "SignerPoaHappy"
    When the signer publishes the OID4VP signing request for contract "PoA Happy Contract"
    Then get http 200:Success code
    When the wallet signs contract "PoA Happy Contract" by consuming the OID4VP signing request as "SignerPoaHappy"
    Then the contract "PoA Happy Contract" has completed signing
    And the signature compliance for contract "PoA Happy Contract" raises no Power of Attorney finding

  @UC-14
  Scenario: Signing is blocked when no Power of Attorney is presented at the ceremony
    Given contract "PoA Missing Contract" has reached contract state "APPROVED"
    When a signing ceremony is started for the signing party of contract "PoA Missing Contract"
    And the ceremony webhook is completed with no Power of Attorney
    Then the signing request is rejected because the Power of Attorney does not authorize this signature

  @UC-14
  Scenario: Signing is blocked when the Power of Attorney authorizes a different party
    Given contract "PoA Wrong Contract" has reached contract state "APPROVED"
    When a signing ceremony is started for the signing party of contract "PoA Wrong Contract"
    And the ceremony webhook is completed with a Power of Attorney for a different party
    Then the signing request is rejected because the Power of Attorney does not authorize this signature

  @DCS-FR-SM-04 @DCS-FR-SM-26
  Scenario: A counterparty that signed without a valid Power of Attorney is raised in audit
    Given contract "PoA Counterparty Contract" is APPROVED and has completed a signing ceremony for signatory "SignerPoaCp"
    When the signer publishes the OID4VP signing request for contract "PoA Counterparty Contract"
    Then get http 200:Success code
    When the wallet signs contract "PoA Counterparty Contract" by consuming the OID4VP signing request as "SignerPoaCp"
    Then the contract "PoA Counterparty Contract" has completed signing
    When the counterparty Power of Attorney on contract "PoA Counterparty Contract" is tampered to authorize a different organization
    Then the signature compliance for contract "PoA Counterparty Contract" raises a Power of Attorney finding
    And an audit event records the Power of Attorney finding for contract "PoA Counterparty Contract"
