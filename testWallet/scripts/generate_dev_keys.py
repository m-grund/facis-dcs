#!/usr/bin/env python3
"""
Generate demo OID4VP materials for testWallet + DCS backend.

Creates:
  - testWallet/keys/issuer-dev.jwk
  - testWallet/keys/wallet.jwk
  - backend/config/oid4vp/trust.dev.json (issuer public JWKS for all trusted DIDs)
  - testWallet/credentials/*.jwt from *.template.json (issuer-signed SD-JWT, one line each)

Optional Vault sync (KV v2):
  secret/dcs/demo/issuer-dev-jwk
  secret/dcs/demo/wallet-jwk
  secret/dcs/demo/trust-json

Usage:
  python3 testWallet/scripts/generate_dev_keys.py
  python3 testWallet/scripts/generate_dev_keys.py --yes
  python3 testWallet/scripts/generate_dev_keys.py --regenerate-keys --yes
  VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=root python3 testWallet/scripts/generate_dev_keys.py --vault-write
  python3 testWallet/scripts/generate_dev_keys.py --vault-read

Flags:
  --yes              Skip the confirmation prompt when local material already exists.
                     trust.json and credentials/*.json are still rewritten. Keeps
                     existing issuer-dev.jwk and wallet.jwk unless --regenerate-keys.

  --regenerate-keys  Replace issuer-dev.jwk and wallet.jwk with newly generated pairs
                     even when both files already exist. Changes holder sub (did:jwk);
                     demo logins and credentials must match the new wallet key.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

import jwt
from cryptography.hazmat.primitives.asymmetric import ec
from jwt.algorithms import ECAlgorithm

POA_VCT = "urn:dcs:poa:v1"
CREDENTIAL_JWT_TYP = "dc+sd-jwt"
DEFAULT_ISSUER_DID = "did:web:dev.example:issuer:poa"
TRUSTED_ISSUER_DIDS = [
    "did:web:dev.example:issuer:poa",
    "did:web:dev2.example:issuer:poa",
]
ISSUER_KID = "issuer-dev"
WALLET_KID = "wallet"
CREDENTIAL_EXT = ".jwt"
VAULT_ISSUER_PATH = "dcs/demo/issuer-dev-jwk"
VAULT_WALLET_PATH = "dcs/demo/wallet-jwk"
VAULT_TRUST_PATH = "dcs/demo/trust-json"
CREDENTIAL_IAT = 1719129600
CREDENTIAL_EXP = 1893456000


def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent.parent


def b64url_uint(value: int) -> str:
    length = (value.bit_length() + 7) // 8 or 1
    raw = value.to_bytes(length, "big")
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def ec_private_jwk(private_key: ec.EllipticCurvePrivateKey, *, kid: str) -> dict[str, Any]:
    numbers = private_key.private_numbers()
    public = numbers.public_numbers
    return {
        "kty": "EC",
        "crv": "P-256",
        "x": b64url_uint(public.x),
        "y": b64url_uint(public.y),
        "d": b64url_uint(numbers.private_value),
        "kid": kid,
        "use": "sig",
        "alg": "ES256",
    }


def public_jwk(private_jwk: dict[str, Any]) -> dict[str, Any]:
    return {k: v for k, v in private_jwk.items() if k != "d"}


def did_jwk_from_public_jwk(key: dict[str, Any]) -> str:
    payload = json.dumps(
        {"crv": key["crv"], "kty": key["kty"], "x": key["x"], "y": key["y"]},
        separators=(",", ":"),
    ).encode()
    return "did:jwk:" + base64.urlsafe_b64encode(payload).rstrip(b"=").decode()


def build_trust_json(*, issuer_public: dict[str, Any], issuer_dids: list[str] | None = None) -> dict[str, Any]:
    dids = issuer_dids or TRUSTED_ISSUER_DIDS
    issuers: dict[str, Any] = {}
    for did in dids:
        issuers[did] = {"jwks": {"keys": [issuer_public]}}
    return {"vcts": [POA_VCT], "issuers": issuers}


def sign_credential_jwt(*, claims: dict[str, Any], issuer_private: dict[str, Any]) -> str:
    return jwt.encode(
        claims,
        ECAlgorithm.from_jwk(json.dumps(issuer_private)),
        algorithm="ES256",
        headers={"kid": issuer_private.get("kid", ISSUER_KID), "typ": CREDENTIAL_JWT_TYP},
    )


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        fh.write(content.rstrip() + "\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump(data, fh, indent=2)
        fh.write("\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def generate_keypairs() -> tuple[dict[str, Any], dict[str, Any]]:
    issuer_private = ec_private_jwk(ec.generate_private_key(ec.SECP256R1()), kid=ISSUER_KID)
    wallet_private = ec_private_jwk(ec.generate_private_key(ec.SECP256R1()), kid=WALLET_KID)
    return issuer_private, wallet_private


def render_credentials(
    *,
    credentials_dir: Path,
    holder_did: str,
    issuer_did: str,
    issuer_private: dict[str, Any],
) -> None:
    for legacy in credentials_dir.glob("*.json"):
        if legacy.name.endswith(".template.json"):
            continue
        legacy.unlink(missing_ok=True)

    for template in sorted(credentials_dir.glob("*.template.json")):
        with template.open(encoding="utf-8") as fh:
            template_data = json.load(fh)
        claims = {
            "iss": issuer_did,
            "sub": holder_did,
            "vct": POA_VCT,
            "organization": template_data["organization"],
            "roles": template_data["roles"],
            "iat": CREDENTIAL_IAT,
            "exp": CREDENTIAL_EXP,
        }
        stem = template.name.replace(".template.json", "")
        token = sign_credential_jwt(claims=claims, issuer_private=issuer_private)
        write_text(credentials_dir / f"{stem}{CREDENTIAL_EXT}", token)


def vault_request(
    *,
    method: str,
    addr: str,
    token: str,
    path: str,
    body: dict[str, Any] | None = None,
) -> dict[str, Any]:
    url = f"{addr.rstrip('/')}/v1/secret/data/{path}"
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "X-Vault-Token": token,
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode())


def vault_read_json(addr: str, token: str, path: str) -> dict[str, Any] | None:
    try:
        payload = vault_request(method="GET", addr=addr, token=token, path=path)
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            return None
        raise
    raw = payload.get("data", {}).get("data", {}).get("value")
    if not raw:
        return None
    return json.loads(raw)


def vault_write_json(addr: str, token: str, path: str, value: Any) -> None:
    vault_request(
        method="POST",
        addr=addr,
        token=token,
        path=path,
        body={"data": {"value": json.dumps(value, separators=(",", ":"))}},
    )


def credential_jwt_paths(credentials_dir: Path) -> list[Path]:
    return sorted(p for p in credentials_dir.glob(f"*{CREDENTIAL_EXT}") if p.is_file())


def existing_local_material(
    *,
    keys_dir: Path,
    trust_path: Path,
    credentials_dir: Path,
) -> list[Path]:
    found: list[Path] = []
    for name in ("issuer-dev.jwk", "wallet.jwk"):
        path = keys_dir / name
        if path.is_file():
            found.append(path)
    if trust_path.is_file():
        found.append(trust_path)
    found.extend(credential_jwt_paths(credentials_dir))
    return found


def planned_overwrite_reasons(
    *,
    args: argparse.Namespace,
    existing: list[Path],
) -> list[str]:
    if not existing:
        return []

    reasons: list[str] = []
    key_paths = {args.keys_dir / "issuer-dev.jwk", args.keys_dir / "wallet.jwk"}
    has_issuer = (args.keys_dir / "issuer-dev.jwk").is_file()
    has_wallet = (args.keys_dir / "wallet.jwk").is_file()

    if args.vault_read:
        reasons.append("replace local files with material read from Vault")
    elif args.regenerate_keys:
        reasons.append("generate new issuer-dev.jwk and wallet.jwk (--regenerate-keys)")
    else:
        if has_issuer and has_wallet:
            reasons.append("keep existing issuer-dev.jwk and wallet.jwk")
        elif has_wallet and not has_issuer:
            reasons.append("keep wallet.jwk and generate a new issuer-dev.jwk")
        elif has_issuer and not has_wallet:
            reasons.append("keep issuer-dev.jwk and generate a new wallet.jwk")

    if args.trust_path in existing:
        reasons.append(f"rewrite {args.trust_path.name} from the issuer public key")

    creds = [p for p in existing if p.parent == args.credentials_dir]
    if creds:
        names = ", ".join(p.name for p in creds)
        reasons.append(f"re-render credentials ({names}) as *{CREDENTIAL_EXT} from *.template.json")

    if args.vault_write:
        reasons.append("write keys and trust to Vault (--vault-write)")

    return reasons


def confirm_overwrite(existing: list[Path], reasons: list[str]) -> bool:
    print("Local OID4VP material already exists:")
    for path in existing:
        print(f"  {path}")
    print()
    print("This run will:")
    for reason in reasons:
        print(f"  - {reason}")
    print()
    print("Overwriting keys or trust invalidates wallet login until backend trust and credentials match.")
    try:
        answer = input("Continue? [y/N]: ").strip().lower()
    except (EOFError, KeyboardInterrupt):
        print()
        return False
    return answer in ("y", "yes")


def materialize_local(
    *,
    keys_dir: Path,
    trust_path: Path,
    credentials_dir: Path,
    issuer_did: str,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
) -> str:
    issuer_public = public_jwk(issuer_private)
    holder_did = did_jwk_from_public_jwk(public_jwk(wallet_private))
    trusted_dids = list(dict.fromkeys([issuer_did, *TRUSTED_ISSUER_DIDS]))
    trust = build_trust_json(issuer_public=issuer_public, issuer_dids=trusted_dids)

    write_json(keys_dir / "issuer-dev.jwk", issuer_private)
    write_json(keys_dir / "wallet.jwk", wallet_private)
    write_json(trust_path, trust)
    render_credentials(
        credentials_dir=credentials_dir,
        holder_did=holder_did,
        issuer_did=issuer_did,
        issuer_private=issuer_private,
    )
    return holder_did


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate demo OID4VP keys, trust.json, and credentials.")
    parser.add_argument("--issuer-did", default=DEFAULT_ISSUER_DID)
    parser.add_argument("--keys-dir", type=Path, default=repo_root() / "testWallet" / "keys")
    parser.add_argument("--trust-path", type=Path, default=repo_root() / "backend" / "config" / "oid4vp" / "trust.dev.json")
    parser.add_argument("--credentials-dir", type=Path, default=repo_root() / "testWallet" / "credentials")
    parser.add_argument(
        "--yes",
        action="store_true",
        help="skip confirmation when local material exists (rewrites trust and credentials; keeps keys unless --regenerate-keys)",
    )
    parser.add_argument(
        "--regenerate-keys",
        action="store_true",
        help="replace issuer-dev.jwk and wallet.jwk with new pairs (changes holder sub / did:jwk)",
    )
    parser.add_argument("--force", action="store_true", help=argparse.SUPPRESS)  # deprecated alias
    parser.add_argument("--overwrite", action="store_true", help=argparse.SUPPRESS)  # deprecated alias
    parser.add_argument("--vault-read", action="store_true", help="Load key material from Vault instead of generating")
    parser.add_argument("--vault-write", action="store_true", help="Store generated material in Vault KV v2")
    args = parser.parse_args()

    if args.force:
        args.regenerate_keys = True
    skip_confirm = args.yes or args.overwrite
    existing = existing_local_material(
        keys_dir=args.keys_dir,
        trust_path=args.trust_path,
        credentials_dir=args.credentials_dir,
    )
    if existing and not skip_confirm:
        reasons = planned_overwrite_reasons(args=args, existing=existing)
        if not sys.stdin.isatty():
            print(
                "Local OID4VP material already exists; use --yes to continue non-interactively.",
                file=sys.stderr,
            )
            return 1
        if not confirm_overwrite(existing, reasons):
            print("Aborted.")
            return 0

    vault_addr = os.environ.get("VAULT_ADDR", "").strip()
    vault_token = os.environ.get("VAULT_TOKEN", "").strip()

    issuer_private: dict[str, Any] | None = None
    wallet_private: dict[str, Any] | None = None
    trust: dict[str, Any] | None = None

    if args.vault_read:
        if not vault_addr or not vault_token:
            print("VAULT_ADDR and VAULT_TOKEN are required for --vault-read", file=sys.stderr)
            return 1
        issuer_private = vault_read_json(vault_addr, vault_token, VAULT_ISSUER_PATH)
        wallet_private = vault_read_json(vault_addr, vault_token, VAULT_WALLET_PATH)
        trust = vault_read_json(vault_addr, vault_token, VAULT_TRUST_PATH)
        if not issuer_private or not wallet_private or not trust:
            print("Vault demo material incomplete; run with --vault-write after cluster init", file=sys.stderr)
            return 1
    elif not args.regenerate_keys:
        issuer_path = args.keys_dir / "issuer-dev.jwk"
        wallet_path = args.keys_dir / "wallet.jwk"
        if issuer_path.is_file():
            with issuer_path.open(encoding="utf-8") as fh:
                issuer_private = json.load(fh)
        if wallet_path.is_file():
            with wallet_path.open(encoding="utf-8") as fh:
                wallet_private = json.load(fh)

    if args.regenerate_keys or (issuer_private is None and wallet_private is None):
        issuer_private, wallet_private = generate_keypairs()
    elif issuer_private is None and wallet_private is not None:
        issuer_private = ec_private_jwk(ec.generate_private_key(ec.SECP256R1()), kid=ISSUER_KID)
    elif wallet_private is None and issuer_private is not None:
        wallet_private = ec_private_jwk(ec.generate_private_key(ec.SECP256R1()), kid=WALLET_KID)

    if trust is None and issuer_private is not None:
        trusted_dids = list(dict.fromkeys([args.issuer_did, *TRUSTED_ISSUER_DIDS]))
        trust = build_trust_json(issuer_public=public_jwk(issuer_private), issuer_dids=trusted_dids)

    assert issuer_private is not None and wallet_private is not None and trust is not None
    holder_did = materialize_local(
        keys_dir=args.keys_dir,
        trust_path=args.trust_path,
        credentials_dir=args.credentials_dir,
        issuer_did=args.issuer_did,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
    )

    if args.vault_write:
        if not vault_addr or not vault_token:
            print("VAULT_ADDR and VAULT_TOKEN are required for --vault-write", file=sys.stderr)
            return 1
        vault_write_json(vault_addr, vault_token, VAULT_ISSUER_PATH, issuer_private)
        vault_write_json(vault_addr, vault_token, VAULT_WALLET_PATH, wallet_private)
        vault_write_json(vault_addr, vault_token, VAULT_TRUST_PATH, trust)
        print(f"Vault sync OK ({time.time():.0f})")

    print(f"holder did:jwk ready: {holder_did[:48]}...")
    print(f"issuer: {args.issuer_did}")
    print(f"trust: {args.trust_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
