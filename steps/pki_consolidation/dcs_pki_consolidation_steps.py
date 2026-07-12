"""BDD step definitions for the pki-consolidation-pkcs11 requirement
(Workstream A, docs/anforderung.md Zeilen 93-144).

Covers only the BDD-testable ACs (AC1, AC2, AC3, AC4, AC5, AC6, AC11, AC12).
AC10 is @skip in the feature file (no real runtime call site exists yet - see
the feature file's header comment). AC7 (extern-validiert), AC8/AC9/AC15
(grep-gate), AC13/AC14 (manueller-Drill) are deliberately NOT implemented
here.

Several scenarios (AC6, AC11, AC12) are written against ASSUMED endpoint
contracts / DB seams that do not exist in the codebase yet - each is
documented at its point of use with the exact grep/search that came up
empty, so a reader can tell "not yet designed" apart from "implemented
wrong". This mirrors the established precedent in
features/19_c2pa_conformance/c2pa_conformance.feature and
steps/peer_trust/dcs_peer_trust_steps.py for pre-design BDD packs.
"""

import base64
import json
import os

import jwt
import requests as _requests
from behave import given, then, when
from jwt.algorithms import ECAlgorithm

from steps.support.api_client import (
    c2pa_internal_sign_url,
    did_document_url,
    get_with_headers,
    post_json,
    signature_retrieve_url,
    signature_validate_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# AC1 / AC2 - this instance's own DID document
# ---------------------------------------------------------------------------


@when("I request this instance's own DID document")
def step_when_request_own_did_document(context):
    context.requests_response = _requests.get(
        did_document_url(context.base_url),
        timeout=context.http_timeout_seconds,
    )


@then("the DID document's verificationMethod key is an ECDSA P-256 JWK, not RSA")
def step_then_did_jwk_is_ec_p256(context):
    body = context.requests_response.json()
    verification_methods = body.get("verificationMethod") or []
    assert verification_methods, (
        f"DID document has no verificationMethod entries: {body}"
    )
    jwk = verification_methods[0].get("publicKeyJwk") or {}
    assert jwk.get("kty") == "EC", (
        "Expected the DID key's publicKeyJwk.kty to be 'EC' (hsm.PublicJWK for the "
        f"HSM-backed dcs-did key, ECDSA P-256), got: {jwk}"
    )
    assert jwk.get("crv") == "P-256", (
        f"Expected the DID key's publicKeyJwk.crv to be 'P-256', got: {jwk}"
    )
    assert "n" not in jwk and "e" not in jwk, (
        "DID key's publicKeyJwk still carries RSA fields ('n'/'e') alongside/instead of "
        f"EC fields - the legacy identity.PublicKeyJWK{{Kty, N, E}} struct has not been "
        f"migrated to EC (x/y): {jwk}"
    )
    assert jwk.get("x") and jwk.get("y"), (
        f"Expected EC JWK 'x' and 'y' coordinates to be present, got: {jwk}"
    )


# ---------------------------------------------------------------------------
# AC3 - OpenID4VP JAR, ES256-signed by the dcs-oid4vp-jar HSM key
# ---------------------------------------------------------------------------


@when("I start an OpenID4VP login and fetch the signed authorization request object")
def step_when_fetch_jar(context):
    login_resp = _requests.post(
        f"{context.base_url}/auth/login", timeout=context.http_timeout_seconds
    )
    assert login_resp.status_code == 200, (
        f"POST /auth/login failed: {login_resp.status_code} {login_resp.text}"
    )
    state = login_resp.json().get("state")
    assert state, f"/auth/login response has no 'state': {login_resp.text}"
    context.oid4vp_login_state = state

    context.requests_response = _requests.get(
        f"{context.base_url}/auth/presentation/request/{state}",
        headers={"Accept": "application/oauth-authz-req+jwt, application/jwt"},
        timeout=context.http_timeout_seconds,
    )


@then(
    "the authorization request JWT is ES256-signed with an embedded EC P-256 JWK "
    "verifiable against itself"
)
def step_then_jar_is_es256_self_verifiable(context):
    token = context.requests_response.text.strip()
    assert token, "authorization request response body is empty"

    header = jwt.get_unverified_header(token)
    assert header.get("alg") == "ES256", (
        f"Expected the JAR JWT header 'alg' to be 'ES256', got: {header}"
    )
    embedded_jwk = header.get("jwk")
    assert isinstance(embedded_jwk, dict), (
        f"Expected the JAR JWT header to carry an embedded 'jwk' claim, got header: {header}"
    )
    assert embedded_jwk.get("kty") == "EC" and embedded_jwk.get("crv") == "P-256", (
        f"Expected the embedded JWK to be an EC P-256 key, got: {embedded_jwk}"
    )

    # Self-consistency check: the JWT genuinely verifies against its own
    # embedded JWK. Combined with the alg=ES256 assertion above, this proves
    # a real ECDSA P-256 private-key operation produced these bytes (not
    # merely that the header CLAIMS ES256) - the concrete instantiation of
    # "hsm.Signer produces a signature verifiable against hsm.PublicJWK" for
    # the dcs-oid4vp-jar label.
    public_key = ECAlgorithm.from_jwk(json.dumps(embedded_jwk))
    try:
        jwt.decode(
            token,
            public_key,
            algorithms=["ES256"],
            options={"verify_aud": False, "verify_exp": False},
        )
    except Exception as exc:  # noqa: BLE001 - re-raised as an assertion for behave
        raise AssertionError(
            f"authorization request JWT signature does not verify against its own "
            f"embedded JWK: {exc}"
        ) from exc


@then("the authorization request JWT's kid names the dcs-oid4vp-jar HSM key label")
def step_then_jar_kid_names_hsm_label(context):
    token = context.requests_response.text.strip()
    header = jwt.get_unverified_header(token)
    expected_kid = os.getenv("DCS_HSM_KEY_JAR", "dcs-oid4vp-jar")
    assert header.get("kid") == expected_kid, (
        f"Expected the JAR JWT header 'kid' to name the HSM key label used for signing "
        f"('{expected_kid}', from env DCS_HSM_KEY_JAR or its documented default), got: "
        f"{header.get('kid')!r} (full header: {header})"
    )


# ---------------------------------------------------------------------------
# AC4 - Contract-Lifecycle-VC proof is ECDSA/ES256, not Ed25519Signature2020
# ---------------------------------------------------------------------------


@then(
    'the embedded contract-lifecycle VC proof for contract "{name}" is ECDSA/ES256, '
    "not Ed25519Signature2020"
)
def step_then_vc_proof_is_ecdsa(context, name):
    pdf_bytes = context.requests_response.content
    assert pdf_bytes, f"PDF export response for contract '{name}' has an empty body"

    # The VC's EmbeddedFile stream is written uncompressed (no /Filter
    # FlateDecode - see pdf-core/compiler/update.go:390-392), so its JSON
    # content is a plain, searchable substring of the PDF bytes, the same
    # way the existing "contract.jsonld" attachment-name check works
    # (steps/pdf_generation/pdf_steps.py:_utf16be usage) - no PDF parsing
    # library is required.
    assert b"Ed25519Signature2020" not in pdf_bytes, (
        f"Exported PDF for contract '{name}' still embeds a VC proof of type "
        "'Ed25519Signature2020' - the PKI consolidation refactor (docs/anforderung.md "
        "Workstream A2.2) requires an ECDSA-based proof suite instead"
    )
    lowered = pdf_bytes.lower()
    assert b"es256" in lowered or b"ecdsa" in lowered, (
        f"Expected the embedded contract-lifecycle VC proof for contract '{name}' to "
        "declare an ECDSA/ES256 proof suite (e.g. a JsonWebSignature2020 proof with "
        "alg ES256, or a DataIntegrityProof with cryptosuite 'ecdsa-rdfc-2019') - found "
        "neither 'ES256' nor 'ecdsa' anywhere in the exported PDF bytes"
    )


# ---------------------------------------------------------------------------
# AC5 - two-instance: both instances publish an EC P-256 DID key
#
# The Given/When/Then steps for "instance A and instance B are both running
# and trust each other", "the initiator on instance A creates and offers a
# contract with instance B as negotiator and approver", and "the contract
# appears on instance B in state OFFERED within a few seconds" are already
# registered by steps/peer_trust/dcs_peer_trust_steps.py (behave's step
# registry is global across step modules) and are reused as-is here rather
# than duplicated - only the new EC-P-256-specific assertion below is added.
# ---------------------------------------------------------------------------


@then("instance A and instance B each publish an ECDSA P-256 DID key, not RSA")
def step_then_both_instances_publish_ec_p256(context):
    for label, base_url in (("A", context.base_url_a), ("B", context.base_url_b)):
        resp = _requests.get(
            did_document_url(base_url), timeout=context.http_timeout_seconds
        )
        assert resp.status_code == 200, (
            f"instance {label} did.json unreachable: {resp.status_code} {resp.text}"
        )
        verification_methods = resp.json().get("verificationMethod") or []
        assert verification_methods, (
            f"instance {label}'s DID document has no verificationMethod entries"
        )
        jwk = verification_methods[0].get("publicKeyJwk") or {}
        assert jwk.get("kty") == "EC" and jwk.get("crv") == "P-256", (
            f"Expected instance {label}'s DID key to be ECDSA P-256 (AC5/A2.4 is a "
            "breaking change: both instances must switch to the HSM-backed ECDSA DID "
            f"signer simultaneously), got kty={jwk.get('kty')!r} crv={jwk.get('crv')!r}"
        )


# ---------------------------------------------------------------------------
# AC6 - new authenticated C2PA-signing endpoint (ASSUMED contract) + full
# export's COSE alg
# ---------------------------------------------------------------------------


@when(
    "I request an ES256 C2PA signature for a COSE Sig_structure payload from the "
    "new internal signing endpoint"
)
def step_when_request_c2pa_signature(context):
    # A minimal, syntactically-plausible COSE Sig_structure ("Signature1"
    # array, see pdf-core/compiler/compiler_c2pa.go:630-638) - the exact
    # bytes do not matter for this scenario (the endpoint is expected to sign
    # whatever bytes it is given), only that the response is a well-formed
    # ES256 signature over them.
    sig_structure = base64.b64encode(b"bdd-pki-consolidation-sig-structure-fixture").decode()
    context.requests_response = post_json(
        context,
        c2pa_internal_sign_url(context),
        {"sig_structure": sig_structure},
        headers=getattr(context, "headers", {}),
    )


@then("the returned signature is a well-formed 64-byte ES256 (r||s) signature")
def step_then_signature_is_well_formed_es256(context):
    body = context.requests_response.json()
    signature_b64 = body.get("signature")
    assert signature_b64, (
        f"Expected a 'signature' field in the C2PA signing endpoint response, got: {body}"
    )
    signature_bytes = base64.b64decode(signature_b64)
    # ES256 raw JOSE/COSE signatures are exactly 64 bytes (32-byte r || 32-byte
    # s) - distinct from the DER-encoded ASN.1 signature crypto11's
    # crypto.Signer would return by default and from an Ed25519 signature
    # (which happens to also be 64 bytes, but is a fundamentally different
    # scheme - this check only confirms SHAPE, not scheme; full end-to-end
    # scheme proof is AC6's second scenario, the COSE alg check below, which
    # inspects a real signed manifest rather than this isolated endpoint).
    assert len(signature_bytes) == 64, (
        f"Expected a 64-byte raw r||s ES256 signature, got {len(signature_bytes)} bytes"
    )
    # NOTE (open point for architect): this isolated endpoint response does
    # not (yet) carry the public key/certificate needed to cryptographically
    # verify this specific signature from the BDD harness. Consider having
    # the endpoint also return the public JWK or x5chain used, so this
    # scenario can do a full verify rather than a shape-only check.


@then("the exported PDF's C2PA COSE_Sign1 protected header declares alg ES256(-7), not EdDSA(-8)")
def step_then_cose_alg_is_es256(context):
    pdf_bytes = context.requests_response.content
    assert pdf_bytes, "PDF export response has an empty body"

    # The COSE protected header is built as a 2-pair CBOR map {1: alg, 33:
    # x5chain} (pdf-core/compiler/compiler_c2pa.go:616-628,
    # buildCoseProtectedHeadersWithX5Chain): cborMap's header byte for a
    # 2-pair map is 0xA2 (major type 5, n=2), followed by cborUint(1) = 0x01
    # (key 1), followed by cborNegInt(alg) - CBOR negative-int encoding
    # stores -(n+1), so alg=-7 (ES256) encodes as byte 0x26 and alg=-8
    # (EdDSA) encodes as byte 0x27. The resulting 3-byte sequences \xa2\x01\x26
    # (ES256) / \xa2\x01\x27 (EdDSA) are specific enough to search for
    # directly in the raw (binary, JUMBF-embedded) manifest bytes, the same
    # direct-byte-search approach steps/pdf_generation/pdf_steps.py already
    # uses for the ASCII "%%C2PA-MANIFEST-BEGIN" marker.
    es256_marker = b"\xa2\x01\x26"
    eddsa_marker = b"\xa2\x01\x27"
    assert eddsa_marker not in pdf_bytes, (
        "Exported PDF's C2PA manifest still declares COSE alg EdDSA(-8) "
        f"(found protected-header byte pattern {eddsa_marker!r}) - the PKI consolidation "
        "refactor (docs/anforderung.md Workstream A2.3) requires ES256(-7) instead"
    )
    assert es256_marker in pdf_bytes, (
        "Exported PDF's C2PA manifest does not declare COSE alg ES256(-7) "
        f"(protected-header byte pattern {es256_marker!r} not found) - see "
        "pdf-core/compiler/compiler_c2pa.go:616-628 for the exact CBOR construction "
        "this pattern is derived from"
    )


# ---------------------------------------------------------------------------
# AC11 - CRL revocation flips a previously valid signature to invalid
#
# NOTE: there is no dev CA / CRL infrastructure in this codebase yet at all
# (`grep -rn "CRL" backend/internal/signingmanagement` returns nothing at the
# time this pack was written - Workstream A3/A5 have not landed). The Given
# step below seeds an ASSUMED persistence point (a new `cert_revoked_at`
# column on the existing `contract_signatures` table, extending rather than
# inventing an unrelated table) directly via context.db, mirroring the
# already-accepted `_seed_trusted_peer` (steps/peer_trust/
# dcs_peer_trust_steps.py) and exp_date-backdating (steps/template_management/
# contract_state_machine_steps.py) precedents for test-only DB seams. If the
# implementer instead models CRL revocation via a serial-number-keyed table
# (closer to how a real X.509 CRL works), only this Given step's SQL needs to
# be re-pointed - the Then assertions are the requirement-accurate,
# load-bearing part.
# ---------------------------------------------------------------------------


@given('signature validation for contract "{name}" currently reports no certificate-revocation finding')
def step_given_baseline_no_cert_revocation_finding(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)
    assert resp.status_code == 200, (
        f"Baseline /signature/validate call failed for contract '{name}': "
        f"{resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    assert not ("revoked" in body_text and ("cert" in body_text or "crl" in body_text)), (
        f"Precondition violated: contract '{name}' already reports a certificate-"
        f"revocation finding before any CRL revocation was seeded: {findings}"
    )


@given(
    'the dev signing certificate used for contract "{name}"\'s signature has been revoked '
    "in the CRL"
)
def step_given_cert_revoked_in_crl(context, name):
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "UPDATE contract_signatures SET cert_revoked_at = NOW() WHERE contract_did = %s",
            (did,),
        )
        context.db.commit()
    except Exception as exc:  # noqa: BLE001
        context.db.rollback()
        raise AssertionError(
            "Could not seed the AC11 CRL-revocation test seam: this assumes a "
            "'cert_revoked_at' column on 'contract_signatures' that does not exist yet "
            f"(docs/anforderung.md Workstream A5 - see this step's module docstring for "
            f"the full rationale): {exc}"
        ) from exc
    finally:
        cursor.close()


@when('I validate the signature for contract "{name}"')
def step_when_validate_signature(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, signature_validate_url(context), {"did": did}, headers=manager_h
    )


@then('signature validation for contract "{name}" reports the certificate as revoked')
def step_then_validate_reports_cert_revoked(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"/signature/validate failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    assert "revoked" in body_text and ("cert" in body_text or "crl" in body_text), (
        f"Expected a certificate/CRL revocation finding (distinct from the existing "
        f"business-level '/signature/revoke' REVOKED-status finding, see "
        f"contractrepository.go's existing 'case \"REVOKED\":' handling) for contract "
        f"'{name}' after revoking its signing certificate in the CRL, got findings: "
        f"{findings}"
    )


# ---------------------------------------------------------------------------
# AC12 - key rotation: old signature stays valid, new signature uses the new
# key, distinguishably
#
# NOTE: key rotation is explicitly an OPS action (a script or Helm Job, per
# docs/anforderung.md Workstream A5's own rotation procedure) - it is not
# triggerable via any HTTP endpoint by design, and there is no versioned-
# key-label mechanism in the codebase yet at all (`grep -rn "key_version\|
# active_version" backend/` returns nothing). The Given step below seeds an
# ASSUMED settings row directly via context.db; the Then assertions read an
# ASSUMED 'key_version' field from GET /signature/retrieve/{did}, which also
# does not exist yet. Both are open points for whatever exact schema A5
# lands with - the important, requirement-accurate claim under test is "old
# and new signatures are distinguishable by key version, and the old one
# keeps validating".
# ---------------------------------------------------------------------------


@given("the active dcs-contract-pades HSM key version has been rotated to a new version")
def step_given_rotate_key_version(context):
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "INSERT INTO pki_active_key_version (label, active_version) "
            "VALUES ('dcs-contract-pades', 2) "
            "ON CONFLICT (label) DO UPDATE SET "
            "active_version = pki_active_key_version.active_version + 1"
        )
        context.db.commit()
    except Exception as exc:  # noqa: BLE001
        context.db.rollback()
        raise AssertionError(
            "Could not seed the AC12 key-rotation test seam: this assumes a "
            "'pki_active_key_version' settings table that does not exist yet "
            f"(docs/anforderung.md Workstream A5 - see this step's module docstring for "
            f"the full rationale): {exc}"
        ) from exc
    finally:
        cursor.close()


@then('signature validation for contract "{name}" reports the signature as still valid after rotation')
def step_then_still_valid_after_rotation(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)
    assert resp.status_code == 200, (
        f"/signature/validate failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    invalidity_markers = ("invalid", "key not found", "unknown key", "expired key", "no such key")
    hit = [m for m in invalidity_markers if m in body_text]
    assert not hit, (
        f"Expected the historical signature for contract '{name}' to remain valid after "
        f"key rotation (old key material must stay usable for verification in the token/"
        f"trust store per docs/anforderung.md Workstream A5), got findings suggesting "
        f"invalidity ({hit}): {findings}"
    )


@then(
    'the applied signatures for contracts "{old_name}" and "{new_name}" are attributed '
    "to different HSM key versions"
)
def step_then_different_key_versions(context, old_name, new_name):
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    versions = {}
    for name in (old_name, new_name):
        did, _ = ContractService._contract_data(context, name)
        resp = get_with_headers(context, signature_retrieve_url(context, did), headers=manager_h)
        assert resp.status_code == 200, (
            f"GET /signature/retrieve/{{did}} failed for contract '{name}': "
            f"{resp.status_code} {resp.text}"
        )
        body = resp.json()
        version = body.get("key_version") if isinstance(body, dict) else None
        assert version is not None, (
            f"Expected the signature evidence for contract '{name}' to name the HSM key "
            f"label/version used (AC12 requires old and new signatures to be "
            f"distinguishable by key label/version) - no such field found in: {body}"
        )
        versions[name] = version
    assert versions[old_name] != versions[new_name], (
        f"Expected '{old_name}' (signed before rotation) and '{new_name}' (signed after "
        f"rotation) to be attributed to different HSM key versions, got the same value "
        f"'{versions[old_name]}' for both"
    )
