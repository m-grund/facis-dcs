#!/usr/bin/env python3
"""Issue PID credentials from *.pid.template.json via online EUDIPLO API.

Outputs one file per template:
  <stem>.pid.template.json -> <stem>.pid.jwt
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path

import jwt
from cryptography.hazmat.primitives.asymmetric import ec
from jwt.algorithms import ECAlgorithm

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.keys import load_json, private_key_material, public_key_material

DEFAULT_CREDENTIALS_DIR = WALLET_ROOT / "credentials"
ISSUE_URL = "https://playground.eudi-wallet.org/api/issue"


def _fetch_json(url: str, *, method: str = "GET", body: bytes | None = None, headers: dict | None = None) -> dict:
    req = urllib.request.Request(url, data=body, headers=headers or {}, method=method)
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _decode_credential_offer_uri(uri: str) -> dict:
    parsed = urllib.parse.urlparse(uri)
    query = urllib.parse.parse_qs(parsed.query)
    raw_offer = query.get("credential_offer", [None])[0]
    if not raw_offer:
        raise ValueError("credential_offer missing in URI")
    return json.loads(raw_offer)


def _issuer_well_known_url(issuer_url: str, kind: str) -> str:
    parsed = urllib.parse.urlparse(issuer_url)
    issuer_path = parsed.path.lstrip("/")
    if not parsed.scheme or not parsed.netloc or not issuer_path:
        raise ValueError(f"invalid credential_issuer URL: {issuer_url}")
    return f"{parsed.scheme}://{parsed.netloc}/.well-known/{kind}/{issuer_path}"


def _b64u_int_32(value: int) -> str:
    return base64.urlsafe_b64encode(value.to_bytes(32, "big")).rstrip(b"=").decode("ascii")


def _build_proof_jwt(*, issuer_url: str, nonce: str, wallet_private_jwk: dict, wallet_public_jwk: dict) -> str:
    jwk = {
        "kty": wallet_public_jwk["kty"],
        "crv": wallet_public_jwk["crv"],
        "x": wallet_public_jwk["x"],
        "y": wallet_public_jwk["y"],
    }
    payload = {"iss": "testWallet-holder", "aud": issuer_url, "iat": int(time.time()), "nonce": nonce}
    headers = {"typ": "openid4vci-proof+jwt", "alg": "ES256", "jwk": jwk}
    return jwt.encode(payload, ECAlgorithm.from_jwk(json.dumps(wallet_private_jwk)), algorithm="ES256", headers=headers)


def _extract_credential_jwt(response: dict) -> str:
    if isinstance(response.get("credential"), str):
        return response["credential"].strip()
    credentials = response.get("credentials")
    if isinstance(credentials, list) and credentials:
        first = credentials[0]
        if isinstance(first, str):
            return first.strip()
        if isinstance(first, dict) and isinstance(first.get("credential"), str):
            return first["credential"].strip()
    raise ValueError(f"credential not found in issuer response: keys={list(response.keys())}")


def issue_from_template(template_payload: dict, *, wallet_private_jwk: dict, wallet_public_jwk: dict) -> str:
    credential_id = str(template_payload.get("credentialId") or "").strip()
    claims = template_payload.get("claims")
    if not credential_id:
        raise ValueError("template requires credentialId")
    if not isinstance(claims, dict):
        raise ValueError("template requires claims object")

    issue_data = _fetch_json(
        ISSUE_URL,
        method="POST",
        body=json.dumps({"credentialId": credential_id, "claims": claims}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    offer_uri = str(issue_data.get("uri") or "").strip()
    if not offer_uri:
        raise ValueError(f"issue response missing uri: {issue_data}")
    offer = _decode_credential_offer_uri(offer_uri)

    issuer_url = str(offer.get("credential_issuer") or "").strip()
    pre_auth = (
        offer.get("grants", {})
        .get("urn:ietf:params:oauth:grant-type:pre-authorized_code", {})
        .get("pre-authorized_code")
    )
    config_ids = offer.get("credential_configuration_ids") or []
    if not issuer_url or not isinstance(pre_auth, str) or not pre_auth or not config_ids:
        raise ValueError("credential offer missing required fields")
    config_id = str(config_ids[0])

    issuer_meta = _fetch_json(_issuer_well_known_url(issuer_url, "openid-credential-issuer"))
    auth_meta = _fetch_json(_issuer_well_known_url(issuer_url, "oauth-authorization-server"))
    token_endpoint = str(auth_meta.get("token_endpoint") or "").strip()
    nonce_endpoint = str(issuer_meta.get("nonce_endpoint") or "").strip()
    credential_endpoint = str(issuer_meta.get("credential_endpoint") or "").strip()
    if not token_endpoint or not nonce_endpoint or not credential_endpoint:
        raise ValueError("issuer metadata missing token/nonce/credential endpoint")

    token_form = urllib.parse.urlencode(
        {
            "grant_type": "urn:ietf:params:oauth:grant-type:pre-authorized_code",
            "pre-authorized_code": pre_auth,
        }
    ).encode("utf-8")
    token_data = _fetch_json(
        token_endpoint,
        method="POST",
        body=token_form,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    access_token = str(token_data.get("access_token") or "").strip()
    if not access_token:
        raise ValueError("token response missing access_token")

    nonce_data = _fetch_json(
        nonce_endpoint,
        method="POST",
        headers={"Authorization": f"Bearer {access_token}"},
    )
    c_nonce = str(nonce_data.get("c_nonce") or "").strip()
    if not c_nonce:
        raise ValueError("nonce response missing c_nonce")

    cfg = (issuer_meta.get("credential_configurations_supported") or {}).get(config_id) or {}
    vct = cfg.get("vct")
    if not isinstance(vct, str) or not vct:
        raise ValueError(f"issuer config {config_id!r} has no vct")

    proof_jwt = _build_proof_jwt(
        issuer_url=issuer_url,
        nonce=c_nonce,
        wallet_private_jwk=wallet_private_jwk,
        wallet_public_jwk=wallet_public_jwk,
    )
    credential_request = {
        "format": "dc+sd-jwt",
        "vct": vct,
        "proof": {"proof_type": "jwt", "jwt": proof_jwt},
    }
    credential_data = _fetch_json(
        credential_endpoint,
        method="POST",
        body=json.dumps(credential_request).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {access_token}"},
    )
    return _extract_credential_jwt(credential_data)


def _template_stem(path: Path) -> str:
    suffix = ".pid.template.json"
    if not path.name.endswith(suffix):
        raise ValueError(f"unexpected template name: {path.name}")
    return path.name[: -len(suffix)]


def issue_pid_credentials(
    *,
    credentials_dir: Path,
    wallet_private_jwk: dict,
    wallet_public_jwk: dict | None = None,
    credential_names: list[str] | None = None,
) -> list[Path]:
    public_jwk = wallet_public_jwk or public_key_material(wallet_private_jwk)

    if credential_names:
        templates = [credentials_dir / f"{name}.pid.template.json" for name in credential_names]
    else:
        templates = sorted(credentials_dir.glob("*.pid.template.json"))

    if not templates:
        raise FileNotFoundError(f"no *.pid.template.json files found in {credentials_dir}")

    output_paths: list[Path] = []
    for template_path in templates:
        if not template_path.is_file():
            raise FileNotFoundError(f"template not found: {template_path}")
        payload = json.loads(template_path.read_text(encoding="utf-8"))
        jwt_value = issue_from_template(
            payload,
            wallet_private_jwk=wallet_private_jwk,
            wallet_public_jwk=public_jwk,
        )
        stem = _template_stem(template_path)
        out_path = credentials_dir / f"{stem}.pid.jwt"
        out_path.write_text(jwt_value + "\n", encoding="utf-8")
        output_paths.append(out_path)
    return output_paths


def main() -> int:
    parser = argparse.ArgumentParser(description="Issue *.pid.jwt from *.pid.template.json")
    parser.add_argument("--credentials-dir", type=Path, default=DEFAULT_CREDENTIALS_DIR)
    parser.add_argument("--credential", action="append", help="base credential stem to issue, e.g. johndoe")
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    args = parser.parse_args()

    wallet_private_jwk = private_key_material(load_json(args.keys_dir / "wallet.jwk"))
    for path in issue_pid_credentials(
        credentials_dir=args.credentials_dir,
        wallet_private_jwk=wallet_private_jwk,
        credential_names=args.credential,
    ):
        print(f"issued: {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
