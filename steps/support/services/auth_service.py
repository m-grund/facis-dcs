"""Authentication and authorization service for BDD steps."""

from __future__ import annotations

import base64
import json
import os
import re
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, unquote, urlparse

import requests

REQUEST_URI_MARKER = "/auth/presentation/request/"


@dataclass(frozen=True)
class AuthCredentials:
    """Roles and organization used to build the OID4VP presentation."""

    roles: list[str]
    organization: str


@dataclass(frozen=True)
class WalletKeys:
    """Issuer and holder private keys from testWallet."""

    issuer_private: dict[str, Any]
    wallet_private: dict[str, Any]


@dataclass(frozen=True)
class LoginInitiation:
    """Response from POST /auth/login."""

    state: str
    request_uri: str
    authorize_url: str


@dataclass(frozen=True)
class AuthorizationRequest:
    """Parsed OpenID4VP authorization request object (JAR)."""

    nonce: str
    client_id: str


class AuthService:
    """Handle auth headers and role-based OID4VP token exchange."""

    CLIENT_ID = "dcs-client"
    DEFAULT_ORGANIZATION = "Acme Corp"

    # ------------------------------------------------------------------
    # Step 1 — resolve roles / organization
    # ------------------------------------------------------------------

    @staticmethod
    def parse_auth_credentials(roles: list[str], organization: str | None = None) -> AuthCredentials:
        """Normalize roles and organization for VP issuance."""
        normalized_roles = [role.strip() for role in roles if role and role.strip()]
        if not normalized_roles:
            raise ValueError("roles must contain at least one non-empty role")
        org = (organization or AuthService.DEFAULT_ORGANIZATION).strip()
        if not org:
            org = AuthService.DEFAULT_ORGANIZATION
        return AuthCredentials(
            roles=normalized_roles,
            organization=org,
        )

    # ------------------------------------------------------------------
    # Step 2 — load testWallet keys and build VP JWT
    # ------------------------------------------------------------------

    @staticmethod
    def resolve_wallet_root() -> Path:
        """Locate testWallet root (repo checkout or helm/BDD override)."""
        override = os.getenv("BDD_TEST_WALLET_DIR", "").strip()
        if override:
            return Path(override).expanduser().resolve()
        return Path(__file__).resolve().parents[3] / "testWallet"

    @staticmethod
    def resolve_wallet_keys_dir() -> Path:
        """Locate wallet key directory; defaults to testWallet/keys."""
        override = os.getenv("BDD_TEST_WALLET_KEYS_DIR", "").strip()
        if override:
            return Path(override).expanduser().resolve()
        return AuthService.resolve_wallet_root() / "keys"

    @staticmethod
    def _ensure_dcs_wallet_importable() -> None:
        wallet_root = str(AuthService.resolve_wallet_root())
        if wallet_root not in sys.path:
            sys.path.insert(0, wallet_root)

    @staticmethod
    def load_wallet_keys(keys_dir: Path | None = None) -> WalletKeys:
        """Load issuer-dev.jwk and wallet.jwk used to sign SD-JWT + KB-JWT."""
        keys_path = keys_dir or AuthService.resolve_wallet_keys_dir()
        issuer_path = keys_path / "issuer-dev.jwk"
        wallet_path = keys_path / "wallet.jwk"
        missing = [name for name, path in (
            ("issuer-dev.jwk", issuer_path),
            ("wallet.jwk", wallet_path),
        ) if not path.is_file()]
        if missing:
            raise FileNotFoundError(
                f"missing {', '.join(missing)} in {keys_path} — "
                "run: python3 testWallet/scripts/generate_keys.py --yes"
            )

        AuthService._ensure_dcs_wallet_importable()
        from dcs_wallet.keys import load_json, private_key_material  # noqa: PLC0415

        return WalletKeys(
            issuer_private=private_key_material(load_json(issuer_path)),
            wallet_private=private_key_material(load_json(wallet_path)),
        )

    @staticmethod
    def build_vp_token(
        credentials: AuthCredentials,
        *,
        nonce: str,
        client_id: str,
        keys: WalletKeys | None = None,
    ) -> str:
        """Build vp_token: issuer SD-JWT from roles, then KB-JWT with request aud/nonce."""
        if not nonce:
            raise ValueError("nonce is required to build vp_token")
        if not client_id:
            raise ValueError("client_id is required to build vp_token")

        wallet_keys = keys or AuthService.load_wallet_keys()
        AuthService._ensure_dcs_wallet_importable()
        from dcs_wallet.issuer import (
            DEFAULT_ISSUER_DID,
            attach_key_binding,
            issue_stored_credential,
        )
        from dcs_wallet.status_list import BDD_CREDENTIAL_TENANT

        issuer_did = os.getenv("BDD_ISSUER_DID", DEFAULT_ISSUER_DID)
        statuslist_base = os.getenv("STATUSLIST_SERVICE_URL", "").strip()
        if not statuslist_base:
            raise RuntimeError(
                "STATUSLIST_SERVICE_URL is required for BDD OID4VP credentials "
                "(set by run_bdd_helm.sh; dev uses credentials/*.jwt with localhost:30821)"
            )
        stored_sd_jwt = issue_stored_credential(
            organization=credentials.organization,
            roles=credentials.roles,
            issuer_private=wallet_keys.issuer_private,
            wallet_private=wallet_keys.wallet_private,
            issuer_did=issuer_did,
            statuslist_service_base=statuslist_base,
            statuslist_tenant=BDD_CREDENTIAL_TENANT,
        )
        return attach_key_binding(
            issued_sd_jwt=stored_sd_jwt,
            wallet_private=wallet_keys.wallet_private,
            aud=client_id,
            nonce=nonce,
        )

    # ------------------------------------------------------------------
    # Step 3 — OID4VP login API calls
    # ------------------------------------------------------------------

    @staticmethod
    def initiate_login(session: requests.Session, api_base: str, *, timeout: float) -> LoginInitiation:
        """POST /auth/login and return state, request_uri, and Hydra authorize_url."""
        response = session.post(f"{api_base.rstrip('/')}/auth/login", timeout=timeout)
        response.raise_for_status()
        body = response.json()
        state = body.get("state")
        request_uri = body.get("request_uri")
        authorize_url = body.get("authorize_url")
        if not state or not request_uri or not authorize_url:
            raise RuntimeError(f"/auth/login missing state, request_uri, or authorize_url: {body}")
        return LoginInitiation(
            state=str(state),
            request_uri=str(request_uri),
            authorize_url=str(authorize_url),
        )

    @staticmethod
    def extract_login_challenge(authorize_url: str, *, timeout: float, session: requests.Session | None = None) -> str:
        """Follow Hydra authorize redirect and read login_challenge from the login UI URL."""
        http = session or requests.Session()
        url = authorize_url
        for _ in range(8):
            response = http.get(url, allow_redirects=False, timeout=timeout)
            if response.status_code not in (301, 302, 303, 307, 308):
                raise RuntimeError(
                    f"authorize_url did not redirect to login UI ({response.status_code}): {response.text[:200]}"
                )
            location = response.headers.get("Location", "").strip()
            if not location:
                raise RuntimeError("authorize redirect missing Location header")
            query = parse_qs(urlparse(location).query)
            challenges = query.get("login_challenge") or []
            if challenges and challenges[0].strip():
                return challenges[0].strip()
            url = location
        raise RuntimeError("login_challenge not found in Hydra authorize redirect chain")

    @staticmethod
    def bind_hydra_login_challenge(
        session: requests.Session,
        api_base: str,
        *,
        state: str,
        authorize_url: str,
        timeout: float,
    ) -> None:
        """Bind Hydra login_challenge to the pending OID4VP presentation (browser step, headless)."""
        login_challenge = AuthService.extract_login_challenge(
            authorize_url,
            timeout=timeout,
            session=session,
        )
        response = session.post(
            f"{api_base.rstrip('/')}/auth/login/challenge",
            json={"state": state, "login_challenge": login_challenge},
            timeout=timeout,
        )
        if response.status_code not in (200, 204):
            raise RuntimeError(
                f"/auth/login/challenge failed ({response.status_code}): {response.text[:300]}"
            )

    @staticmethod
    def fetch_authorization_request(
        session: requests.Session,
        request_uri: str,
        *,
        timeout: float,
    ) -> AuthorizationRequest:
        """POST request_uri (JAR) and parse nonce + client_id."""
        response = session.post(
            request_uri,
            timeout=timeout,
            headers={"Accept": "application/oauth-authz-req+jwt, application/jwt"},
        )
        response.raise_for_status()
        jar_token = response.text.strip()
        if not jar_token.startswith("eyJ"):
            raise RuntimeError("authorization request response is not a JWT")

        payload = AuthService.decode_jwt_payload(jar_token)
        nonce = str(payload.get("nonce") or "")
        client_id = str(payload.get("client_id") or AuthService.CLIENT_ID)
        if not nonce:
            raise RuntimeError("authorization request JWT missing nonce")
        return AuthorizationRequest(nonce=nonce, client_id=client_id)

    @staticmethod
    def submit_presentation(
        session: requests.Session,
        api_base: str,
        *,
        state: str,
        vp_token: str,
        timeout: float,
    ) -> str:
        """POST /auth/presentation/callback and return redirect_uri."""
        response = session.post(
            f"{api_base.rstrip('/')}/auth/presentation/callback",
            json={"state": state, "vp_token": vp_token},
            timeout=timeout,
        )
        if response.status_code != 200:
            raise RuntimeError(
                f"/auth/presentation/callback failed ({response.status_code}): {response.text[:300]}"
            )
        body = response.json()
        redirect_uri = body.get("redirect_uri")
        if not redirect_uri:
            raise RuntimeError(f"/auth/presentation/callback missing redirect_uri: {body}")
        return str(redirect_uri)

    @staticmethod
    def resolve_oauth_callback_url(
        session: requests.Session,
        redirect_uri: str,
        api_base: str,
        *,
        timeout: float,
    ) -> str:
        """Follow Hydra redirects after VP accept until the API /auth/callback has ?code=."""
        url = redirect_uri
        for _ in range(12):
            if "consent_challenge" in url:
                parsed = urlparse(url)
                consent_challenge = (parse_qs(parsed.query).get("consent_challenge") or [""])[0]
                if consent_challenge:
                    response = session.get(
                        f"{api_base.rstrip('/')}/auth/consent?consent_challenge={consent_challenge}",
                        allow_redirects=False,
                        timeout=timeout,
                    )
                    if response.status_code not in (301, 302, 303, 307, 308):
                        raise RuntimeError(
                            f"/auth/consent failed ({response.status_code}): {response.text[:300]}"
                        )
                    url = response.headers.get("Location", "")
                    continue

            if url.startswith("http://localhost:5173") or url.startswith("https://localhost:5173"):
                url = AuthService.normalize_callback_url(url, api_base)

            parsed = urlparse(url)
            if parsed.path.endswith("/auth/callback") and parse_qs(parsed.query).get("code"):
                return url

            response = session.get(url, allow_redirects=False, timeout=timeout)
            if response.status_code not in (301, 302, 303, 307, 308):
                raise RuntimeError(
                    f"oauth redirect chain stopped ({response.status_code}) at {url[:160]}: {response.text[:200]}"
                )
            location = response.headers.get("Location", "").strip()
            if not location:
                raise RuntimeError(f"oauth redirect missing Location at {url[:160]}")
            if location.startswith("http://localhost:5173") or location.startswith("https://localhost:5173"):
                location = AuthService.normalize_callback_url(location, api_base)
            parsed = urlparse(location)
            if parsed.path.endswith("/auth/callback") and parse_qs(parsed.query).get("code"):
                return location
            url = location

        raise RuntimeError("oauth redirect chain did not reach /auth/callback?code=")

    @staticmethod
    def complete_session(
        session: requests.Session,
        api_base: str,
        redirect_uri: str,
        *,
        timeout: float,
    ) -> str:
        """Follow OAuth callback and refresh to obtain Hydra access_token."""
        callback_url = AuthService.resolve_oauth_callback_url(
            session,
            redirect_uri,
            api_base,
            timeout=timeout,
        )
        callback_response = session.get(callback_url, allow_redirects=False, timeout=timeout)
        if callback_response.status_code != 302:
            raise RuntimeError(
                f"/auth/callback failed ({callback_response.status_code}): {callback_response.text[:300]}"
            )

        refresh_response = session.post(f"{api_base.rstrip('/')}/auth/refresh", timeout=timeout)
        refresh_response.raise_for_status()
        access_token = refresh_response.json().get("access_token")
        if not access_token:
            raise RuntimeError(f"/auth/refresh missing access_token: {refresh_response.text[:300]}")
        return str(access_token)

    @staticmethod
    def exchange_roles_for_access_token(
        api_base: str,
        roles: list[str],
        *,
        organization: str | None = None,
        timeout: float = 60,
    ) -> str:
        """Full OID4VP headless login: roles → vp_token → access_token."""
        credentials = AuthService.parse_auth_credentials(roles, organization)
        session = requests.Session()
        session.headers.update({
            "User-Agent": "bdd-auth-service",
            "Accept": "application/json",
        })

        initiation = AuthService.initiate_login(session, api_base, timeout=timeout)
        AuthService.bind_hydra_login_challenge(
            session,
            api_base,
            state=initiation.state,
            authorize_url=initiation.authorize_url,
            timeout=timeout,
        )
        auth_request = AuthService.fetch_authorization_request(
            session,
            initiation.request_uri,
            timeout=timeout,
        )
        vp_token = AuthService.build_vp_token(
            credentials,
            nonce=auth_request.nonce,
            client_id=auth_request.client_id,
        )
        redirect_uri = AuthService.submit_presentation(
            session,
            api_base,
            state=initiation.state,
            vp_token=vp_token,
            timeout=timeout,
        )
        return AuthService.complete_session(
            session,
            api_base,
            redirect_uri,
            timeout=timeout,
        )


    # ------------------------------------------------------------------
    # Step 4 — set context headers for Behave steps
    # ------------------------------------------------------------------

    @staticmethod
    def set_headers_for_roles(
        context,
        roles: list[str],
        username_prefix: str = "bdd",
        use_expired_jwt: bool = False,
        organization: str | None = None,
    ):
        """Set context.headers with a bearer token for the given roles."""
        del username_prefix  # OID4VP login derives identity from VP, not BDD username.
        # TBD: extend issue_credentials.py to support generating expired JWT credentials
        if use_expired_jwt:
            username = AuthService.username_for_roles(roles, "bdd")
            token = AuthService.create_expired_jwt(AuthService.CLIENT_ID, username, roles)
        else:
            api_base = getattr(context, "base_url", os.getenv("BDD_DCS_BASE_URL", "http://127.0.0.1:8991"))
            timeout = float(getattr(context, "http_timeout_seconds", os.getenv("BDD_HTTP_TIMEOUT_SECONDS", "60")))
            token = AuthService.exchange_roles_for_access_token(
                api_base,
                roles,
                organization=organization,
                timeout=timeout,
            )
        context.headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }

    @staticmethod
    def get_headers_for_roles(
        roles: list[str],
        username_prefix: str = "bdd",
        use_expired_jwt: bool = False,
        organization: str | None = None,
        api_base: str | None = None,
        timeout: float = 60,
    ) -> dict:
        """Return auth headers for a given role (without modifying context)."""
        if use_expired_jwt:
            username = AuthService.username_for_roles(roles, username_prefix)
            token = AuthService.create_expired_jwt(AuthService.CLIENT_ID, username, roles)
        else:
            token = AuthService.exchange_roles_for_access_token(
                api_base or os.getenv("BDD_DCS_BASE_URL", "http://127.0.0.1:8991"),
                roles,
                organization=organization,
                timeout=timeout,
            )
        return {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }

    # ------------------------------------------------------------------
    # Helpers (JWT decode, legacy expired-token path, username)
    # ------------------------------------------------------------------

    @staticmethod
    def decode_jwt_payload(token: str) -> dict[str, Any]:
        parts = token.strip().split(".")
        if len(parts) < 2:
            raise ValueError("token is not a compact JWT")
        payload = parts[1]
        padding = "=" * (-len(payload) % 4)
        data = json.loads(base64.urlsafe_b64decode(payload + padding).decode())
        if not isinstance(data, dict):
            raise ValueError("JWT payload must be a JSON object")
        return data

    @staticmethod
    def normalize_callback_url(redirect_uri: str, api_base: str) -> str:
        """Map Vite (:5173) OAuth callback URLs to the BDD API base when using local air."""
        api = urlparse(api_base)
        target = urlparse(redirect_uri)
        if target.netloc.endswith(":5173"):
            api_origin = f"{api.scheme}://{api.netloc}"
            return redirect_uri.replace(f"{target.scheme}://{target.netloc}", api_origin, 1)
        return redirect_uri

    # TBD: credential didn't contain any username info.
    @staticmethod
    def username_for_roles(roles: list[str], username_prefix: str = "bdd") -> str:
        """Convert role names to a deterministic BDD username."""
        role_safe = [
            re.sub(r"[^A-Za-z0-9]+", "-", role.lower()).strip("-")
            for role in roles
            if role and role.strip()
        ]
        return f"{username_prefix}-{'-'.join(role_safe)}"

    @staticmethod
    def create_expired_jwt(client_id, username, roles):
        """Create an expired JWT token for negative credential tests."""
        header = {"alg": "none"}
        payload = {
            "sub": username,
            "iss": "https://auth.eclipse.org/auth/realms/community",
            "azp": client_id,
            "resource_access": {"dcs-client": {"roles": roles}},
            "exp": int(time.time()) - 3600,
        }

        encoded_header = base64.urlsafe_b64encode(json.dumps(header).encode()).decode().rstrip("=")
        encoded_payload = base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip("=")
        return f"{encoded_header}.{encoded_payload}."
