from __future__ import annotations

import base64
import hashlib
import json
import os
import time
from pathlib import Path
from typing import Any

import jwt
from jwt.algorithms import ECAlgorithm

VP_FORMAT = "dc+sd-jwt"


def _wallet_root() -> Path:
    return Path(__file__).resolve().parent.parent


def _keys_dir() -> Path:
    override = os.environ.get("DCS_WALLET_KEYS_DIR", "").strip()
    if override:
        return Path(override)
    return _wallet_root() / "keys"


def load_jwk(filename: str) -> dict[str, Any]:
    path = _keys_dir() / filename
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_dev_keys.py"
        )
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def load_credential(name: str) -> dict[str, Any]:
    stem = name.removesuffix(".json")
    path = _wallet_root() / "credentials" / f"{stem}.json"
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_dev_keys.py "
            f"(needs testWallet/credentials/{stem}.template.json)"
        )
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def build_vp_token(*, credential_name: str, nonce: str, client_id: str = "") -> str:
    credential_claims = load_credential(credential_name)
    issuer_name = os.environ.get("DCS_ISSUER_KEY_FILE", "issuer-dev.jwk")
    wallet_name = os.environ.get("DCS_WALLET_KEY_FILE", "wallet.jwk")
    issuer_jwk = load_jwk(issuer_name)
    wallet_jwk = load_jwk(wallet_name)

    holder_did = str(credential_claims["sub"])
    now = int(time.time())
    claims = dict(credential_claims)
    claims["iat"] = now

    credential_jwt = jwt.encode(
        claims,
        ECAlgorithm.from_jwk(json.dumps(issuer_jwk)),
        algorithm="ES256",
        headers={"kid": issuer_jwk.get("kid", "")},
    )

    sd_hash = base64.urlsafe_b64encode(hashlib.sha256(credential_jwt.encode()).digest()).rstrip(b"=").decode()
    kb_claims = {
        "sub": holder_did,
        "nonce": nonce,
        "iat": now,
        "sd_hash": sd_hash,
    }
    if client_id:
        kb_claims["aud"] = client_id
    kb_jwt = jwt.encode(
        kb_claims,
        ECAlgorithm.from_jwk(json.dumps(wallet_jwk)),
        algorithm="ES256",
        headers={"kid": wallet_jwk.get("kid", "")},
    )

    envelope = {
        "format": VP_FORMAT,
        "credential_jwt": credential_jwt,
        "kb_jwt": kb_jwt,
    }
    return json.dumps(envelope, separators=(",", ":"))
