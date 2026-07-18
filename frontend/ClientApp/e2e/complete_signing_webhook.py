"""Wallet leg of the signing ceremony: builds the holder-bound PoA SD-JWT VC +
KB-JWT presentation the way the real wallet does and delivers it over the
wallet's own webhook channel (the EUDIPLO-test-client role). The same PoA the
signer authenticated with at login authorizes the signature — no separate PID.
Self-contained — it uses the same testWallet/dcs_wallet signing primitives
AuthService uses for the OID4VP login, without importing the behave step modules
(which pull in the bdd-executor runtime).

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

from steps.support.services.auth_service import AuthService  # noqa: E402

WEBHOOK_SECRET_HEADER = "X-EUDIPLO-Webhook-Secret"


def build_poa_presentation(*, organization: str, roles: list[str], aud: str, nonce: str):
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
            "vct": "urn:dcs:poa:v1",
            "iat": now - 3600,
            "exp": now + 3600,
            "cnf": {"jwk": cnf_jwk(holder_public)},
        },
        selective_claims={"organization": organization, "roles": roles},
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
    organization, roles = "E2E Vertical Signer", ["Contract Signer"]
    presentation, subject_did = build_poa_presentation(
        organization=organization,
        roles=roles,
        aud="dcs-signature-ceremony",
        nonce=str(uuid.uuid4()),
    )
    response = requests.post(
        f"{base_url}/signature/request/webhook",
        json={
            "ceremony_id": ceremony_id,
            "vp_token": presentation,
            "poa_claims": {"sub": subject_did, "organization": organization, "roles": roles},
        },
        headers={WEBHOOK_SECRET_HEADER: os.getenv("BDD_EUDIPLO_WEBHOOK_SECRET", "bdd-eudiplo-webhook-secret")},
        timeout=60,
    )
    response.raise_for_status()
    print(response.status_code, response.text[:500])


if __name__ == "__main__":
    main()
