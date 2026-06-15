from __future__ import annotations

import base64
import json
import time
from pathlib import Path
from typing import Any

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, private_key_material, public_key_material, public_jwk, write_text
from dcs_wallet.sdjwt import KB_JWT_TYP, DEFAULT_SD_ALG, create_property_disclosure, join_sd_jwt, sd_hash, split_sd_jwt

POA_VCT = "urn:dcs:poa:v1"
CREDENTIAL_JWT_TYP = "dc+sd-jwt"
DEFAULT_ISSUER_DID = "did:web:dev.example:issuer:poa"
TRUSTED_ISSUER_DIDS = [
    "did:web:dev.example:issuer:poa",
    "did:web:dev2.example:issuer:poa",
]
CREDENTIAL_EXT = ".jwt"
CREDENTIAL_IAT = 1719129600
CREDENTIAL_EXP = 1893456000
DEFAULT_KB_AUD = "dcs-client"
DEFAULT_KB_NONCE = "test-nonce"


def _jwt_private_key(jwk: dict[str, Any]) -> Any:
    return ECAlgorithm.from_jwk(json.dumps(private_key_material(jwk)))


def _decode_jwt_payload_unverified(token: str) -> dict[str, Any]:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    data = json.loads(base64.urlsafe_b64decode(payload).decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError("JWT payload must be a JSON object")
    return data


def sign_credential_sd_jwt(
    *,
    visible_claims: dict[str, Any],
    selective_claims: dict[str, Any],
    issuer_private: dict[str, Any],
) -> str:
    disclosures: list[str] = []
    sd_digests: list[str] = []
    for claim_name, claim_value in selective_claims.items():
        encoded, digest = create_property_disclosure(claim_name, claim_value)
        disclosures.append(encoded)
        sd_digests.append(digest)

    payload = {**visible_claims, "_sd": sd_digests, "_sd_alg": DEFAULT_SD_ALG}
    issuer_jwt = jwt.encode(
        payload,
        _jwt_private_key(issuer_private),
        algorithm="ES256",
        headers={
            "typ": CREDENTIAL_JWT_TYP,
            "alg": "ES256",
            "jwk": public_key_material(issuer_private),
        },
    )
    return join_sd_jwt(issuer_jwt, disclosures)


def sign_key_binding_jwt(
    *,
    issuer_jwt: str,
    disclosures: list[str],
    wallet_private: dict[str, Any],
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
    sd_alg: str = DEFAULT_SD_ALG,
) -> str:
    if not aud:
        raise ValueError("aud is required for KB-JWT")
    if not nonce:
        raise ValueError("nonce is required for KB-JWT")

    kb_claims = {
        "iat": int(time.time()),
        "aud": aud,
        "nonce": nonce,
        "sd_hash": sd_hash(issuer_jwt, disclosures, sd_alg=sd_alg),
    }
    return jwt.encode(
        kb_claims,
        _jwt_private_key(wallet_private),
        algorithm="ES256",
        headers={"typ": KB_JWT_TYP, "alg": "ES256"},
    )


def attach_key_binding(
    *,
    issued_sd_jwt: str,
    wallet_private: dict[str, Any],
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> str:
    issuer_jwt, disclosures, _old_kb = split_sd_jwt(issued_sd_jwt)
    issuer_payload = _decode_jwt_payload_unverified(issuer_jwt)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        wallet_private=wallet_private,
        aud=aud,
        nonce=nonce,
        sd_alg=str(issuer_payload.get("_sd_alg") or DEFAULT_SD_ALG),
    )
    return join_sd_jwt(issuer_jwt, disclosures, kb_jwt)


def issue_access_credential(
    *,
    organization: str,
    roles: list[str],
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> str:
    holder_public = public_jwk(wallet_private)
    holder_did = did_jwk_from_public_jwk(holder_public)
    visible_claims = {
        "iss": issuer_did,
        "sub": holder_did,
        "vct": POA_VCT,
        "iat": CREDENTIAL_IAT,
        "exp": CREDENTIAL_EXP,
        "cnf": {"jwk": cnf_jwk(holder_public)},
    }
    selective_claims = {
        "organization": organization,
        "roles": roles,
    }
    issued_sd_jwt = sign_credential_sd_jwt(
        visible_claims=visible_claims,
        selective_claims=selective_claims,
        issuer_private=issuer_private,
    )
    return attach_key_binding(
        issued_sd_jwt=issued_sd_jwt,
        wallet_private=wallet_private,
        aud=aud,
        nonce=nonce,
    )


def issue_credential_from_template(
    *,
    template_path: Path,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> str:
    with template_path.open(encoding="utf-8") as fh:
        template_data = json.load(fh)
    organization = template_data.get("organization")
    roles = template_data.get("roles")
    if not isinstance(organization, str) or not organization:
        raise ValueError(f"{template_path} must contain a non-empty organization")
    if not isinstance(roles, list) or not all(isinstance(role, str) for role in roles):
        raise ValueError(f"{template_path} must contain roles as a list of strings")
    return issue_access_credential(
        organization=organization,
        roles=roles,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
        aud=aud,
        nonce=nonce,
    )


def issue_credential_file(
    *,
    credentials_dir: Path,
    credential_name: str,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> Path:
    stem = credential_name.removesuffix(CREDENTIAL_EXT).removesuffix(".template")
    template_path = credentials_dir / f"{stem}.template.json"
    if not template_path.is_file():
        raise FileNotFoundError(f"template not found: {template_path}")
    token = issue_credential_from_template(
        template_path=template_path,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
        aud=aud,
        nonce=nonce,
    )
    output_path = credentials_dir / f"{stem}{CREDENTIAL_EXT}"
    write_text(output_path, token)
    return output_path


def issue_all_template_files(
    *,
    credentials_dir: Path,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> list[Path]:
    paths: list[Path] = []
    for template_path in sorted(credentials_dir.glob("*.template.json")):
        stem = template_path.name.replace(".template.json", "")
        paths.append(
            issue_credential_file(
                credentials_dir=credentials_dir,
                credential_name=stem,
                issuer_private=issuer_private,
                wallet_private=wallet_private,
                issuer_did=issuer_did,
                aud=aud,
                nonce=nonce,
            )
        )
    return paths
