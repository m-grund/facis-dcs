"""Wallet leg of the signing ceremony: builds the PID SD-JWT VC + KB-JWT
presentation the way the real wallet does and delivers it over the wallet's
own webhook channel (the EUDIPLO-test-client role). Self-contained — it uses
the same testWallet/dcs_wallet signing primitives AuthService uses for the
OID4VP login, without importing the behave step modules (which pull in the
bdd-executor runtime).

Usage: python3 complete_signing_webhook.py <ceremony_id>
Env:   STATUSLIST_SERVICE_URL, BDD_DCS_BASE_URL
"""

import os
import sys
import time
import uuid

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
sys.path.insert(0, REPO_ROOT)

from steps.support import localhost_resolver  # noqa: E402

localhost_resolver.install()

import requests  # noqa: E402

from steps.support.api_client import did_document_url  # noqa: E402
from steps.support.services.auth_service import AuthService  # noqa: E402

WEBHOOK_SECRET_HEADER = "X-EUDIPLO-Webhook-Secret"


def build_pid_presentation(*, given_name: str, family_name: str, aud: str, nonce: str):
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.issuer import DEFAULT_ISSUER_DID, sign_credential_sd_jwt, sign_key_binding_jwt
    from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, public_jwk
    from dcs_wallet.sdjwt import join_sd_jwt, split_sd_jwt

    keys = AuthService.load_wallet_keys()
    holder_key = keys.wallet_private
    holder_public = public_jwk(holder_key)
    subject_did = did_jwk_from_public_jwk(holder_public)

    now = int(time.time())
    issued = sign_credential_sd_jwt(
        visible_claims={
            "iss": DEFAULT_ISSUER_DID,
            "sub": subject_did,
            "vct": "urn:eudi:pid:1",
            "iat": now - 3600,
            "exp": now + 3600,
            "cnf": {"jwk": cnf_jwk(holder_public)},
        },
        selective_claims={"given_name": given_name, "family_name": family_name},
        issuer_private=keys.issuer_private,
    )
    issuer_jwt, disclosures, _ = split_sd_jwt(issued)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        wallet_private=holder_key,
        aud=aud,
        nonce=nonce,
    )
    return join_sd_jwt(issuer_jwt, disclosures, kb_jwt), subject_did


def main() -> None:
    ceremony_id = sys.argv[1]
    base_url = os.environ["BDD_DCS_BASE_URL"].rstrip("/")
    # The organization the wallet's Power of Attorney authorizes it to act for.
    # One org runs one DCS, so that organization is the signing org's own DID,
    # resolved from its did:web document — the same public DID trust anchor the
    # testWallet self-issues under and every peer resolves against.
    poa_organization = requests.get(did_document_url(base_url), timeout=30).json()["id"]
    given_name, family_name = "E2E Vertical Signer", "E2E-Testperson"
    presentation, subject_did = build_pid_presentation(
        given_name=given_name,
        family_name=family_name,
        aud="dcs-signature-ceremony",
        nonce=str(uuid.uuid4()),
    )
    response = requests.post(
        f"{base_url}/signature/request/webhook",
        json={
            "ceremony_id": ceremony_id,
            "vp_token": presentation,
            "pid_claims": {"sub": subject_did, "given_name": given_name, "family_name": family_name},
            "poa_organization": poa_organization,
        },
        headers={WEBHOOK_SECRET_HEADER: os.getenv("BDD_EUDIPLO_WEBHOOK_SECRET", "bdd-eudiplo-webhook-secret")},
        timeout=60,
    )
    response.raise_for_status()
    print(response.status_code, response.text[:500])


if __name__ == "__main__":
    main()
