"""Wallet-driven signing for the BDD harness (ADR-12).

The DCS holds no signing key: it prepares the to-be-signed PDF, and validates +
records whatever the signatory signs. This helper plays the signatory (the test
wallet + QTSP): it fetches the prepared PDF from /signature/prepare, signs it
with the signatory's own key via the external SCA (a real EU DSS), and submits
it to /signature/submit. It replaces the removed /signature/apply.

BDD_DSS_URL must point at a DSS reachable from the harness (the deploy enables
the dss chart); it defaults to the local dev DSS.
"""

from __future__ import annotations

import base64
import os

from steps.support.api_client import post_json
from steps.support.services.auth_service import AuthService


def wallet_sign(
    context,
    did,
    *,
    signer_did,
    signatory,
    field_name=None,
    credential_type="AES",
    base_url=None,
    headers=None,
):
    """Run prepare -> sign -> submit and return the final HTTP response.

    signatory is the natural person who signs: the wallet key identity and the
    signing certificate subject ("CN=DCS Signatory <signatory>"). The AES sole-
    control gate derives the expected person from the ceremony's verified PID (its
    given_name matches this name), not from the caller. It is NOT the signature
    field: the field is the participating party's DCS instance DID
    (dcs:signatoryName, see seedSignatureFields), so — unless a caller pins
    field_name for a multi-signer contract — we sign whichever field the prepared
    PDF carries (field=""). A precondition failure (no completed ceremony, contract
    not APPROVED) is returned straight from /signature/prepare.
    """
    base = (base_url or context.base_url).rstrip("/")
    signer_headers = headers or AuthService.get_headers_for_roles(["Contract Signer"])
    body = {"did": did, "signer_did": signer_did, "credential_type": credential_type}
    if field_name is not None:
        body["field_name"] = field_name

    prepare_resp = post_json(context, f"{base}/signature/prepare", body, headers=signer_headers)
    if prepare_resp.status_code != 200:
        return prepare_resp

    field = field_name or ""
    dss_url = os.getenv("BDD_DSS_URL", "http://localhost:18099")
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.remote_signer import sign_pdf  # noqa: PLC0415

    signed_pdf = sign_pdf(
        base64.b64decode(prepare_resp.json()["document"]),
        user=signatory,
        dss_url=dss_url,
        field=field,
        keys_dir=AuthService.resolve_wallet_keys_dir(),
    )

    submit_body = dict(
        body,
        signed_pdf=base64.b64encode(signed_pdf).decode(),
        jades_signature="",
    )
    return post_json(context, f"{base}/signature/submit", submit_body, headers=signer_headers)
