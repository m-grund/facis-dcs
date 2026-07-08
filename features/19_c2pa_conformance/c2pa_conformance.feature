# Requirement: c2pa-conformance
#
# Covers Workstream D ("C2PA conformance patch", docs/anforderung.md
# Zeilen 270-282) — only the ACs the analyst marked Pruefmittel = BDD:
#
#   AC1 — GET /c2pa/manifest/{contract_did} returns the raw C2PA manifest
#         store bytes (Content-Type: application/c2pa, HTTP 200) for a
#         signed/exported contract, WITHOUT any JWT/auth (public sibling of
#         GET /.well-known/did.json, backend/design/did.go).
#   AC2 — GET /c2pa/manifest/{contract_did}?history=true returns a parsed
#         JSON enumeration of the manifest chain (labels + dcs.lifecycle
#         assertions) instead of the raw store.
#   AC3 — the manifest embedded in a signed PDF references the AC1 remote
#         endpoint via the C2PA claim's `remote_manifests` field.
#   AC5 — for each of the six lifecycle states (Active, Suspended,
#         Terminated, Replaced, Expired, Draft), the verify endpoint reports
#         the matching lifecycle_status.
#   AC6 — the verify response carries four independently named checks (PDF
#         signature, C2PA manifest, VC signature, status list), and the PDF
#         signature check honestly reports "not yet available" rather than
#         faking a pass (Workstream B/PAdES does not exist yet).
#
# Deliberately OUT of scope for this pack:
#   - AC7 (Prüfmittel = grep-gate: a Deviation-Register entry citing
#     C2PA-001's measurement — the dokumentierer/orchestrator adds this,
#     no Gherkin scenario is appropriate for a documentation-existence
#     check).
#
# AC4 (strip-then-verify) is INTENTIONALLY @skip — see the Scenario itself
# for the detailed rationale. Summary: proving the backend's remote-manifest
# fallback honestly requires the SERVER'S OWN stored/cached PDF to be
# missing its `content_credential.c2pa` attachment. There is no HTTP-level
# way to put the server into that state today (verify_contract_pdf always
# re-fetches its own IPFS-cached copy by DID —
# backend/internal/pdfgeneration/query/verifycontract.go:94-103 — there is
# no upload-a-PDF-and-verify-it endpoint). This is the EXACT same class of
# problem the codebase already hit and already resolved the same way: see
# features/03_contract_creation/contract_format_review.feature:84-93
# ("Tampered PDF fails hash verification", @skip, "requires injecting a
# tampered PDF into IPFS ... covered by the Go unit tests"). AC4 follows
# that established precedent rather than inventing a new, unreviewed
# IPFS-write test seam.
#
# --- Design gaps this pack surfaced (open points for architect/analyst) ---
#
# 1. GET /c2pa/manifest/{contract_did} does not exist in backend/design/ at
#    all yet (grep backend/design/*.go — no match). AC1/AC2/AC3 are written
#    against the endpoint CONTRACT the analyst/architect table specifies
#    (public, application/c2pa, ?history=true), not against existing code —
#    all three are legitimately RED until Workstream D1 lands.
#
# 2. The "remote_manifests" C2PA claim field name used in AC3 is not a
#    guess: pdf-core already established it while implementing D1 —
#    pdf-core/features/manifest_url.feature:1-6 and
#    pdf-core/features/steps/dcs_pdf_core_steps.py:1816-1825 document that
#    c2pa-rs 0.85.1 (c2patool 0.26.61) REJECTS `remote_manifests` in V2
#    claims ("unknown V2 claim field: remote_manifests"), and that pdf-core
#    therefore currently does NOT embed it and instead documents the gap as
#    a deviation. Per this task's explicit user decision, AC3 is written
#    against the literal claim-field approach anyway (not XMP), so this
#    scenario is expected to stay RED until that known c2patool
#    incompatibility is resolved (or the deviation is formally accepted and
#    this scenario is re-pointed at a compatible verifier).
#
# 3. AC5's six lifecycle banners, per architect re-assessment + explicit user
#    decisions on this pass:
#      - Draft      -> ContractState DRAFT (reachable, existing workflow).
#      - Terminated -> ContractState TERMINATED (reachable, existing
#        workflow).
#      - Active     -> ContractState SIGNED is used as the reachable proxy:
#        provenance.MapCWEStateToC2PA (backend/internal/pdfgeneration/
#        provenance/lifecycle.go:85-86) maps BOTH "SIGNED" and "ACTIVE" to
#        the C2PA lifecycle value "active", and ContractState.Active itself
#        is unreachable via any wired command yet (EventDeploy — Workstream
#        G's deployment trigger — "is NOT triggered by any in-scope command
#        yet", contractstate/transition.go:48-52).
#      - Suspended  -> GO (user decision): lifecycle.go:87-88 maps
#        ContractState REVOKED to "suspended". signingmanagement/command/
#        revoke.go currently only flips the `contract_signatures` row's
#        status to REVOKED and never transitions the contract's own `state`
#        column — the accepted fix is to extend revoke.go to additionally
#        call contractstate.ValidateTransition(current, EventRevoke) +
#        UpdateState(Revoked) after the signature flip, analogous to
#        apply.go:123-127 (the Signed/Active -> Revoked transition edge
#        already exists, transition.go:130,134). The scenario below
#        therefore now exercises the real, wired /signature/revoke command
#        and expects a green result once that extension lands — it is no
#        longer treated as "not reachable".
#      - Replaced   -> OUT OF SCOPE for this pass (user decision): not a
#        ContractState value in contractstate/transition.go at all (only a
#        lowercase pass-through string in lifecycle.go:100-101 for callers
#        that already use SRS vocabulary directly) — genuinely unreachable
#        via any contract command today. Rather than inventing an ad hoc
#        trigger for it, this pack deliberately omits a real scenario for
#        Replaced; it is tracked as a @skip placeholder pointing at the
#        Deviation-Register entry the dokumentierer will add (C2PA-001
#        measurement family) — no green/red BDD result is expected for it.
#      - Expired    -> GO (user decision): reachable via a test-only seam.
#        `contract/update` (the only way to set exp_date through the API)
#        rejects it unless it is at least ONE DAY in the future
#        (command/update.go:114-118) and only accepts EventUpdate from Draft
#        state, so a real Expired contract would otherwise need a genuine
#        24h+ wait. Instead, this pack backdates `exp_date` directly via
#        context.db (mirroring the accepted `_seed_trusted_peer` precedent
#        in steps/peer_trust/dcs_peer_trust_steps.py:108-119) and polls
#        briefly for the already-running expiry cron
#        (contractworkflowengine/cronjobs.go, conf.ExpirationCronJobTimeOut()
#        = 1 minute; contractworkflowengine/db/pg/contractrepository.go:
#        241-261's ReadExpiredContracts query) to force-flip the contract to
#        EXPIRED. This is a test-support seam only — no production behavior
#        is changed by it.
#    Draft/Active/Terminated/Suspended are covered by the shared Scenario
#    Outline below, reusing the existing `contract "<name>" has reached
#    contract state "<state>"` step (steps/template_management/
#    contract_state_machine_steps.py's `_reach_state` helper). Expired is a
#    separate Scenario (it needs an extra backdating Given step beyond the
#    generic one). Replaced is a separate @skip Scenario (see above).

@DCS-OR-C2PA-008
Feature: C2PA remote manifest, verifier banner completeness (Workstream D)

  Background:
    Given I am authenticated with roles: "Contract Manager"

  @REQ-c2pa-conformance-AC1 @DCS-OR-C2PA-008
  Scenario: The public C2PA manifest endpoint returns the raw manifest store without auth
    Given contract "C2PA Manifest Contract" has reached contract state "SIGNED"
    And contract "C2PA Manifest Contract" has an exported PDF
    When I request the public C2PA manifest for contract "C2PA Manifest Contract"
    Then get http 200:Success code
    And the response has Content-Type "application/c2pa"
    And the response body is a non-empty C2PA JUMBF manifest store

  @REQ-c2pa-conformance-AC2 @DCS-OR-C2PA-008
  Scenario: The manifest history query returns a parsed JSON chain enumeration instead of raw bytes
    Given contract "C2PA History Contract" has reached contract state "SIGNED"
    And contract "C2PA History Contract" has an exported PDF
    When I request the C2PA manifest history for contract "C2PA History Contract"
    Then get http 200:Success code
    And the response is a JSON list of manifest labels with dcs.lifecycle assertions

  @REQ-c2pa-conformance-AC3 @DCS-OR-C2PA-008
  Scenario: The embedded manifest's remote_manifests claim field references the public manifest endpoint
    Given contract "C2PA Remote Ref Contract" has reached contract state "SIGNED"
    And contract "C2PA Remote Ref Contract" has an exported PDF
    When I request the public C2PA manifest for contract "C2PA Remote Ref Contract"
    Then get http 200:Success code
    And the C2PA manifest response for contract "C2PA Remote Ref Contract" declares a remote_manifests field pointing to its own public manifest endpoint

  @REQ-c2pa-conformance-AC4 @DCS-OR-C2PA-008 @skip
  Scenario: Verification succeeds from the remote manifest after the embedded attachment is stripped
    # See the module-level comment block above ("AC4 is INTENTIONALLY
    # @skip") for the full rationale. This scenario documents the INTENDED
    # behavior so the tags/traceability still exist; it is not executed as
    # black-box proof today. A genuine automated proof needs either:
    #   (a) a Go-level test in backend/internal/pdfgeneration/query (mocking
    #       IPFSClient.FetchFile to return a PDF whose
    #       `content_credential.c2pa` attachment has been stripped, then
    #       asserting VerifyContractPdfHandler falls back to
    #       pdf_manifest_ipfs_cid / the AC1 remote-manifest bytes), or
    #   (b) a new, reviewed test-support seam (e.g. exposing
    #       IPFS_TENANT_BASE_URL, already present in backend/.env.dev1:49,
    #       to this BDD harness so a step could legitimately overwrite the
    #       contract's stored PDF pointer) — not added here without
    #       architect sign-off, since it is closer to infrastructure than to
    #       a step definition.
    Given contract "C2PA Stripped Contract" has reached contract state "SIGNED"
    And contract "C2PA Stripped Contract" has an exported PDF
    And a local copy of contract "C2PA Stripped Contract"'s PDF with the content_credential.c2pa attachment removed
    When contract "C2PA Stripped Contract" is verified via the backend verify endpoint
    Then the verification result reports c2pa_manifest_found is true, served from the remote manifest fallback

  @REQ-c2pa-conformance-AC5 @DCS-OR-C2PA-006
  Scenario Outline: The verify endpoint reports the matching lifecycle_status banner for each state
    Given contract "<name>" has reached contract state "<contract_state>"
    When contract "<name>" is exported and verified as PDF
    Then get http 200:Success code
    And the C2PA lifecycle_status for contract "<name>" is "<banner>"

    Examples: reachable via existing/extended wired commands
      | name                          | contract_state | banner     |
      | C2PA Banner Draft             | DRAFT           | draft      |
      | C2PA Banner Active-via-Signed | SIGNED          | active     |
      | C2PA Banner Terminated        | TERMINATED      | terminated |
      | C2PA Banner Suspended         | REVOKED         | suspended  |

  @REQ-c2pa-conformance-AC5 @DCS-OR-C2PA-006
  Scenario: The verify endpoint reports the "expired" lifecycle_status banner after the expiry cron fires
    # "Expired" needs an extra Given step beyond the shared
    # "has reached contract state" one-liner (see design-gaps note above):
    # the contract must first reach a non-terminal state (SIGNED), then have
    # its exp_date backdated via the context.db test seam, then the already
    # -running expiry cron (polls every 1 minute) is given a short window to
    # force-flip it to EXPIRED before verifying.
    Given contract "C2PA Banner Expired" has reached contract state "SIGNED"
    And contract "C2PA Banner Expired" has an expiry date in the past
    When contract "C2PA Banner Expired" is exported and verified as PDF
    Then get http 200:Success code
    And the C2PA lifecycle_status for contract "C2PA Banner Expired" is "expired"

  @REQ-c2pa-conformance-AC5 @DCS-OR-C2PA-006 @skip
  Scenario: Replaced lifecycle banner is out of scope for this pass
    # Explicit user decision (see design-gaps note above): "Replaced" is not
    # a reachable ContractState in contractstate/transition.go at all (only
    # a lowercase pass-through string in lifecycle.go for callers that
    # already use SRS vocabulary directly), and no ad hoc trigger is
    # invented here to force it. This scenario intentionally documents the
    # gap via the AC5 tag/traceability without asserting anything — the
    # @skip keeps it out of both the green and red result counts, so it
    # cannot be misread as a genuine pass or a genuine failure. Tracked as
    # an out-of-scope deviation for the dokumentierer's Deviation-Register
    # (C2PA-001 measurement family), not as a BDD result.
    Given I am authenticated with roles: "Contract Manager"

  @REQ-c2pa-conformance-AC6 @DCS-OR-C2PA-006
  Scenario: The verify response carries four independently named checks, and PDF signature is honestly "not yet available"
    Given contract "C2PA Four Checks Contract" has reached contract state "SIGNED"
    And contract "C2PA Four Checks Contract" has an exported PDF
    When contract "C2PA Four Checks Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verify response for contract "C2PA Four Checks Contract" includes four named checks: PDF signature, C2PA manifest, VC signature, and status list
    And the PDF signature check for contract "C2PA Four Checks Contract" is marked as not yet available rather than passed
