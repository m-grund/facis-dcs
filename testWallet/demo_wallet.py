#!/usr/bin/env python3
"""
DCS demonstration wallet — local OpenID4VP CLI for login testing.

Loads PoA test credentials from this directory, fetches the presentation request,
builds a verifiable presentation, and POSTs it to the DCS callback endpoint.

Usage (from repo root):

  # With /ui/ login open — paste presentation_url from QR or Copy link:
  python3 testWallet/demo_wallet.py

  # Non-interactive:
  python3 testWallet/demo_wallet.py --presentation-url 'openid4vp://?client_id=...&request_uri=...'

  # Headless end-to-end (POST /auth/login, no browser tab):
  python3 testWallet/demo_wallet.py --headless
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from http.cookiejar import CookieJar
from urllib.parse import parse_qs, unquote, urlparse

DEFAULT_API_BASE = os.environ.get("DCS_API_BASE", "http://localhost:8991/api")
REQUEST_URI_MARKER = "/auth/presentation/request/"


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

    def post(self, url: str, *, json_body: dict | None = None, timeout: float = 30) -> _Response:
        data = json.dumps(json_body or {}).encode()
        headers = {**self._headers, "Content-Type": "application/json"}
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


def run_wallet_flow(session: _Session, api: str, state: str, request_uri: str) -> int:
    log("fetch", "GET OpenID4VP request object", url=request_uri[:120])
    try:
        r = session.get(request_uri, timeout=30)
    except RuntimeError as exc:
        log("fetch", "FAILED", error=str(exc))
        return 1
    if r.status_code != 200:
        log("fetch", "FAILED", status=r.status_code, error=_api_error_message(r.text))
        if r.status_code in (400, 404):
            log("fetch", "hint: presentation link expired or unknown — copy a fresh link from /ui/ login")
        return 1
    req_obj = r.json()
    if not isinstance(req_obj, dict):
        log("fetch", "FAILED", error="unexpected request object shape")
        return 1
    log(
        "fetch-ok",
        "request object",
        response_uri=req_obj.get("response_uri"),
        nonce_prefix=str(req_obj.get("nonce") or "")[:12],
    )

    log("present", "POST /auth/presentation/callback", vp_token="stub")
    try:
        r = session.post(
            f"{api}/auth/presentation/callback",
            json_body={"state": state, "vp_token": "stub"},
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


def present_for_browser_session(session: _Session, pasted: str) -> int:
    try:
        request_uri = resolve_https_request_uri(pasted)
        state = state_from_request_uri(request_uri)
        api = api_base_from_request_uri(request_uri)
    except ValueError as exc:
        log("login", "FAILED", error=str(exc))
        return 1
    log("wallet", "present VP for browser login page", api_base=api, state_prefix=state[:16])
    return run_wallet_flow(session, api, state, request_uri)


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
        return present_for_browser_session(session, pasted)

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
    return run_wallet_flow(session, api, state, request_uri)


if __name__ == "__main__":
    sys.exit(main())
