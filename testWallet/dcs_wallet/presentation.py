from __future__ import annotations

import base64
import hashlib
import json
import time
from pathlib import Path
from typing import Any

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.credential import load_credential_claims, load_credential_jwt

VP_FORMAT = "dc+sd-jwt"


def _wallet_root() -> Path:
    return Path(__file__).resolve().parent.parent


def load_jwk(filename: str) -> dict[str, Any]:
    path = _wallet_root() / "keys" / filename
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_dev_keys.py"
        )
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def _sd_hash(presentation: str) -> str:
    digest = hashlib.sha256(presentation.encode()).digest()
    return base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def build_vp_token(*, credential_name: str, nonce: str, client_id: str = "") -> str:
    credential_jwt = load_credential_jwt(credential_name)
    credential_claims = load_credential_claims(credential_name)
    wallet_jwk = load_jwk("wallet.jwk")

    holder_did = str(credential_claims["sub"])
    now = int(time.time())

    presentation = credential_jwt
    kb_claims = {
        "sub": holder_did,
        "nonce": nonce,
        "iat": now,
        "sd_hash": _sd_hash(presentation),
    }
    if client_id:
        kb_claims["aud"] = client_id
    kb_jwt = jwt.encode(
        kb_claims,
        ECAlgorithm.from_jwk(json.dumps(wallet_jwk)),
        algorithm="ES256",
        headers={"kid": wallet_jwk.get("kid", "")},
    )

    return f"{presentation}~{kb_jwt}"
