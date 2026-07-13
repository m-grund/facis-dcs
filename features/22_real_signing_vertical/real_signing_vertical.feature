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
#   a) RESOLVED - AC3's PAdES-B-B fallback path IS now driven by this
#      harness, though NOT the originally-envisioned way. The original
#      rationale conflated two different problems: pki-consolidation-pkcs11's
#      AC1 genuinely needs the backend's own STARTUP config broken (its
#      HSM/PKCS#11 open() is fatal on bad config, and there is exactly one
#      running backend instance this harness cannot restart mid-suite) -
#      that one really is unreachable here. AC3's TSA, by contrast, is
#      reached over HTTP at RUNTIME. The PAdES timestamp specifically is
#      fetched by PDF-CORE via its own DCS_PDF_CORE_TSA_URL env; repointing
#      that at an unreachable address and rolling instance A's pdf-core takes
#      the TSA away from the PAdES path only, triggering pdf-core's B-B
#      fallback, while leaving ORCE up. Investigation finding that RULED OUT
#      the simpler "scale all of ORCE to 0": /signature/apply ALSO calls
#      ORCE's archive-notary (apply.go), which HARD-FAILS with a 500 when
#      ORCE is down and has no fallback - so scaling all of ORCE aborts apply
#      before any B-B PDF is persisted. That archive-notary asymmetry (PAdES
#      timestamping degrades gracefully; archive notarization does not) is a
#      real open point for the architect. pdf-core is per-release so this
#      only affects instance A, but it MUST still run under the suite-wide
#      flock for its entire duration; see steps/real_signing_vertical/
#      dcs_real_signing_vertical_orce_steps.py's module docstring.
#   b) RESOLVED - AC16 now uses the IPFS CID-swap seam (steps/support/
#      tamper_seam.py) established by contract_format_review's "Tampered
#      PDF fails hash verification". The observable signal is
#      /signature/validate's embedded-PID cross-check finding, not a
#      literal "PAdES signature invalid" verdict - see
#      steps/real_signing_vertical/dcs_real_signing_vertical_tamper_steps.py's
#      module docstring for why no stronger, honest claim is provable from
#      this black-box harness (no reachable endpoint cryptographically
#      re-verifies the CMS signature over its /ByteRange, and pdf-core's own
#      /verify treats the whole PAdES-signed span as an opaque, unchecked
#      suffix by design). c2pa-conformance's AC4 needs this SAME seam PLUS a
#      genuinely separate, not-yet-implemented backend feature (a real
#      remote-manifest fallback in the verify path) and stays @skip for that
#      reason - see that feature file's header comment for what was found.
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

  # Made REAL: design-gap (a) assumed the only way to take the TSA away was
  # restarting the backend with a broken TSA_URL, which this harness indeed
  # cannot do. The TSA the PAdES timestamp uses is, however, reached by
  # PDF-CORE over HTTP at runtime via pdf-core's own DCS_PDF_CORE_TSA_URL env
  # (deployment/helm/templates/pdf-core-deployment.yaml) — repointing THAT at
  # an unreachable address and rolling instance A's pdf-core takes the TSA
  # away from the PAdES path, which is the actual runtime condition PAdES-B-B
  # fallback (pdf-core/compiler/pades.go) exists for. This is DIFFERENT from
  # pki-consolidation-pkcs11's AC1 (that one genuinely needs a broken HSM
  # config at backend STARTUP, unreachable here).
  #
  # NOTE (investigation finding): scaling the whole shared ORCE deployment to
  # 0 — the originally-envisioned mechanism — does NOT work, because
  # /signature/apply has a SECOND, independent ORCE dependency (the archive
  # notary, backend/internal/signingmanagement/command/apply.go ->
  # http://dcs-orce:1880/archive/notary) that HARD-FAILS the whole apply with
  # a 500 when ORCE is down, aborting before any PAdES-B-B PDF is persisted.
  # Only the PAdES timestamp path has a fallback; the archive-notary path
  # does not (a real open point for the architect — see the steps module
  # docstring). Repointing pdf-core's TSA env isolates the PAdES path while
  # leaving ORCE (and the archive notary) up, so apply completes and yields a
  # genuine, inspectable B-B PDF.
  #
  # pdf-core is PER-RELEASE, so this only affects instance A, never instance
  # B or the shared ORCE — but it MUST still run under the suite-wide flock
  # for its entire duration, since any other agent signing via instance A
  # during the TSA-down window would unexpectedly get a B-B signature. See
  # steps/real_signing_vertical/dcs_real_signing_vertical_orce_steps.py.
  @REQ-real-signing-vertical-AC3 @DCS-OR-C2PA-002
  Scenario: PAdES-B-B fallback when the TSA is unavailable, and recovery to PAdES-B-T once it returns
    Given I am authenticated with roles: "Contract Manager"
    When pdf-core's RFC3161 TSA endpoint is made unavailable for this scenario
    And contract "RSV TSA Down Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTsaDown", signed while the TSA is unavailable
    Then the signed PDF for contract "RSV TSA Down Contract" carries no RFC3161 timestamp token
    And pdf-core logged a PAdES-B-B fallback WARN
    When pdf-core's RFC3161 TSA endpoint is restored
    And contract "RSV TSA Recovered Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTsaRecovered", signed after the TSA is restored
    Then the signed PDF for contract "RSV TSA Recovered Contract" embeds an RFC3161 timestamp token

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

  # Made REAL via the IPFS CID-swap seam (steps/support/tamper_seam.py) —
  # the same one that unblocked contract_format_review's "Tampered PDF
  # fails hash verification". See steps/real_signing_vertical/
  # dcs_real_signing_vertical_tamper_steps.py's module docstring for why
  # the observable signal here is /signature/validate's embedded-PID
  # cross-check finding, not a literal PAdES cryptographic signature
  # verdict — no endpoint reachable by this harness re-verifies the CMS
  # signature over its /ByteRange, and pdf-core's own /verify treats the
  # entire PAdES-signed span (including the evidence attachment) as an
  # opaque, unchecked suffix by design. c2pa-conformance's AC4 needs the
  # SAME seam plus a genuinely separate, not-yet-implemented backend
  # feature (remote-manifest fallback) and stays @skip for that reason —
  # see that feature file's header comment.
  @REQ-real-signing-vertical-AC16 @DCS-FR-SM-08
  Scenario: Corrupting the signature-evidence attachment invalidates the embedded-PID cross-check
    Given contract "RSV Evidence Tamper Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerEvidenceTamper"
    When the signature-evidence attachment for contract "RSV Evidence Tamper Contract" is corrupted on the server-stored PDF
    Then the signature validation findings for contract "RSV Evidence Tamper Contract" report the embedded signing evidence as invalid

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
