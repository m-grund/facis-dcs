from __future__ import annotations

import base64
import json
import os
from pathlib import Path
from typing import Any

from cryptography.hazmat.primitives.asymmetric import ec

REQUIRED_EC_PUBLIC_FIELDS = ("kty", "crv", "x", "y")


def wallet_root() -> Path:
    return Path(__file__).resolve().parent.parent


def b64url_uint(value: int) -> str:
    length = (value.bit_length() + 7) // 8 or 1
    raw = value.to_bytes(length, "big")
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def generate_ec_private_jwk() -> dict[str, Any]:
    private_key = ec.generate_private_key(ec.SECP256R1())
    numbers = private_key.private_numbers()
    public = numbers.public_numbers
    return {
        "kty": "EC",
        "crv": "P-256",
        "x": b64url_uint(public.x),
        "y": b64url_uint(public.y),
        "d": b64url_uint(numbers.private_value),
    }


def public_key_material(jwk: dict[str, Any]) -> dict[str, Any]:
    missing = [field for field in REQUIRED_EC_PUBLIC_FIELDS if not jwk.get(field)]
    if missing:
        raise ValueError(f"incomplete EC JWK: missing {', '.join(missing)}")
    return {field: str(jwk[field]) for field in REQUIRED_EC_PUBLIC_FIELDS}


def private_key_material(jwk: dict[str, Any]) -> dict[str, Any]:
    public = public_key_material(jwk)
    if not jwk.get("d"):
        raise ValueError("incomplete EC private JWK: missing d")
    return {**public, "d": str(jwk["d"])}


def public_jwk(private_or_public_jwk: dict[str, Any]) -> dict[str, Any]:
    return public_key_material(private_or_public_jwk)


def cnf_jwk(public_key: dict[str, Any]) -> dict[str, Any]:
    return public_key_material(public_key)


def did_jwk_from_public_jwk(key: dict[str, Any]) -> str:
    public = public_key_material(key)
    payload = json.dumps(
        {
            "crv": public["crv"],
            "kty": public["kty"],
            "x": public["x"],
            "y": public["y"],
        },
        separators=(",", ":"),
    ).encode("utf-8")
    return "did:jwk:" + base64.urlsafe_b64encode(payload).rstrip(b"=").decode("ascii")


def build_trust_json(*, issuer_public: dict[str, Any], issuer_dids: list[str], vcts: list[str]) -> dict[str, Any]:
    issuer_key = public_key_material(issuer_public)
    issuers: dict[str, Any] = {}
    for did in issuer_dids:
        issuers[did] = {"jwks": {"keys": [issuer_key]}}
    return {"vcts": list(dict.fromkeys(vcts)), "issuers": issuers}


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return data


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump(data, fh, indent=2)
        fh.write("\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        fh.write(content.rstrip() + "\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
