"""Headless wallet+QTSP stand-in for the OID4VP Document-Retrieval signing
ceremony (ADR-12).

Where ceremony_driver.py calls the DCS's prepare/submit endpoints directly, this
driver consumes the STANDARD OID4VP Document-Retrieval request object instead:
given the QR (client_id + request_uri the DCS's publish step emits), it

    1. fetches the signed request object (JAR) from request_uri,
    2. parses its claims (documentDigests, documentLocations, response_uri, nonce),
    3. fetches the to-be-signed document from documentLocations[].uri,
    4. signs it with the signatory's own key via the external SCA (an EU DSS),
    5. posts the signed document back to response_uri (direct_post,
       form-urlencoded documentWithSignature[]).

The DCS validates the returned signature identifies the signatory (sole control)
and finalizes the contract. Nothing DCS-specific crosses the wallet boundary but
the standard request object, so a real EUDI wallet swaps in by config.

    resp = sign_via_document_retrieval(
        request_uri="https://dcs/api/signature/request/<id>/object",
        user="SignerOne", dss_url="http://localhost:18099",
        keys_dir=Path("~/.dcs/wallet-keys"),
    )
"""

from __future__ import annotations

import base64
import json
import urllib.request
from pathlib import Path
from urllib.parse import urlencode, urlsplit, urlunsplit

from dcs_wallet.remote_signer import sign_pdf


def _reorigin(url: str, origin_of: str) -> str:
    """Rewrite url to carry the scheme+host of origin_of. The publish step builds
    request_uri, documentLocations, and response_uri all from one public base, so
    they share an origin; pointing them at the origin the caller actually reached
    (the request_uri it was handed) keeps the document and callback reachable even
    when the DCS's advertised public host differs from the caller's route."""
    src, dst = urlsplit(url), urlsplit(origin_of)
    return urlunsplit((dst.scheme, dst.netloc, src.path, src.query, src.fragment))


def _get(url: str, accept: str = "*/*") -> bytes:
    req = urllib.request.Request(url, headers={"Accept": accept})
    with urllib.request.urlopen(req, timeout=120) as resp:
        return resp.read()


def _decode_jwt_claims(compact_jwt: str) -> dict:
    """Decode a compact JWS payload (claims) without verifying the signature —
    the wallet trusts the request_uri it was handed and only needs the claims."""
    parts = compact_jwt.strip().split(".")
    if len(parts) < 2:
        raise RuntimeError("request object is not a compact JWT")
    payload_b64 = parts[1]
    payload_b64 += "=" * (-len(payload_b64) % 4)
    return json.loads(base64.urlsafe_b64decode(payload_b64))


def sign_via_document_retrieval(
    *,
    request_uri: str,
    user: str,
    dss_url: str,
    keys_dir: Path,
    field: str = "",
) -> dict:
    """Consume the OID4VP Document-Retrieval request object and return the DCS's
    callback response (JSON). user is the sole-control token: the signing
    certificate's subject is "CN=DCS Signatory <user>", which the DCS's
    validation gate requires to identify the ceremony's signatory.
    """
    request_object = _get(request_uri, accept="application/oauth-authz-req+jwt").decode()
    claims = _decode_jwt_claims(request_object)

    locations = claims.get("documentLocations") or []
    response_uri = claims.get("response_uri")
    if not locations:
        raise RuntimeError("request object carries no documentLocations")
    if not response_uri:
        raise RuntimeError("request object carries no response_uri")

    document_uri = _reorigin(locations[0]["uri"], request_uri)
    response_uri = _reorigin(response_uri, request_uri)

    to_be_signed = _get(document_uri, accept="application/pdf")
    signed_pdf = sign_pdf(
        to_be_signed, user=user, dss_url=dss_url, field=field, keys_dir=keys_dir
    )

    # The EUDI walletdriven-signer direct_post: an application/x-www-form-urlencoded
    # body carrying the PAdES-signed document (enveloped in the PDF) in the
    # documentWithSignature[] list. The ceremony identity is the response_uri path,
    # so no state is echoed.
    body = urlencode({
        "documentWithSignature[0]": base64.b64encode(signed_pdf).decode(),
    }).encode()
    post = urllib.request.Request(
        response_uri,
        data=body,
        headers={"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
    )
    with urllib.request.urlopen(post, timeout=120) as resp:
        return json.loads(resp.read())


def _main() -> None:
    import argparse
    import os
    import sys

    parser = argparse.ArgumentParser(
        description="Sign a DCS contract via the OID4VP Document-Retrieval ceremony (fetch request object -> fetch document -> sign -> post to callback)."
    )
    parser.add_argument("--request-uri", required=True, help="request_uri from the publish QR")
    parser.add_argument("--user", required=True, help="signatory name; the signing cert is 'CN=DCS Signatory <user>'")
    parser.add_argument("--field", default="", help="signature field for multi-signer contracts")
    parser.add_argument("--dss-url", default=os.getenv("DSS_URL", "http://localhost:18099"))
    parser.add_argument("--keys-dir", default=os.getenv("BDD_TEST_WALLET_KEYS_DIR", str(Path.home() / ".dcs" / "wallet-keys")))
    args = parser.parse_args()

    resp = sign_via_document_retrieval(
        request_uri=args.request_uri,
        user=args.user,
        field=args.field,
        dss_url=args.dss_url,
        keys_dir=Path(args.keys_dir),
    )
    print(json.dumps(resp, indent=2))
    if resp.get("status") != "SIGNED":
        sys.exit(f"contract not SIGNED: {resp}")


if __name__ == "__main__":
    _main()
