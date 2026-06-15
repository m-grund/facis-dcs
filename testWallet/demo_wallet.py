#!/usr/bin/env python3
"""
DCS demonstration wallet — local OpenID4VP CLI for login testing.

Loads PoA test credentials from this directory, fetches the presentation request,
builds a verifiable presentation, and POSTs it to the DCS callback endpoint.

Usage (from repo root):

  # With /ui/ login open — paste presentation_url from QR or Copy link:
  python3 testWallet/demo_wallet.py

  # Pick credential interactively after pasting the presentation link (lists roles).

  # Non-interactive:
  python3 testWallet/demo_wallet.py --credential neusta-gmbh_johndoe --presentation-url 'openid4vp://...'

  # Headless end-to-end (POST /auth/login, no browser tab):
  python3 testWallet/demo_wallet.py --headless --credential test
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from dataclasses import dataclass
import urllib.error
import urllib.request
from http.cookiejar import CookieJar
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse

sys.path.insert(0, str(Path(__file__).resolve().parent))
from dcs_wallet.credential import CREDENTIAL_EXT, decode_jwt_payload, load_credential_claims
from dcs_wallet.presentation import build_vp_token

DEFAULT_API_BASE = os.environ.get("DCS_API_BASE", "http://localhost:8991/api")
DEFAULT_CREDENTIAL = os.environ.get("DCS_WALLET_CREDENTIAL", "test")
REQUEST_URI_MARKER = "/auth/presentation/request/"
_CREDENTIALS_DIR = Path(__file__).resolve().parent / "credentials"
_KEYS_DIR = Path(__file__).resolve().parent / "keys"
_REQUIRED_KEYS = ("issuer-dev.jwk", "wallet.jwk")
_GENERATE_HINT = "python3 testWallet/scripts/generate_keys.py --yes && python3 testWallet/scripts/issue_credentials.py"


@dataclass(frozen=True)
class CredentialOption:
    stem: str
    organization: str
    roles: list[str]


def log(step: str, msg: str, **extra: object) -> None:
    suffix = ""
    if extra:
        suffix = " " + json.dumps(extra, default=str)
    print(f"[{step}] {msg}{suffix}")


def _api_error_message(body: str) -> str:
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        return body.strip()[:300]
    if isinstance(data, dict):
        msg = data.get("message") or data.get("error") or data.get("detail")
        if msg:
            return str(msg)
    return body.strip()[:300]


class _Response:
    def __init__(self, status: int, headers: dict[str, str], body: bytes) -> None:
        self.status_code = status
        self.headers = headers
        self._body = body

    def json(self) -> object:
        if not self._body:
            return {}
        return json.loads(self._body.decode())

    @property
    def text(self) -> str:
        return self._body.decode(errors="replace")


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class _Session:
    def __init__(self) -> None:
        self._jar = CookieJar()
        self._opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self._jar))
        self._headers = {
            "User-Agent": "testWallet/demo_wallet",
            "Accept": "application/json",
        }

    def get(self, url: str, *, timeout: float = 30, allow_redirects: bool = True) -> _Response:
        req = urllib.request.Request(url, headers=self._headers, method="GET")
        if allow_redirects:
            return self._open(self._opener, req, timeout)
        opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self._jar),
            _NoRedirect(),
        )
        try:
            return self._open(opener, req, timeout)
        except urllib.error.HTTPError as exc:
            if exc.code in (301, 302, 303, 307, 308):
                return _Response(exc.code, {k.lower(): v for k, v in exc.headers.items()}, exc.read())
            raise

    def post(self, url: str, *, json_body: dict | None = None, timeout: float = 30, accept: str | None = None) -> _Response:
        data = b"" if json_body is None else json.dumps(json_body).encode()
        headers = dict(self._headers)
        if json_body is not None:
            headers["Content-Type"] = "application/json"
        if accept:
            headers["Accept"] = accept
        req = urllib.request.Request(url, data=data, headers=headers, method="POST")
        return self._open(self._opener, req, timeout)

    @staticmethod
    def _open(opener, req: urllib.request.Request, timeout: float) -> _Response:
        try:
            with opener.open(req, timeout=timeout) as fp:
                body = fp.read()
                headers = {k.lower(): v for k, v in fp.headers.items()}
                return _Response(fp.status, headers, body)
        except urllib.error.HTTPError as exc:
            body = exc.read()
            headers = {k.lower(): v for k, v in exc.headers.items()}
            return _Response(exc.code, headers, body)
        except urllib.error.URLError as exc:
            raise RuntimeError(f"request failed: {exc.reason}") from exc


def normalize_callback_url(redirect_uri: str, api_base: str) -> str:
    api = urlparse(api_base)
    target = urlparse(redirect_uri)
    if target.netloc.endswith(":5173") and api.port == 8991:
        return redirect_uri.replace(f"{target.scheme}://{target.netloc}", f"{api.scheme}://{api.netloc}", 1)
    return redirect_uri


def api_base_from_request_uri(request_uri: str) -> str:
    parsed = urlparse(request_uri)
    path = parsed.path
    idx = path.find(REQUEST_URI_MARKER)
    if idx < 0:
        raise ValueError(f"request_uri missing {REQUEST_URI_MARKER}")
    return f"{parsed.scheme}://{parsed.netloc}{path[:idx]}"


def _query_from_openid4vp_url(value: str) -> str:
    parsed = urlparse(value)
    if parsed.scheme != "openid4vp":
        raise ValueError("expected openid4vp:// scheme")
    if parsed.query:
        return parsed.query
    if "?" in value:
        return value.split("?", 1)[1]
    raise ValueError("openid4vp:// missing query parameters")


def resolve_https_request_uri(raw: str) -> str:
    """Resolve presentation_url (openid4vp://) or HTTPS request_uri to fetch URL."""
    value = raw.strip().strip("'\"")
    if not value:
        raise ValueError("empty link")

    if value.startswith("openid4vp:"):
        params = parse_qs(_query_from_openid4vp_url(value))
        refs = params.get("request_uri") or []
        if not refs or not refs[0].strip():
            raise ValueError("openid4vp:// missing request_uri parameter")
        value = unquote(refs[0].strip())

    parsed = urlparse(value)
    if parsed.scheme not in ("http", "https"):
        raise ValueError("request_uri must be an http(s) URL")
    if REQUEST_URI_MARKER not in parsed.path:
        raise ValueError(f"request_uri must contain {REQUEST_URI_MARKER}")
    return value


def prompt_presentation_url() -> str:
    print("Paste presentation_url from the login page (QR / Copy link), then press Enter:")
    print("  (openid4vp://… from POST /auth/login)")
    try:
        line = sys.stdin.readline()
    except KeyboardInterrupt:
        print()
        raise SystemExit(130) from None
    if not line:
        raise ValueError("no input (EOF)")
    return resolve_https_request_uri(line)


def decode_authorization_request_jwt(token: str) -> dict:
    return decode_jwt_payload(token)


def list_available_credentials() -> list[CredentialOption]:
    options: list[CredentialOption] = []
    for path in sorted(_CREDENTIALS_DIR.glob(f"*{CREDENTIAL_EXT}")):
        data = load_credential_claims(path.stem)
        roles_raw = data.get("roles") or []
        roles = [r for r in roles_raw if isinstance(r, str)]
        options.append(
            CredentialOption(
                stem=path.stem,
                organization=str(data.get("organization") or "?"),
                roles=roles,
            )
        )
    return options


def _format_roles(roles: list[str]) -> str:
    if not roles:
        return "(none)"
    return ", ".join(roles)


def prompt_credential_choice(options: list[CredentialOption]) -> str:
    if len(options) == 1:
        opt = options[0]
        log(
            "wallet",
            "using only available credential",
            credential=opt.stem,
            organization=opt.organization,
            roles_count=len(opt.roles),
        )
        return opt.stem

    print("\nSelect credential to present:")
    for index, opt in enumerate(options, start=1):
        print(f"  [{index}] {opt.stem}")
        print(f"      organization: {opt.organization}")
        print(f"      roles ({len(opt.roles)}): {_format_roles(opt.roles)}")
    print()

    while True:
        try:
            line = input(f"Enter 1–{len(options)} [default 1]: ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            raise SystemExit(130) from None
        if not line:
            return options[0].stem
        try:
            choice = int(line)
        except ValueError:
            print(f"Invalid input — enter a number from 1 to {len(options)}.")
            continue
        if 1 <= choice <= len(options):
            chosen = options[choice - 1]
            log(
                "wallet",
                "credential selected",
                credential=chosen.stem,
                organization=chosen.organization,
                roles_count=len(chosen.roles),
            )
            return chosen.stem
        print(f"Invalid choice — enter a number from 1 to {len(options)}.")


def ensure_wallet_keys() -> bool:
    missing = [name for name in _REQUIRED_KEYS if not (_KEYS_DIR / name).is_file()]
    if not missing:
        return True
    log("wallet", "FAILED", error=f"missing keys: {', '.join(missing)} — run: {_GENERATE_HINT}")
    return False


def resolve_credential_name(credential_name: str | None) -> str | None:
    if credential_name:
        return credential_name
    options = list_available_credentials()
    if not options:
        log("wallet", "FAILED", error=f"no credentials/*{CREDENTIAL_EXT} found — run: {_GENERATE_HINT}")
        return None
    return prompt_credential_choice(options)


def state_from_request_uri(request_uri: str) -> str:
    parsed = urlparse(request_uri)
    path = parsed.path
    idx = path.find(REQUEST_URI_MARKER)
    if idx < 0:
        raise ValueError(f"request_uri missing {REQUEST_URI_MARKER}")
    tail = path[idx + len(REQUEST_URI_MARKER) :]
    state = unquote(tail.split("/")[0].strip())
    if not state:
        raise ValueError("request_uri has empty state")
    return state


def run_wallet_flow(
    session: _Session,
    api: str,
    state: str,
    request_uri: str,
    *,
    credential_name: str | None,
) -> int:
    log("fetch", "POST OpenID4VP request object", url=request_uri[:120])
    try:
        r = session.post(
            request_uri,
            timeout=30,
            accept="application/oauth-authz-req+jwt, application/jwt",
        )
    except RuntimeError as exc:
        log("fetch", "FAILED", error=str(exc))
        return 1
    if r.status_code != 200:
        log("fetch", "FAILED", status=r.status_code, error=_api_error_message(r.text))
        if r.status_code in (400, 404):
            log("fetch", "hint: presentation link expired or unknown — copy a fresh link from /ui/ login")
        return 1
    body = r.text.strip()
    content_type = (r.headers.get("content-type") or "").lower()
    if body.startswith("eyJ") or "oauth-authz-req+jwt" in content_type:
        try:
            req_obj = decode_authorization_request_jwt(body)
        except ValueError as exc:
            log("fetch", "FAILED", error=str(exc))
            return 1
    else:
        log("fetch", "FAILED", error="expected signed authorization request JWT (application/oauth-authz-req+jwt)")
        return 1
    log(
        "fetch-ok",
        "request object",
        response_uri=req_obj.get("response_uri"),
        nonce_prefix=str(req_obj.get("nonce") or "")[:12],
    )

    chosen = resolve_credential_name(credential_name)
    if not chosen:
        return 1
    if not ensure_wallet_keys():
        return 1

    nonce = str(req_obj.get("nonce") or "")
    client_id = str(req_obj.get("client_id") or "")
    try:
        vp_token = build_vp_token(
            credential_name=chosen,
            nonce=nonce,
            client_id=client_id,
        )
    except Exception as exc:
        log("present", "FAILED to build VP", error=str(exc))
        return 1
    log("present", "POST /auth/presentation/callback", credential=chosen)
    try:
        r = session.post(
            f"{api}/auth/presentation/callback",
            json_body={"state": state, "vp_token": vp_token},
            timeout=60,
        )
    except RuntimeError as exc:
        log("present", "FAILED", error=str(exc))
        return 1
    if r.status_code != 200:
        err = _api_error_message(r.text)
        log("present", "FAILED", status=r.status_code, error=err)
        if "hydra login challenge" in err.lower():
            log(
                "present",
                "hint: keep /ui/ open — complete Hydra authorize and bind login challenge before presenting VP",
            )
        return 1
    body = r.json()
    if not isinstance(body, dict):
        log("present", "FAILED", error="unexpected callback response shape")
        return 1
    redirect_uri = body.get("redirect_uri")
    if not redirect_uri:
        log("present", "FAILED missing redirect_uri")
        return 1
    log("present-ok", "VP accepted; browser should poll complete and open callback", redirect_prefix=redirect_uri[:96])

    if os.environ.get("DCS_WALLET_FINISH_BROWSER", "1").lower() in ("0", "false", "no"):
        log("wallet", "DONE — VP posted; browser tab should redirect via polling")
        return 0

    callback_url = normalize_callback_url(redirect_uri, api)
    log("oauth-callback", "GET /auth/callback (headless)", callback_prefix=callback_url[:100])
    r = session.get(callback_url, allow_redirects=False, timeout=30)
    if r.status_code != 302:
        log("oauth-callback", "FAILED", status=r.status_code, body=r.text[:200])
        return 1
    success_loc = r.headers.get("location")
    log("oauth-callback-ok", "callback OK", redirect=success_loc)

    log("session", "POST /auth/refresh")
    r = session.post(f"{api}/auth/refresh", timeout=30)
    if r.status_code != 200:
        log("session", "FAILED", status=r.status_code, body=r.text[:200])
        return 1
    access = r.json().get("access_token", "")
    log("session", "refresh OK", access_prefix=access[:32])
    log("wallet", "DONE — full headless login path succeeded")
    return 0


def present_for_browser_session(session: _Session, pasted: str, *, credential_name: str | None) -> int:
    try:
        request_uri = resolve_https_request_uri(pasted)
        state = state_from_request_uri(request_uri)
        api = api_base_from_request_uri(request_uri)
    except ValueError as exc:
        log("login", "FAILED", error=str(exc))
        return 1
    log("wallet", "present VP for browser login page", api_base=api, state_prefix=state[:16])
    return run_wallet_flow(session, api, state, request_uri, credential_name=credential_name)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Present PoA credentials to DCS via OpenID4VP (interactive or headless).",
    )
    parser.add_argument("--api-base", default=DEFAULT_API_BASE, help="Override API base (default from request-uri when set)")
    parser.add_argument(
        "--presentation-url",
        "--request-uri",
        dest="presentation_url",
        default=os.environ.get("DCS_PRESENTATION_URL", os.environ.get("DCS_OID4VP_REQUEST_URI", "")),
        help="presentation_url (openid4vp://) or HTTPS request_uri; skips prompt",
    )
    parser.add_argument(
        "--credential",
        default=None,
        help="Credential stem in testWallet/credentials/ (skip interactive picker when set)",
    )
    parser.add_argument(
        "--headless",
        action="store_true",
        help="POST /auth/login and run full flow without a browser tab",
    )
    parser.add_argument(
        "--browser-only",
        action="store_true",
        help=argparse.SUPPRESS,
    )
    args = parser.parse_args()
    credential = args.credential or (DEFAULT_CREDENTIAL if args.headless else None)

    session = _Session()

    if not args.headless:
        os.environ["DCS_WALLET_FINISH_BROWSER"] = "0"
        pasted = (args.presentation_url or "").strip()
        if not pasted:
            try:
                pasted = prompt_presentation_url()
            except ValueError as exc:
                log("login", "FAILED", error=str(exc))
                return 1
        return present_for_browser_session(session, pasted, credential_name=credential)

    api = args.api_base.rstrip("/")
    log("wallet", "headless login (new initiate)", api_base=api)
    log("login", "POST /auth/login")
    r = session.post(f"{api}/auth/login", timeout=30)
    if r.status_code != 200:
        log("login", "FAILED", status=r.status_code, body=r.text[:300])
        return 1
    init_body = r.json()
    state = init_body["state"]
    request_uri = init_body["request_uri"]
    log("login-ok", "initiate OK", request_uri_prefix=request_uri[:96], state_prefix=state[:16])
    return run_wallet_flow(session, api, state, request_uri, credential_name=credential)


if __name__ == "__main__":
    sys.exit(main())
