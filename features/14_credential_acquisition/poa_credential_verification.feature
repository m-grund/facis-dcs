# Identity and PoA Credential Acquisition (UC-14). PID credential acquisition,
# verification, and binding to the signing session IS implemented and
# exercised end-to-end by 22_real_signing_vertical/real_signing_vertical.
# feature (EUDIPLO ceremony, SD-JWT VC + KB-JWT presentation, embedded under
# the PAdES signature) — not duplicated here.
#
# What remains genuinely unimplemented is Power-of-Attorney (PoA) credential
# chain verification (FR-SM-03/04: presenting and validating a PoA credential
# proving delegated signing authority, on top of the PID identity credential).
# docs/anforderung.md B3/deviation-register item 7a records this explicitly:
# "PoA presentation only if the demo wallet supports a PoA credential ...
# full chain-walk to trust anchors = roadmap", citing SRS TBD-B (XFSC PCM
# wallet availability, itself Open per the SRS). There is no PoA credential
# type, DCQL query, or chain-walk verifier anywhere in backend/ or
# testWallet/ (grep -ri "poa\|power.of.attorney" backend/internal
# testWallet returns nothing outside this comment).

@UC-14 @DCS-FR-SM-03 @DCS-FR-SM-04
Feature: Power-of-Attorney credential verification at signing

  # @skip: PoA credential acquisition/chain-walk verification is not
  # implemented (deviation-register item 7a, docs/anforderung.md E3) — no PoA
  # credential type, DCQL query, or trust-anchor chain-walk exists in the
  # codebase. PID-only identity binding is implemented and covered by
  # 22_real_signing_vertical instead.
  @skip @UC-14-01 @DCS-FR-SM-03
  Scenario: Signer presents a PoA credential proving delegated signing authority
    Given I hold a PoA credential compliant with eIDAS framework for organization "BDD Org"
    When I present the PoA credential during the signing ceremony
    Then the PoA credential chain is verified to a trusted root
    And the signature is bound to the delegated authority

  # @skip: same reason as above — a revoked PoA credential cannot be
  # distinguished from "no PoA credential" because no PoA verification path
  # exists at all.
  @skip @UC-14-01 @DCS-FR-SM-04
  Scenario: A revoked PoA credential blocks signing
    Given I hold a revoked PoA credential for organization "BDD Org"
    When I attempt to initiate signing for contract "PoA Revoked Contract"
    Then the request is denied with a client error
