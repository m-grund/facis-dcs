"""BDD step definitions for PKI consolidation
(features/21_pki_consolidation_pkcs11; SRS DCS-IR-HI-01, DCS-NFR-SEC-02,
DCS-OR-C2PA-007).

The swappable trust-anchor scenario (DCS_TRUST_ANCHORS) is @skip in the
feature file - see its inline comment. The CRL-revocation and key-rotation
scenarios seed their preconditions directly via the test DB connection
(context.db); each seam is documented at its point of use.
"""

import json
import os

import jwt
import requests as _requests
from behave import given, then, when
from jwt.algorithms import ECAlgorithm

from steps.support.api_client import (
    did_document_url,
    get_with_headers,
    post_json,
    signature_retrieve_url,
    signature_validate_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# This instance's own DID document
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
        "DID key's publicKeyJwk carries RSA fields ('n'/'e') alongside/instead of "
        f"the EC (x/y) fields the P-256 dcs-did key publishes: {jwk}"
    )
    assert jwk.get("x") and jwk.get("y"), (
        f"Expected EC JWK 'x' and 'y' coordinates to be present, got: {jwk}"
    )


# ---------------------------------------------------------------------------
# OpenID4VP JAR, ES256-signed by the dcs-oid4vp-jar HSM key
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
# Contract-Lifecycle-VC proof is ECDSA/ES256, not Ed25519Signature2020
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
        "'Ed25519Signature2020' - the HSM-backed VC signer (DCS-IR-HI-01) "
        "requires an ECDSA-based proof suite instead"
    )
    lowered = pdf_bytes.lower()
    assert b"es256" in lowered or b"ecdsa" in lowered, (
        f"Expected the embedded contract-lifecycle VC proof for contract '{name}' to "
        "declare an ECDSA/ES256 proof suite (e.g. a JsonWebSignature2020 proof with "
        "alg ES256, or a DataIntegrityProof with cryptosuite 'ecdsa-rdfc-2019') - found "
        "neither 'ES256' nor 'ecdsa' anywhere in the exported PDF bytes"
    )


# ---------------------------------------------------------------------------
# Two-instance: both instances publish an EC P-256 DID key
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
            f"Expected instance {label}'s DID key to be ECDSA P-256 (this is a "
            "breaking change: both instances must switch to the HSM-backed ECDSA DID "
            f"signer simultaneously), got kty={jwk.get('kty')!r} crv={jwk.get('crv')!r}"
        )


# ---------------------------------------------------------------------------
# Full export's COSE alg (pdf-core embeds ES256 COSE_Sign1; the DCS signs the
# Sig_structure with the dcs-c2pa key and pdf-core embeds it — pdf-core is keyless)
# ---------------------------------------------------------------------------


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
        "refactor requires ES256(-7) instead"
    )
    assert es256_marker in pdf_bytes, (
        "Exported PDF's C2PA manifest does not declare COSE alg ES256(-7) "
        f"(protected-header byte pattern {es256_marker!r} not found) - see "
        "pdf-core/compiler/compiler_c2pa.go:616-628 for the exact CBOR construction "
        "this pattern is derived from"
    )


# ---------------------------------------------------------------------------
# CRL revocation flips a previously valid signature to invalid
#
# The Given step below seeds the revocation marker (the `cert_revoked_at`
# column on `contract_signatures`) directly via context.db, mirroring the
# accepted `_seed_trusted_peer` (steps/peer_trust/dcs_peer_trust_steps.py)
# and exp_date-backdating (steps/template_management/
# contract_state_machine_steps.py) precedents for test-only DB seams. The
# Then assertions on /signature/validate are the requirement-accurate,
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
            "Could not seed the CRL-revocation test seam (the 'cert_revoked_at' "
            f"column on 'contract_signatures'): {exc}"
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
# Key rotation: old signature stays valid, new signature uses the new key,
# distinguishably
#
# Key rotation is an OPS action (scripts/rotate-hsm-key.sh) - it is not
# triggerable via any HTTP endpoint by design. The Given step below moves
# the active key-version pointer (the 'pki_active_key_version' settings
# table, backend/migrations/sql/20260709b_pki_key_versioning.sql) directly
# via context.db; the Then assertions read the 'key_version' field from
# GET /signature/retrieve/{did}. The requirement-accurate claim under test
# is "old and new signatures are distinguishable by key version, and the
# old one keeps validating".
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
            "Could not seed the key-rotation test seam (the "
            f"'pki_active_key_version' settings table): {exc}"
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
        f"trust store), got findings suggesting "
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
            f"label/version used (old and new signatures must be "
            f"distinguishable by key label/version) - no such field found in: {body}"
        )
        versions[name] = version
    assert versions[old_name] != versions[new_name], (
        f"Expected '{old_name}' (signed before rotation) and '{new_name}' (signed after "
        f"rotation) to be attributed to different HSM key versions, got the same value "
        f"'{versions[old_name]}' for both"
    )
