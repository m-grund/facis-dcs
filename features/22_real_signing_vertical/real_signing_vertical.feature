# Requirement: real-signing-vertical
#
# Covers Workstream B ("Real signing vertical: PAdES + EUDIPLO ceremony + PID
# binding", docs/anforderung.md Zeilen 145-199) - only the ACs the analyst
# marked Pruefmittel = BDD: AC1-AC6, AC8-AC17, AC19.
#
# Deliberately OUT of scope for this pack:
#   - AC7 (Pruefmittel = grep-gate: the "dss" -> "signer"/ContractSigner
#     rename + STUB_SIGNATURE_PLACEHOLDER/credential_type:'stub' removal -
#     checked by the verifier via grep, not a Gherkin scenario).
#   - AC18 (Pruefmittel = extern-validiert - the Adobe/DSS-demo-webapp manual
#     validation step in B-acceptance).
#
# See steps/real_signing_vertical/dcs_real_signing_vertical_steps.py's module
# docstring for the full rationale behind every binding decision and design
# gap summarized below:
#
#   1. pdf-core's own POST /sign is not reachable from this harness at all
#      (only the backend is, via BDD_DCS_BASE_URL) - AC1-AC5/AC9/AC14/AC15/
#      AC17/AC19 exercise PAdES indirectly through POST /signature/apply
#      (extended per B2) and inspect the PDF bytes GET /pdf/export/contract/
#      {did} serves afterwards, using the same direct-byte-search technique
#      already established elsewhere in this codebase's BDD packs.
#   2. POST /signature/request, GET /signature/request/{id}, and POST
#      /signature/request/webhook do not exist in backend/design/*.go yet -
#      AC10-AC13 are written against the ASSUMED contract docs/anforderung.md
#      B3 specifies verbatim.
#   3. EUDIPLO is never co-deployed here; this harness plays the "EUDIPLO
#      test client" role itself, POSTing a real, protocol-correct SD-JWT VC +
#      KB-JWT PID presentation straight at the assumed webhook contract
#      (built with the existing testWallet/dcs_wallet signing primitives).
#   4. The webhook shared-secret header name (X-EUDIPLO-Webhook-Secret) is
#      assumed - open point for the architect/implementer.
#   5. Byte-level PDF assertions (SubFilter, x5chain, RFC3161 timestamp,
#      ByteRange coverage) are direct-byte-search heuristics, not a full PDF/
#      CMS/ASN.1 parse - each documents its own precision limit at its point
#      of use in the steps module.
#
# Design gaps (open points for architect/analyst):
#   a) AC3's PAdES-B-B fallback path cannot be driven by this harness (same
#      class of "restart with deliberately broken config" problem as
#      pki-consolidation-pkcs11's AC1 negative path) - documented as an
#      accepted manual/ops verification concern, not a Gherkin scenario.
#   b) AC16 needs the same unavailable "upload a tampered PDF and verify it"
#      seam already identified for c2pa-conformance's AC4 and
#      contract_format_review's "Tampered PDF fails hash verification" - @skip
#      here, following that precedent.
#   c) AC20 (Signature Manager UI: QR/poll/result, AES badge) has no coverage
#      in this pack - this repo-root BDD harness has no browser-automation
#      convention at all (see features/16_other/frontend.feature, a bare
#      reachability check). The SERVICE-LEVEL contract the UI would call is
#      already exercised by AC10-AC13/AC19 below; the UI-specific rendering
#      claims are recorded as an explicit coverage gap, not fabricated.

@DCS-FR-SM-16 @DCS-IR-SI-10
Feature: Real signing vertical - PAdES signature, EUDIPLO ceremony, PID binding (Workstream B)

  # ---------------------------------------------------------------------
  # B1 - pdf-core POST /sign (PAdES), exercised indirectly via
  # POST /signature/apply after a completed ceremony (see steps module
  # docstring point 1).
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC1 @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: Applying a signature produces a PDF with a cryptographically valid PAdES signature in the named AcroForm field
    Given contract "RSV AcroForm Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerOne"
    Then the signed PDF for contract "RSV AcroForm Contract" contains a PAdES signature naming AcroForm field "SignerOne"
    And the signed PDF for contract "RSV AcroForm Contract" has a structurally valid PAdES ByteRange

  @REQ-real-signing-vertical-AC2 @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: The PAdES signature declares SubFilter ETSI.CAdES.detached with a full embedded x5chain
    Given contract "RSV SubFilter Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTwo"
    Then the signed PDF for contract "RSV SubFilter Contract" declares SubFilter ETSI.CAdES.detached
    And the signed PDF for contract "RSV SubFilter Contract" embeds a non-empty X.509 certificate chain

  @REQ-real-signing-vertical-AC3 @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: The PAdES signature carries an RFC3161 timestamp from the configured TSA (PAdES-B-T)
    Given contract "RSV Timestamp Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerThree"
    Then the signed PDF for contract "RSV Timestamp Contract" embeds an RFC3161 timestamp token

  @REQ-real-signing-vertical-AC3 @DCS-OR-C2PA-002 @skip
  Scenario: PAdES-B-B fallback when the TSA is unavailable is a documented deviation, not exercised here
    # See design-gap (a) in the header comment above: this would require
    # restarting the single running backend instance with a deliberately
    # broken/missing TSA_URL mid-run, which this harness cannot do (identical
    # class of problem as pki-consolidation-pkcs11's AC1 negative path).
    # Tracked as an accepted manual/ops verification concern.
    Given I am authenticated with roles: "Contract Manager"

  @REQ-real-signing-vertical-AC4 @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: The order /update -> /sign -> /update leaves the PAdES signature and C2PA chain valid
    Given contract "RSV Order Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFour"
    When the signature for contract "RSV Order Contract" is revoked as a post-sign C2PA update
    Then get http 200:Success code
    When I re-export the signed PDF for contract "RSV Order Contract"
    Then the signed PDF for contract "RSV Order Contract" still has a structurally valid PAdES signature
    When contract "RSV Order Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verification result shows match is true

  # ---------------------------------------------------------------------
  # B2 - real contract signer + apply-flow fixes
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC5 @DCS-FR-SM-16 @DCS-IR-SI-10
  Scenario: The applied signature is a real PAdES signature persisted with an IPFS CID, not the stub placeholder
    Given contract "RSV No Stub Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFive"
    Then the contract_signatures row for contract "RSV No Stub Contract" is a real signature, not the STUB placeholder

  @REQ-real-signing-vertical-AC6 @FR-SM-18 @DCS-IR-SI-10
  Scenario: The apply endpoint honors the requested signer_did and credential_type instead of discarding them
    Given contract "RSV Apply Fields Contract" is APPROVED and has completed a signing ceremony for signatory "ExplicitFieldsSigner"
    When contract signer applies a signature to contract "RSV Apply Fields Contract" using the ceremony's signer_did and credential_type "AES"
    Then get http 200:Success code
    And the signature envelope for contract "RSV Apply Fields Contract" reflects the ceremony's signer_did and credential_type "AES"

  @REQ-real-signing-vertical-AC8 @DCS-FR-SM-16 @FR-SM-25 @UC-04-02
  Scenario: Apply is refused with a typed error until a completed PID presentation exists for the signer
    Given contract "RSV Ceremony Gate Contract" has reached contract state "APPROVED"
    When contract signer applies a signature to contract "RSV Ceremony Gate Contract" without a prior signing ceremony
    Then the apply request is rejected with a typed ceremony-required error

  @REQ-real-signing-vertical-AC9 @DCS-FR-CWE-04
  Scenario: The signature record binds both the PDF hash and the JSON-LD content hash
    Given contract "RSV Dual Hash Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerSix"
    Then the contract_signatures row for contract "RSV Dual Hash Contract" records both a PDF hash and a JSON-LD content hash

  # ---------------------------------------------------------------------
  # B3 - EUDIPLO signing ceremony (assumed endpoint contract - see steps
  # module docstring point 2)
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC10 @FR-SM-14
  Scenario: POST /signature/request starts a ceremony for an authorized Contract Signer
    Given contract "RSV Ceremony Start Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Ceremony Start Contract" field "SignerSeven" as "Contract Signer"
    Then get http 200:Success code
    And the ceremony response includes a ceremony_id, wallet_uri, and expires_at

  @REQ-real-signing-vertical-AC10 @FR-SM-14
  Scenario: POST /signature/request denies a caller without an authorized signing role
    Given contract "RSV Ceremony Denied Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Ceremony Denied Contract" field "SignerEight" as "Contract Observer"
    Then the ceremony start request is denied for that role

  @REQ-real-signing-vertical-AC11 @FR-SM-14
  Scenario: GET /signature/request/{id} reports the ceremony's lifecycle status as it progresses
    Given contract "RSV Ceremony Status Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerNine"
    When I poll the signing ceremony status for contract "RSV Ceremony Status Contract"
    Then get http 200:Success code
    And the signing ceremony for contract "RSV Ceremony Status Contract" has status "verified"

  @REQ-real-signing-vertical-AC12 @NFR-SEC-18 @FR-SM-14
  Scenario: The webhook receiver marks the ceremony verified and persists PID claims when the shared secret is correct
    Given contract "RSV Webhook Auth Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Webhook Auth Contract" field "SignerTen" as "Contract Signer"
    Then get http 200:Success code
    When the EUDIPLO webhook confirms the presentation for contract "RSV Webhook Auth Contract" with the correct shared secret
    Then get http 200:Success code
    When I poll the signing ceremony status for contract "RSV Webhook Auth Contract"
    Then the signing ceremony for contract "RSV Webhook Auth Contract" has status "verified"

  @REQ-real-signing-vertical-AC12 @NFR-SEC-18 @FR-SM-14
  Scenario: The webhook receiver rejects a request presenting an incorrect shared secret
    Given contract "RSV Webhook Bad Secret Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Webhook Bad Secret Contract" field "SignerEleven" as "Contract Signer"
    Then get http 200:Success code
    When a caller posts the EUDIPLO webhook for contract "RSV Webhook Bad Secret Contract" with an incorrect shared secret
    Then the webhook request is rejected for the incorrect shared secret

  @REQ-real-signing-vertical-AC13 @UC-04-02
  Scenario: The ceremony completes headlessly by fulfilling the OID4VP presentation/webhook contract, no wallet UI involved
    Given contract "RSV Headless Ceremony Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTwelve"
    When I poll the signing ceremony status for contract "RSV Headless Ceremony Contract"
    Then get http 200:Success code
    And the signing ceremony for contract "RSV Headless Ceremony Contract" has status "verified"

  # ---------------------------------------------------------------------
  # B4 - identity binding: PID fragment + signing-summary VC embedded UNDER
  # the signature (embed-first-sign-second)
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC14 @DCS-FR-SM-08 @NFR-SEC-18
  Scenario: The presented SD-JWT VC + KB-JWT is embedded verbatim before signing, inside the PAdES ByteRange
    Given contract "RSV Verbatim Presentation Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerThirteen"
    Then the SD-JWT VC presentation for contract "RSV Verbatim Presentation Contract" is embedded verbatim inside the PAdES ByteRange

  @REQ-real-signing-vertical-AC15 @DCS-FR-SM-08
  Scenario: A ContractSigningSummaryCredential is issued and embedded under the signature
    Given contract "RSV Summary VC Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFourteen"
    Then a ContractSigningSummaryCredential for contract "RSV Summary VC Contract" is embedded inside the PAdES ByteRange

  @REQ-real-signing-vertical-AC16 @DCS-FR-SM-08 @skip
  Scenario: Removing the signature-evidence attachment invalidates the PAdES validation
    # See design-gap (b) in the header comment above: every verify-shaped
    # endpoint this harness can reach always re-fetches the SERVER'S OWN
    # stored PDF by DID - there is no upload-a-tampered-PDF-and-verify-it
    # endpoint, the identical class of problem already accepted for
    # c2pa-conformance's AC4 (@skip) and contract_format_review's "Tampered
    # PDF fails hash verification" (@skip). Real evidence for this claim is
    # expected from pdf-core's own pyHanko-based BDD harness (per
    # docs/anforderung.md B-acceptance: "write this as an explicit test")
    # or a Go-level unit test mocking IPFSClient.FetchFile.
    Given I am authenticated with roles: "Contract Manager"

  @REQ-real-signing-vertical-AC17 @UC-04-02 @UC-04-03
  Scenario: The verify side re-verifies the embedded PID presentation and cross-checks it against the signature record
    Given contract "RSV Verify Crosscheck Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFifteen"
    When I validate the signature for contract "RSV Verify Crosscheck Contract"
    Then get http 200:Success code
    And the signature validation findings for contract "RSV Verify Crosscheck Contract" cross-check the embedded PID evidence

  # ---------------------------------------------------------------------
  # B-acceptance - full e2e
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC19 @UC-04-02 @UC-04-03 @DCS-FR-SM-16
  Scenario: End-to-end - accept, ceremony, AES-signed PDF, verify stays green, contract_signatures carries AES + ipfs_cid + ceremony link
    Given contract "RSV E2E Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerSixteen"
    When contract "RSV E2E Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verification result shows match is true
    And the contract_signatures row for contract "RSV E2E Contract" is a real signature, not the STUB placeholder
    And the contract_signatures row for contract "RSV E2E Contract" is linked to a signature_ceremonies row

  # ---------------------------------------------------------------------
  # B5 - Signature Manager UI: documented coverage gap (see design-gap (c)
  # in the header comment above). No fabricated pass/fail - the tag/
  # traceability is kept present via @skip.
  # ---------------------------------------------------------------------

  @REQ-real-signing-vertical-AC20 @skip
  Scenario: Signature Manager UI ceremony flow and AES badge - not provable from this HTTP-only BDD harness
    # This repo-root BDD harness has no browser-automation convention (see
    # features/16_other/frontend.feature - a bare reachability check). The
    # service-level contract the UI would call (start ceremony, poll status,
    # apply, AES credential_type) is already exercised end-to-end by AC10-
    # AC13/AC19 above. The UI-specific claims - frontend/ClientApp/src/
    # services/signature-management-service.ts no longer hardcoding
    # credential_type: 'stub', the QR/poll/result modal, and the AES badge
    # render - need a browser-level test this harness does not have; recorded
    # here as an explicit coverage gap for the analyst/architect, not a
    # fabricated result.
    Given I am authenticated with roles: "Contract Manager"
