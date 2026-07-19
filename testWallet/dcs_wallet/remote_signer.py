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
import time
import urllib.error
import urllib.request
from pathlib import Path

from dcs_wallet.signer import ensure_signing_material, sign_dtbs


def _unsigned_signature_fields(pdf_bytes: bytes) -> list[str]:
    """Names of the AcroForm signature fields the prepared PDF carries and that
    are not yet signed — the fields the DCS placed from the contract's declared
    signatoryName (pdf-core /T == signatoryName)."""
    import io  # noqa: PLC0415

    from pypdf import PdfReader  # noqa: PLC0415

    reader = PdfReader(io.BytesIO(pdf_bytes))
    return [
        name
        for name, field in (reader.get_fields() or {}).items()
        if field.get("/FT") == "/Sig" and not field.get("/V")
    ]


def _dss_post(dss_url: str, path: str, body: dict) -> dict:
    req = urllib.request.Request(
        dss_url.rstrip("/") + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as exc:
        # Surface the DSS server's error body — its exception message names the
        # exact rejected parameter, which a bare "HTTP 500" hides.
        detail = exc.read().decode("utf-8", "replace")[:3000]
        raise RuntimeError(f"DSS {path} returned HTTP {exc.code}: {detail}") from exc


def _pades_params(cert_b64: str, field: str) -> dict:
    return {
        "signingCertificate": {"encodedCertificate": cert_b64},
        "signatureLevel": "PAdES_BASELINE_B",
        "digestAlgorithm": "SHA256",
        "signaturePackaging": "ENVELOPED",
        # A fixed signing time shared by both remote calls: the CMS
        # SignedAttributes carry the signing-time, and DSS regenerates them per
        # call, so without pinning it getDataToSign and signDocument cover
        # different bytes and the signature fails validation (SIG_CRYPTO_FAILURE).
        "blevelParams": {"signingDate": int(time.time() * 1000)},
        "imageParameters": {"fieldParameters": {"fieldId": field}},
    }


def sign_pdf(prepared_pdf: bytes, *, user: str, dss_url: str, field: str = "", keys_dir: Path, name: str = "contract.pdf") -> bytes:
    """Sign prepared_pdf's AcroForm signature field as the external SCA, signing
    the DTBS with the signatory's own key. field selects which field on a
    multi-signer document; empty picks the document's own field. The field must
    already exist: the two remote calls (getDataToSign then signDocument) are
    only deterministic — and so only produce a valid signature — over a
    pre-placed field, so a signable contract declares its signature field
    (pdf-core /T == signatoryName) and prepare renders it.
    """
    existing = _unsigned_signature_fields(prepared_pdf)
    if not existing:
        raise RuntimeError("prepared PDF has no unsigned signature field to sign")
    if not field:
        field = existing[0]
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
