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
import io
import json
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta
from pathlib import Path

from dcs_wallet.signer import ensure_signing_material, sign_dtbs

_FONT_PATH = Path(__file__).resolve().parent / "fonts" / "Arimo-Regular.ttf"


def _load_font(size: int):
    from PIL import ImageFont  # noqa: PLC0415

    if not _FONT_PATH.is_file():
        raise FileNotFoundError(f"signature appearance font missing: {_FONT_PATH}")
    return ImageFont.truetype(str(_FONT_PATH), size=size)


def _format_signing_date(signing_ms: int) -> str:
    local = datetime.fromtimestamp(signing_ms / 1000).astimezone()
    offset = local.utcoffset() or timedelta(0)
    total_minutes = int(offset.total_seconds() // 60)
    sign = "+" if total_minutes >= 0 else "-"
    total_minutes = abs(total_minutes)
    hours, minutes = divmod(total_minutes, 60)
    return f"{local.strftime('%Y.%m.%d %H:%M:%S')} {sign}{hours:02d}'{minutes:02d}'"


def _text_width(draw, text: str, font) -> float:
    bbox = draw.textbbox((0, 0), text, font=font)
    return float(bbox[2] - bbox[0])


def _wrap_line(draw, text: str, font, max_width: float) -> list[str]:
    text = text.strip()
    if not text:
        return []
    if _text_width(draw, text, font) <= max_width:
        return [text]

    tokens: list[str] = []
    buf = ""
    for ch in text:
        buf += ch
        if ch in " -":
            tokens.append(buf)
            buf = ""
    if buf:
        tokens.append(buf)

    lines: list[str] = []
    current = ""
    for token in tokens:
        trial = current + token
        if _text_width(draw, trial, font) <= max_width:
            current = trial
            continue
        if current.strip():
            lines.append(current.rstrip())
            current = ""
        token = token.lstrip() if not current else token
        if _text_width(draw, token, font) <= max_width:
            current = token
            continue
        chunk = ""
        for ch in token:
            trial = chunk + ch
            if chunk and _text_width(draw, trial, font) > max_width:
                lines.append(chunk)
                chunk = ch
            else:
                chunk = trial
        current = chunk
    if current.strip():
        lines.append(current.rstrip())
    return lines


def _wrap_paragraph(draw, text: str, font, max_width: float) -> list[str]:
    lines: list[str] = []
    for logical in text.splitlines():
        wrapped = _wrap_line(draw, logical, font, max_width)
        lines.extend(wrapped or [""])
    return lines


def _block_height(draw, lines: list[str], font, spacing: int) -> float:
    if not lines:
        return 0.0
    total = 0.0
    for i, line in enumerate(lines):
        bbox = draw.textbbox((0, 0), line or " ", font=font)
        total += bbox[3] - bbox[1]
        if i + 1 < len(lines):
            total += spacing
    return total


def _appearance_png(display_name: str, signing_ms: int) -> bytes:
    from PIL import Image, ImageDraw  # noqa: PLC0415

    width, height = 1080, 210
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)

    left_margin = 12
    right_margin = 12
    gap = 20
    left_col_width = int(width * 0.46) - left_margin
    right_x = left_margin + left_col_width + gap
    right_col_width = width - right_x - right_margin

    name_size = 64
    meta_size = 28
    name_spacing = 4
    meta_spacing = 8

    font_name = _load_font(name_size)
    name_lines = _wrap_paragraph(draw, display_name, font_name, left_col_width)
    while _block_height(draw, name_lines, font_name, name_spacing) > height - 16 and name_size > 36:
        name_size -= 4
        font_name = _load_font(name_size)
        name_lines = _wrap_paragraph(draw, display_name, font_name, left_col_width)

    font_meta = _load_font(meta_size)
    meta_src = f"Digitally signed by {display_name}\nDate: {_format_signing_date(signing_ms)}"
    meta_lines = _wrap_paragraph(draw, meta_src, font_meta, right_col_width)
    while _block_height(draw, meta_lines, font_meta, meta_spacing) > height - 16 and meta_size > 18:
        meta_size -= 2
        font_meta = _load_font(meta_size)
        meta_lines = _wrap_paragraph(draw, meta_src, font_meta, right_col_width)

    def _draw_block(x: float, lines: list[str], font, spacing: int) -> None:
        block_h = _block_height(draw, lines, font, spacing)
        y = (height - block_h) / 2
        for line in lines:
            bbox = draw.textbbox((0, 0), line or " ", font=font)
            draw.text((x, y - bbox[1]), line, fill=(0, 0, 0), font=font)
            y += (bbox[3] - bbox[1]) + spacing

    _draw_block(left_margin, name_lines, font_name, name_spacing)
    _draw_block(right_x, meta_lines, font_meta, meta_spacing)

    buf = io.BytesIO()
    image.save(buf, format="PNG", optimize=True)
    return buf.getvalue()


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


def _pades_params(
    cert_b64: str,
    field: str,
    display_name: str,
    signing_ms: int,
) -> dict:
    appearance = _appearance_png(display_name, signing_ms)
    return {
        "signingCertificate": {"encodedCertificate": cert_b64},
        # PAdES-B-T: the SCA (DSS) embeds an RFC3161 signature-timestamp from its
        # configured TSP source. The DSS demo's default source is an in-process
        # TSA (config/tsp-config.xml); prod swaps that file for an OnlineTSPSource
        # pointing at a real/ORCE TSA — config-only, no code change.
        "signatureLevel": "PAdES_BASELINE_T",
        "digestAlgorithm": "SHA256",
        "signaturePackaging": "ENVELOPED",
        # A fixed signing time shared by both remote calls: the CMS
        # SignedAttributes carry the signing-time, and DSS regenerates them per
        # call, so without pinning it getDataToSign and signDocument cover
        # different bytes and the signature fails validation (SIG_CRYPTO_FAILURE).
        "blevelParams": {"signingDate": signing_ms},
        "imageParameters": {
            "fieldParameters": {"fieldId": field},
            "image": {
                "bytes": base64.b64encode(appearance).decode(),
                "name": "signature-appearance.png",
            },
            "imageScaling": "STRETCH",
        },
    }


def sign_pdf(
    prepared_pdf: bytes,
    *,
    user: str,
    dss_url: str,
    field: str = "",
    keys_dir: Path,
    name: str = "contract.pdf",
) -> bytes:
    """Sign prepared_pdf's AcroForm signature field as the external SCA, signing
    the DTBS with the signatory's own key. field selects which field on a
    multi-signer document; empty is allowed only when exactly one unsigned field
    remains. The field must already exist: the two remote calls (getDataToSign
    then signDocument) are only deterministic — and so only produce a valid
    signature — over a pre-placed field, so a signable contract declares its
    signature field (pdf-core /T == signatoryName) and prepare renders it.
    """
    existing = _unsigned_signature_fields(prepared_pdf)
    if not existing:
        raise RuntimeError("prepared PDF has no unsigned signature field to sign")
    if not field:
        if len(existing) != 1:
            raise RuntimeError(
                "field is required when the PDF contains multiple unsigned signature fields; "
                f"found {existing!r}"
            )
        field = existing[0]
    elif field not in existing:
        raise RuntimeError(f"signature field {field!r} is not an unsigned field on the PDF; found {existing!r}")

    signing_jwk, cert_der = ensure_signing_material(user, keys_dir)
    cert_b64 = base64.b64encode(cert_der).decode()
    signing_ms = int(time.time() * 1000)
    params = _pades_params(cert_b64, field, user, signing_ms)
    doc = {"bytes": base64.b64encode(prepared_pdf).decode(), "name": name}


    dtbs_b64 = _dss_post(
        dss_url,
        "/services/rest/signature/one-document/getDataToSign",
        {"parameters": params, "toSignDocument": doc},
    )["bytes"]
    signature = sign_dtbs(base64.b64decode(dtbs_b64), signing_jwk)

    signed_b64 = _dss_post(
        dss_url,
        "/services/rest/signature/one-document/signDocument",
        {
            "parameters": params,
            "toSignDocument": doc,
            "signatureValue": {"algorithm": "ECDSA_SHA256", "value": base64.b64encode(signature).decode()},
        },
    )["bytes"]
    return base64.b64decode(signed_b64)
