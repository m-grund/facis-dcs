"""BDD steps for the OID4VP Document-Retrieval signing ceremony (ADR-12,
features/22_real_signing_vertical/oid4vp_document_retrieval.feature).

The DCS publishes a STANDARD OID4VP Document-Retrieval request object for a
PID-verified ceremony; the harness plays the wallet+QTSP stand-in, consuming the
published QR exactly as a real EUDI wallet would — fetch the request object,
fetch the to-be-signed document, sign it with the signatory's own key via the
external EU DSS SCA, and post the signed document back to the response_uri
callback, where the DCS reuses the /signature/submit validate + finalize path.

The APPROVED + completed-ceremony precondition (and the ceremony id it stashes on
context.ceremony_ids) is reused from dcs_real_signing_vertical_steps.py.
"""

from __future__ import annotations

import os

import requests as _requests
from behave import then, when

from steps.support.api_client import (
    origin_url,
    post_json,
    signature_request_leaf_url,
    signature_request_publish_url,
    signature_view_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


def _harness_request_uri(context, ceremony_id: str) -> str:
    """The published request_uri, but on the origin the harness actually routes
    to (context.base_url). The DCS builds the request object's URLs from its
    advertised public base, which need not equal the harness route."""
    return signature_request_leaf_url(context, ceremony_id, "object")


@when('the signer publishes the OID4VP signing request for contract "{name}"')
def step_when_publish_signing_request(context, name):
    ceremony_id = (getattr(context, "ceremony_ids", {}) or {}).get(name)
    assert ceremony_id, f"No completed ceremony recorded for contract {name!r}"
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    resp = post_json(
        context,
        signature_request_publish_url(context, ceremony_id),
        {"ceremony_id": ceremony_id, "credential_type": "AES"},
        headers=signer_h,
    )
    context.requests_response = resp
    if not hasattr(context, "publish_responses"):
        context.publish_responses = {}
    if resp.status_code == 200:
        context.publish_responses[name] = resp.json()


@then("the publish response carries a client_id, request_uri, and expires_at")
def step_then_publish_response_fields(context):
    body = context.requests_response.json()
    for key in ("client_id", "request_uri", "expires_at", "wallet_uri"):
        assert body.get(key), f"publish response missing {key!r}: {body}"


@then(
    "the published request object is a signed JAR carrying document_digests, "
    "document_locations, response_uri, and a nonce"
)
def step_then_request_object_is_jar(context, name=None):
    # Resolve the one published contract in this scenario.
    published = getattr(context, "publish_responses", {}) or {}
    assert published, "no signing request was published"
    contract_name = name or next(iter(published))
    ceremony_id = context.ceremony_ids[contract_name]

    resp = _requests.get(
        _harness_request_uri(context, ceremony_id),
        headers={"Accept": "application/oauth-authz-req+jwt"},
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"request object fetch failed: {resp.status_code} {resp.text}"

    compact = resp.text.strip()
    parts = compact.split(".")
    assert len(parts) == 3 and all(parts), f"request object is not a signed compact JWS: {compact[:60]!r}"

    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.oid4vp_signing import _decode_jwt_claims  # noqa: PLC0415

    claims = _decode_jwt_claims(compact)
    assert claims.get("document_digests"), f"no document_digests in request object: {claims}"
    assert claims["document_digests"][0].get("hash"), f"document digest has no hash: {claims}"
    assert claims.get("document_locations"), f"no document_locations in request object: {claims}"
    assert claims.get("response_uri"), f"no response_uri in request object: {claims}"
    assert claims.get("nonce"), f"no nonce in request object: {claims}"
    assert claims.get("client_id_scheme") == "x509_san_dns", (
        f"request object must use client_id_scheme x509_san_dns: {claims}"
    )


@when('the wallet signs contract "{name}" by consuming the OID4VP signing request as "{signatory}"')
def step_when_wallet_signs_via_qr(context, name, signatory):
    ceremony_id = context.ceremony_ids[name]
    request_uri = _harness_request_uri(context, ceremony_id)

    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.oid4vp_signing import sign_via_document_retrieval  # noqa: PLC0415

    dss_url = os.getenv("BDD_DSS_URL", "http://localhost:18099")
    context.wallet_callback_response = sign_via_document_retrieval(
        request_uri=request_uri,
        user=signatory,
        dss_url=dss_url,
        keys_dir=AuthService.resolve_wallet_keys_dir(),
    )
    ContractService._refresh_contract(context, name)


@then('the wallet callback reports the contract "{name}" as SIGNED')
def step_then_callback_reports_signed(context, name):
    resp = getattr(context, "wallet_callback_response", None)
    assert resp, "no wallet callback response captured"
    assert resp.get("status") == "SIGNED", f"expected callback status SIGNED, got: {resp}"


@then('the signature view for contract "{name}" shows a "{status}" signature for field "{field}"')
def step_then_view_single_signature(context, name, status, field):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = _requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"signature view failed: {resp.status_code} {resp.text}"
    signatures = resp.json().get("signatures") or []
    match = [s for s in signatures if s.get("field_name") == field]
    assert match, f"no signature covering field {field!r}, got: {signatures}"
    sig = match[0]
    assert sig.get("status") == status, f"expected {field!r} to be {status!r}, got: {sig.get('status')!r}"
    assert sig.get("signer_did"), f"expected a signer identity on {field!r}: {sig}"
