"""Authentication and authorization service for BDD steps."""

import base64
import json
import re


class AuthService:
    """Handle auth headers and role-based token generation."""
    
    CLIENT_ID = "dcs-client"
    
    @staticmethod
    def set_headers_for_role(context, role: str, username_prefix: str = "bdd"):
        """Set context.headers for a given role."""
        username = AuthService.username_for_role(role, username_prefix)
        token = AuthService.create_custom_jwt(AuthService.CLIENT_ID, username, role)
        context.headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }
    
    @staticmethod
    def headers_for_role(role: str, username_prefix: str = "bdd") -> dict:
        """Return auth headers for a given role (without modifying context)."""
        username = AuthService.username_for_role(role, username_prefix)
        token = AuthService.create_custom_jwt(AuthService.CLIENT_ID, username, role)
        return {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json"
        }
    
    @staticmethod
    def username_for_role(role: str, username_prefix: str = "bdd") -> str:
        """Convert role name to BDD username."""
        role_safe = re.sub(r"[^A-Za-z0-9]+", "-", role.lower()).strip("-")
        return f"{username_prefix}-{role_safe}"
    
    @staticmethod
    def create_custom_jwt(client_id, username, role):
        """Create a JWT token for testing."""
        header = {"alg": "none"}
        payload = {
            "sub": username,
            "iss": "https://auth.eclipse.org/auth/realms/community",
            "azp": client_id,
            "resource_access": {"dcs-client": {"roles": [role]}},
            "exp": 9999999999
        }
        
        encoded_header = base64.urlsafe_b64encode(json.dumps(header).encode()).decode().rstrip("=")
        encoded_payload = base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip("=")
        
        token = f"{encoded_header}.{encoded_payload}."
        return token
