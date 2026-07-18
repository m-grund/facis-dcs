"""The wallet drives the remote signing (EUDI walletdriven-signer model).

The wallet holds the signatory's key (sole control). Given a prepared PDF (the
DCS has embedded the PoA + placed the AcroForm field), the wallet drives its
EXTERNAL SCA — an EU DSS — through the rQES two-call flow and signs the
data-to-be-signed itself with the signatory's key. The DCS never sees the key
and never calls the wallet; the wallet returns the finished signed document.

    signed_pdf = sign_pdf(prepared_pdf, user="johndoe", dss_url=..., field="SignerOne", keys_dir=...)
"""

from __future__ import annotations

import base64
import json
import urllib.request
from pathlib import Path

from dcs_wallet.signer import ensure_signing_material, sign_dtbs


def _dss_post(dss_url: str, path: str, body: dict) -> dict:
    req = urllib.request.Request(
        dss_url.rstrip("/") + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "Accept": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read())


def _pades_params(cert_b64: str, field: str) -> dict:
    params: dict = {
        "signingCertificate": {"encodedCertificate": cert_b64},
        "signatureLevel": "PAdES_BASELINE_B",
        "digestAlgorithm": "SHA256",
        "signaturePackaging": "ENVELOPED",
    }
    if field:
        params["imageParameters"] = {"fieldParameters": {"fieldId": field}}
    return params


def sign_pdf(prepared_pdf: bytes, *, user: str, dss_url: str, field: str, keys_dir: Path, name: str = "contract.pdf") -> bytes:
    """Sign prepared_pdf into its AcroForm field, driving DSS as the external SCA
    and signing the DTBS with the signatory's own key. Returns the signed PDF.
    """
    signing_jwk, cert_der = ensure_signing_material(user, keys_dir)
    cert_b64 = base64.b64encode(cert_der).decode()
    params = _pades_params(cert_b64, field)
    doc = {"bytes": base64.b64encode(prepared_pdf).decode(), "name": name}

    dtbs_b64 = _dss_post(dss_url, "/services/rest/signature/one-document/getDataToSign",
                         {"parameters": params, "toSignDocument": doc})["bytes"]
    signature = sign_dtbs(base64.b64decode(dtbs_b64), signing_jwk)

    signed_b64 = _dss_post(dss_url, "/services/rest/signature/one-document/signDocument", {
        "parameters": params,
        "toSignDocument": doc,
        "signatureValue": {"algorithm": "ECDSA_SHA256", "value": base64.b64encode(signature).decode()},
    })["bytes"]
    return base64.b64decode(signed_b64)
