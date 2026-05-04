import re

from steps.support.api_client import get_with_headers, post_json, template_approve_url, template_create_url, template_retrieve_by_id_url, template_submit_url, template_verify_url
from steps.support.services.auth_service import AuthService


class TemplateService: 
    """Service class for template-related operations."""

    @staticmethod
    def template_env_key(name: str) -> str:
        normalized = re.sub(r"[^A-Za-z0-9]+", "_", name).strip("_").upper()
        return f"BDD_TEMPLATE_DID_{normalized}"


    @staticmethod
    def template_type_for_category(category: str) -> str:
        category_key = category.strip().lower()
        return {
            "legal": "FRAME_CONTRACT",
            "procurement": "SUB_CONTRACT",
        }.get(category_key, category.strip().upper().replace(" ", "_"))


    @staticmethod
    def create_fresh_template(context) -> tuple:
        """Create a Draft template as Template Creator; return (did, updated_at)."""
        headers = AuthService.headers_for_role(context, "Template Creator")
        payload = {
            "template_type": TemplateService.template_type_for_category("Legal"),
            "name": "BDD Standard NDA",
            "description": "BDD auto-created template",
            "template_data": {
                "title": "BDD Standard NDA",
                "clauses": [{"id": "c1", "text": "Confidentiality clause"}],
            },
        }
        resp = post_json(context, template_create_url(context), payload, headers=headers)
        assert resp.status_code == 200, f"Template create failed: {resp.text}"
        did = resp.json().get("did")
        assert did, f"No DID in create response: {resp.text}"
        body = TemplateService.fetch_template(context, did, headers=headers)
        return did, body.get("updated_at")
        
    @staticmethod
    def create_approved_template(context) -> tuple:
        """Create and approve a template; return (did, updated_at)."""
        did, updated_at = TemplateService.create_fresh_template(context)
        updated_at = TemplateService.do_submit(context, did, updated_at)
        updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
        approver_headers = AuthService.headers_for_role(context, "Template Approver")
        approve_resp = post_json(
            context,
            template_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=approver_headers,
        )
        assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
        updated_at = TemplateService.fetch_template(context, did, headers=approver_headers).get("updated_at")
        return did, updated_at

    @staticmethod
    def fetch_template(context, did: str, headers=None) -> dict:
        resp = get_with_headers(context, template_retrieve_by_id_url(context, did), headers=headers)
        assert resp.status_code == 200, f"Template retrieve failed: {resp.text}"
        return resp.json()

    @staticmethod
    def template_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.headers_for_role(context, "Template Reviewer")
        AuthService.headers_for_role(context, "Template Approver")
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [AuthService.username_for_role("Template Reviewer")],
            "approver": AuthService.username_for_role("Template Approver"),
        }

    @staticmethod
    def template_reviewer_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.headers_for_role(context, "Template Approver")
        return {
            "did": did,
            "updated_at": updated_at,
            "approver": AuthService.username_for_role("Template Approver"),
            "forward_to": "approval",
        }

    @staticmethod
    def do_submit(context, did: str, updated_at: str) -> str:
        """Submit template as Template Creator; return refreshed updated_at."""
        headers = AuthService.headers_for_role(context, "Template Creator")
        resp = post_json(
            context,
            template_submit_url(context),
            TemplateService.template_submit_payload(context, did, updated_at),
            headers=headers,
        )
        assert resp.status_code == 200, f"Template submit failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def do_verify(context, did: str, updated_at: str) -> str:
        """Verify template content as Template Reviewer; return refreshed updated_at."""
        headers = AuthService.headers_for_role(context, "Template Reviewer")
        resp = post_json(context, template_verify_url(context), {"did": did, "updated_at": updated_at}, headers=headers)
        assert resp.status_code == 200, f"Template verify failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def do_recommend_for_approval(context, did: str, updated_at: str) -> str:
        """Submit reviewer recommendation and advance review workflow."""
        # Backend requires verification before reviewer recommendation submit.
        updated_at = TemplateService.do_verify(context, did, updated_at)
        headers = AuthService.headers_for_role(context, "Template Reviewer")
        resp = post_json(
            context,
            template_submit_url(context),
            TemplateService.template_reviewer_submit_payload(context, did, updated_at),
            headers=headers,
        )
        assert resp.status_code == 200, f"Template review submit failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def named(context, name: str) -> dict:
        return (getattr(context, "named_templates", None) or {}).get(name, {})

    @staticmethod
    def store_named(context, name: str, did: str, updated_at: str):
        if not hasattr(context, "named_templates") or context.named_templates is None:
            context.named_templates = {}
        context.named_templates[name] = {"did": did, "updated_at": updated_at}