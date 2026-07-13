# Requirement: pki-consolidation-pkcs11
#
# Covers Workstream A ("PKI consolidation: PKCS#11 + SoftHSM2, trust anchors,
# rotation", docs/anforderung.md Zeilen 93-144) - only the ACs the analyst
# marked Pruefmittel = BDD:
#
#   AC1  - Backend with a correctly configured PKCS#11 module/token/PIN
#          becomes reachable/healthy (positive path only - see rationale
#          below for why the negative "hard-abort on misconfiguration" path
#          is NOT modeled as a Gherkin scenario here).
#   AC2  - hsm.Signer(label)/hsm.PublicJWK(label) is instantiated for the
#          DID key: GET /.well-known/did.json publishes an ECDSA P-256 JWK
#          (kty EC, crv P-256), not the current RSA JWK (kty RSA, n/e).
#   AC3  - The OID4VP JAR (POST /auth/login -> GET/POST
#          /auth/presentation/request/{state}) is ES256-signed with an
#          embedded EC P-256 JWK that the JWT itself verifies against, and
#          the signing key's kid matches the dcs-oid4vp-jar HSM label.
#   AC4  - A PDF export's embedded Contract-Lifecycle-VC no longer carries
#          an Ed25519Signature2020 proof; it declares an ECDSA/ES256 proof
#          suite instead.
#   AC5  - (two-instance) The did:web peer-sync challenge is signed with the
#          HSM DID key and verified by the receiving instance against the
#          verificationMethod JWK - both instances must be ECDSA P-256
#          simultaneously (breaking change, per the user decision).
#   AC6  - The new, authenticated, non-public C2PA-signing backend endpoint
#          returns a well-formed ES256 signature for Sig_structure bytes;
#          AND a full PDF export's embedded C2PA manifest declares COSE alg
#          ES256(-7), not EdDSA(-8).
#   AC10 - DCS_TRUST_ANCHORS=dev-ca|eu-lotl switches the trust-anchor source
#          for all verification layers via configuration - see the @skip
#          scenario below for why this is NOT modeled as a real Gherkin
#          scenario in this pass (the mechanism does not exist in the
#          codebase yet in ANY form, and its closest existing relative,
#          VerifyEIDASCertificate/EUTrustPool, is a startup-only self-check,
#          not a runtime per-peer/per-signature verification path).
#   AC11 - Revoking the dev signing certificate in the CRL flips a
#          previously valid signature's /signature/validate result from
#          "no revocation finding" to "certificate revoked".
#   AC12 - After a key rotation (new versioned label, active pointer moved),
#          a historical signature made with the OLD key still validates,
#          while a new signing operation is attributable to the NEW key
#          version.
#
# Deliberately OUT of scope for this pack (Pruefmittel != BDD, checked by the
# verifier against recorded manual/extern/grep evidence instead):
#   - AC7 (extern-validiert), AC8/AC9/AC15 (grep-gate), AC13/AC14
#     (manueller-Drill - AC13 is the rotation-drill evidence itself; per this
#     task's explicit instruction it is NOT covered by a Gherkin scenario
#     here at all, not even a documented @skip).
#
# --- Binding decisions for this pass (see the task/owner instructions) ---
#
# 1. C2PA signing path (AC6): pdf-core holds NO PKCS11/signature material.
#    pdf-core builds the COSE Sig_structure bytes and calls a NEW,
#    authenticated backend endpoint that signs via hsm.Signer("dcs-c2pa")
#    and returns the raw ES256 (r||s) signature; pdf-core only embeds it.
#    This endpoint DOES NOT EXIST YET (searched backend/design/*.go - no
#    match for "c2pa" + "sign" together outside the existing public,
#    unauthenticated C2PAService.GetManifest). The scenario below is written
#    against an ASSUMED contract - POST /internal/c2pa/sign, authenticated
#    (Security(JWTAuth) with a "Sys. Contract Signer"-style scope, mirroring
#    the existing sys-role vocabulary on /signature/apply and /signature/
#    verify), payload {"sig_structure": "<base64>"}, response
#    {"signature": "<base64, 64 raw r||s bytes>"} - this is an OPEN POINT for
#    the architect to confirm/rename; the important, load-bearing behavior
#    under test is "authenticated endpoint exists and returns an ES256-shaped
#    signature", not the exact path string.
#
# 2. Rotation-drill evidence (AC13, NOT part of this BDD pack - manueller-
#    Drill): protokolliertes Vorher/Nachher + weiterhin gueltige Altsignatur.
#    No Gherkin scenario here for AC13 itself; AC12 below is the narrower,
#    BDD-testable claim ("old key's signature still validates, new signing
#    uses the new key") that AC13's drill will additionally evidence
#    manually.
#
# --- Design gaps / open points this pack surfaced ---
#
# a) AC1's negative path ("missing/wrong PKCS11 module path, token label, or
#    PIN aborts startup hard") cannot be driven by this BDD harness: there is
#    exactly ONE already-running backend instance under test
#    (BDD_DCS_BASE_URL), and this harness has no supervisor step that can
#    restart it with deliberately broken environment variables and then
#    assert the process never becomes reachable. The existing convention for
#    this exact class of problem in this codebase is main.go's own hard
#    log.Fatalf pattern for other required dependencies (see
#    backend/cmd/dcs/main.go:376-391, crypto-provider/status-list-service
#    readiness probes) - hsm.Open() is expected to follow the identical
#    pattern per docs/anforderung.md A1 ("fail HARD if module/token/PIN
#    wrong"). This is treated the same way AC1 (positive path only) is
#    treated here: the POSITIVE path ("this reachable instance's own
#    hsm.Open()-gated DID document is servable") is exercised as real,
#    non-skipped evidence; the negative path is documented here as an
#    accepted manual/ops-verification concern (start the backend once with a
#    deliberately wrong PKCS11_MODULE_PATH/PKCS11_PIN and observe it never
#    becomes healthy) rather than invented as a dishonest BDD scenario.
#
# b) AC10 has NO existing runtime mechanism to hook into at all (see the
#    @skip scenario below for the full rationale) - flagged for
#    analyst/architect re-scoping once Workstream A5's TrustAnchors
#    abstraction has an actual call site this harness can drive over HTTP.
#
# c) AC11/AC12 both need persistence points (a CRL-revocation marker; a
#    versioned active-key-version pointer) that do not exist in the schema
#    yet at all (`grep -rn "CRL\|crl" backend/internal/signingmanagement`
#    and `grep -rn "key_version\|active_version" backend/` both return
#    nothing at the time this pack was written). Both scenarios document
#    their assumed seam explicitly in the step file
#    (steps/pki_consolidation/dcs_pki_consolidation_steps.py) and are
#    expected to be RED for "seam does not exist" reasons in addition to
#    "feature not implemented" reasons - re-point the Given steps at
#    whatever schema A5 actually lands with; the Then assertions are the
#    load-bearing, requirement-accurate part.

@DCS-IR-HI-01 @DCS-NFR-SEC-02
Feature: PKI consolidation - PKCS#11 + SoftHSM2, ECDSA P-256, trust anchors, rotation

  @REQ-pki-consolidation-pkcs11-AC1 @DCS-IR-HI-01
  Scenario: A correctly PKCS#11-configured backend is reachable and serves its HSM-backed DID document
    # See design-gaps note (a) above for why only the positive path is
    # modeled: a reachable, 200-responding /.well-known/did.json can only
    # happen if main.go's hsm.Open() (which must fail hard on a bad PKCS11
    # module/token/PIN per A1) already succeeded at startup - the same
    # "reachability implies the hard startup gate passed" argument the
    # crypto-provider/status-list-service readiness probes already rely on
    # in this codebase (backend/cmd/dcs/main.go:376-391).
    When I request this instance's own DID document
    Then get http 200:Success code

  @REQ-pki-consolidation-pkcs11-AC2 @DCS-IR-HI-01
  Scenario: The published DID key is an ECDSA P-256 JWK, not the legacy RSA JWK
    # Per the task's own guidance: reuse the existing public DID-document
    # endpoint as the AC2 test point rather than inventing a test-only API.
    # A full sign+verify round trip for a NON-peer-facing label (proving
    # hsm.Signer(label) output verifies against hsm.PublicJWK(label) end to
    # end) is already demonstrated concretely for a DIFFERENT label
    # (dcs-oid4vp-jar) in AC3 below, and for the DID label specifically in
    # AC5's two-instance peer-sync round trip - this scenario is the
    # lighter-weight, single-instance structural proof that hsm.PublicJWK
    # for the DID key is wired to publish EC P-256, not RSA.
    When I request this instance's own DID document
    Then get http 200:Success code
    And the DID document's verificationMethod key is an ECDSA P-256 JWK, not RSA

  @REQ-pki-consolidation-pkcs11-AC3 @DCS-IR-HI-01
  Scenario: The OpenID4VP authorization request JWT (JAR) is ES256-signed by the dcs-oid4vp-jar HSM key
    When I start an OpenID4VP login and fetch the signed authorization request object
    Then get http 200:Success code
    And the authorization request JWT is ES256-signed with an embedded EC P-256 JWK verifiable against itself
    And the authorization request JWT's kid names the dcs-oid4vp-jar HSM key label

  @REQ-pki-consolidation-pkcs11-AC4 @DCS-IR-HI-01
  Scenario: The exported PDF's Contract-Lifecycle-VC proof is ECDSA/ES256, not Ed25519Signature2020
    Given I am authenticated with roles: "Contract Manager"
    And contract "PKI VC Proof Contract" is in "Draft" status
    When I export contract "PKI VC Proof Contract" as PDF
    Then get http 200:Success code
    And the embedded contract-lifecycle VC proof for contract "PKI VC Proof Contract" is ECDSA/ES256, not Ed25519Signature2020

  @REQ-pki-consolidation-pkcs11-AC5 @DCS-IR-HI-01 @two-instance
  Scenario: The peer-sync challenge is DID-key signed with ECDSA P-256 and verified on both instances
    # Reuses the existing two-instance Given/When/Then steps from
    # steps/peer_trust/dcs_peer_trust_steps.py (the two-instance-peer-trust
    # requirement's AC7) rather than duplicating that ~80-line setup: a
    # successful offer replication between A and B can only happen if BOTH
    # instances' did:web challenge-response signing/verification (base/
    # identity/did.go Sign/Verify) works - which, after this refactor, means
    # ECDSA P-256 end to end. This IS the genuine breaking-change test: it
    # only turns green once both instance A and instance B are on the
    # PKCS#11/ECDSA DID signer simultaneously.
    Given instance A and instance B are both running and trust each other
    Then instance A and instance B each publish an ECDSA P-256 DID key, not RSA
    When the initiator on instance A creates and offers a contract with instance B as negotiator and approver
    Then the contract appears on instance B in state OFFERED within a few seconds

  @REQ-pki-consolidation-pkcs11-AC6 @DCS-IR-HI-01
  Scenario: The new authenticated C2PA-signing endpoint returns a well-formed ES256 signature
    # See binding decision 1 in the header comment above: the exact endpoint
    # path/payload is an ASSUMED contract (POST /internal/c2pa/sign), not
    # yet designed in backend/design/*.go. This scenario is expected to be
    # RED until that design work lands, independent of whether hsm.Signer
    # itself is done - that is the correct, intended signal (same class of
    # "endpoint contract assumed ahead of design" precedent already
    # established in features/19_c2pa_conformance/c2pa_conformance.feature
    # for GET /c2pa/manifest/{contract_did}).
    Given I am authenticated with roles: "Contract Signer"
    When I request an ES256 C2PA signature for a COSE Sig_structure payload from the new internal signing endpoint
    Then get http 200:Success code
    And the returned signature is a well-formed 64-byte ES256 (r||s) signature

  @REQ-pki-consolidation-pkcs11-AC6 @DCS-IR-HI-01
  Scenario: A full PDF export's embedded C2PA manifest declares COSE alg ES256, not EdDSA
    Given I am authenticated with roles: "Contract Manager"
    And contract "PKI C2PA COSE Alg Contract" is in "Draft" status
    When I export contract "PKI C2PA COSE Alg Contract" as PDF
    Then get http 200:Success code
    And the exported PDF's C2PA COSE_Sign1 protected header declares alg ES256(-7), not EdDSA(-8)

  @REQ-pki-consolidation-pkcs11-AC10 @DCS-NFR-SEC-02 @skip
  Scenario: DCS_TRUST_ANCHORS switches the trust-anchor source for all verification layers
    # @skip — deferred BY DECISION, not by test limitation. The swappable
    # trust-anchor source demands the "trust migration" the codebase itself
    # parks (sdjwt/keys.go: x5c accepted WITHOUT chain validation "until
    # chain validation lands with the trust migration"), and the project's
    # authoritative blocker-decision register
    # (entscheidungen_zu_den_blockern.txt) explicitly defers exactly this
    # work: deviation 8 (PoA/issuer chain-walk to a trust anchor = roadmap,
    # SRS TBD-B acknowledges the XFSC PCM ecosystem is not yet testable)
    # and deviation 10a (issuer-authorization validation beyond the known-
    # roles check = "offen und eingeplant"). Landing DCS_TRUST_ANCHORS
    # without that migration would either be a config flag with no enforced
    # semantics (false green) or a unilateral pre-emption of a consciously
    # deferred architecture decision. The tag keeps AC10's traceability
    # present; the @skip keeps it out of pass/fail counts.
    Given I am authenticated with roles: "Contract Manager"

  @REQ-pki-consolidation-pkcs11-AC11 @DCS-OR-C2PA-007
  Scenario: Revoking the dev signing certificate in the CRL flips a previously valid signature to invalid
    Given contract "PKI CRL Revocation Contract" has reached contract state "SIGNED"
    And signature validation for contract "PKI CRL Revocation Contract" currently reports no certificate-revocation finding
    And the dev signing certificate used for contract "PKI CRL Revocation Contract"'s signature has been revoked in the CRL
    When I validate the signature for contract "PKI CRL Revocation Contract"
    Then get http 200:Success code
    And signature validation for contract "PKI CRL Revocation Contract" reports the certificate as revoked

  @REQ-pki-consolidation-pkcs11-AC12 @DCS-OR-C2PA-007
  Scenario: A historical signature made with the old HSM key keeps validating after rotation, while a new signature uses the new key
    Given I am authenticated with roles: "Contract Manager"
    And contract "PKI Rotation Old Contract" has reached contract state "SIGNED"
    When I validate the signature for contract "PKI Rotation Old Contract"
    Then get http 200:Success code
    And signature validation for contract "PKI Rotation Old Contract" reports the signature as still valid after rotation
    Given the active dcs-contract-pades HSM key version has been rotated to a new version
    And contract "PKI Rotation New Contract" has reached contract state "SIGNED"
    When I validate the signature for contract "PKI Rotation Old Contract"
    Then get http 200:Success code
    And signature validation for contract "PKI Rotation Old Contract" reports the signature as still valid after rotation
    And the applied signatures for contracts "PKI Rotation Old Contract" and "PKI Rotation New Contract" are attributed to different HSM key versions
