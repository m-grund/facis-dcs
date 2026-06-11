from __future__ import annotations

import base64
import json
from pathlib import Path
from typing import Any

CREDENTIAL_EXT = ".jwt"


def _wallet_root() -> Path:
    return Path(__file__).resolve().parent.parent


def credentials_dir() -> Path:
    return _wallet_root() / "credentials"


def credential_path(stem: str) -> Path:
    name = stem.removesuffix(CREDENTIAL_EXT)
    return credentials_dir() / f"{name}{CREDENTIAL_EXT}"


def decode_jwt_payload(token: str) -> dict[str, Any]:
    parts = token.strip().split(".")
    if len(parts) < 2:
        raise ValueError("credential is not a compact JWT")
    payload = parts[1]
    padding = "=" * (-len(payload) % 4)
    raw = json.loads(base64.urlsafe_b64decode(payload + padding).decode())
    if not isinstance(raw, dict):
        raise ValueError("credential JWT payload is not an object")
    return raw


def load_credential_jwt(name: str) -> str:
    path = credential_path(name)
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_dev_keys.py"
        )
    token = path.read_text(encoding="utf-8").strip()
    if not token.startswith("eyJ"):
        raise ValueError(f"{path} must contain a single-line compact JWT")
    return token


def load_credential_claims(name: str) -> dict[str, Any]:
    return decode_jwt_payload(load_credential_jwt(name))
