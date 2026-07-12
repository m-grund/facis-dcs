"""Contract service API client for test steps."""

import requests

from steps.support.api_client import (
    contract_create_url,
    contract_retrieve_by_id_url,
    contract_submit_url,
    contract_verify_url,
    get_with_headers,
    post_json,
    template_approve_url,
    template_create_url,
    template_register_url,
    template_submit_url,
    template_verify_url
)
from steps.support.services.auth_service import AuthService
from steps.support.services.template_service import TemplateService


class ContractService:
    """Contract service API client."""
    @staticmethod
    def _ensure_store(context, name, value):
        if not hasattr(context, name) or getattr(context, name) is None:
            setattr(context, name, value)

    @staticmethod
    def _local_peer_did(context):
        """Reviewers/Negotiators/Approvers on contract/create and
        contract/submit are peer DIDs (other DCS instances), not usernames
        — see backend/internal/contractworkflowengine/command/create.go and
        the CauserDID-based IsValidReviewer/IsValidNegotiator checks in
        submit.go. For a single-instance BDD run the only peer that can ever
        act (CauserDID is always this instance's own DID server-side) is
        this instance itself, fetched from its own did:web document.
        """
        if not hasattr(context, "local_peer_did_cache"):
            from steps.support.api_client import did_document_url  # noqa: PLC0415

            resp = requests.get(
                did_document_url(context.base_url),
                timeout=context.http_timeout_seconds,
            )
            assert resp.status_code == 200, (
                f"could not fetch this instance's own did:web document: "
                f"{resp.status_code} {resp.text}"
            )
            did = resp.json().get("id")
            assert did, f"own did.json response has no 'id' field: {resp.text}"
            context.local_peer_did_cache = did
        return context.local_peer_did_cache

    @staticmethod
    def _template_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Template Reviewer"])
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [AuthService.username_for_roles(["Template Reviewer"])],
            "approver": AuthService.username_for_roles(["Template Approver"]),
        }

    @staticmethod
    def _template_reviewer_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "approver": AuthService.username_for_roles(["Template Approver"]),
            "forward_to": "approval",
        }

    @staticmethod
    def _contract_submit_payload(context, did: str, updated_at: str) -> dict:
        peer_did = ContractService._local_peer_did(context)
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [peer_did],
            "approvers": [peer_did],
            "negotiators": [peer_did],
        }

    @staticmethod
    def _contract_reviewer_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Contract Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "forward_to": "approval",
        }

    @staticmethod
    def _create_approved_template_for_contract(context):
        creator_h = AuthService.get_headers_for_roles(["Template Creator"])
        create_resp = post_json(
            context,
            template_create_url(context),
            {
                "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
                "name": "BDD Contract Source Template",
                "description": "BDD template for contract workflows",
                "template_data": {
                    "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
                    "@type": "dcs:ContractTemplate",
                    "dcs:metadata": {
                        "@type": "dcs:TemplateMetadata",
                        "dcs:title": "BDD Contract Source Template",
                    },
                    "dcs:documentStructure": {
                        "@type": "dcs:DocumentStructure",
                        "dcs:blocks": {
                            "@list": [
                                {
                                    "@id": "urn:uuid:block-clause-1",
                                    "@type": "dcs:Clause",
                                    "dcs:content": {"@list": ["Base clause"]},
                                }
                            ]
                        },
                        "dcs:layout": [
                            {
                                "@id": "urn:uuid:block-root",
                                "dcs:isRoot": True,
                                "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                            }
                        ],
                    },
                },
            },
            headers=creator_h,
        )
        assert create_resp.status_code == 200, create_resp.text
        t_did = create_resp.json().get("did")

        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        updated_at = retrieve_resp.json().get("updated_at")

        submit_resp = post_json(
            context,
            template_submit_url(context),
            ContractService._template_submit_payload(context, t_did, updated_at),
            headers=creator_h,
        )
        assert submit_resp.status_code == 200, submit_resp.text

        reviewer_h = AuthService.get_headers_for_roles(["Template Reviewer"])
        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
        updated_at = retrieve_resp.json().get("updated_at")

        verify_resp = post_json(
            context,
            template_verify_url(context),
            {"did": t_did, "updated_at": updated_at},
            headers=reviewer_h,
        )
        assert verify_resp.status_code == 200, verify_resp.text

        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
        updated_at = retrieve_resp.json().get("updated_at")

        review_submit_resp = post_json(
            context,
            template_submit_url(context),
            ContractService._template_reviewer_submit_payload(context, t_did, updated_at),
            headers=reviewer_h,
        )
        assert review_submit_resp.status_code == 200, review_submit_resp.text

        approver_h = AuthService.get_headers_for_roles(["Template Approver"])
        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=approver_h)
        updated_at = retrieve_resp.json().get("updated_at")
        approve_resp = post_json(
            context,
            template_approve_url(context),
            {"did": t_did, "updated_at": updated_at},
            headers=approver_h,
        )
        assert approve_resp.status_code == 200, approve_resp.text

        # contract/create only accepts templates in state REGISTERED or
        # PUBLISHED (see ReadContractTemplateDataByID) — APPROVED alone is
        # not enough, register is a distinct step after approval.
        manager_h = AuthService.get_headers_for_roles(["Template Manager"])
        register_resp = post_json(
            context,
            template_register_url(context),
            {"did": t_did},
            headers=manager_h,
        )
        assert register_resp.status_code == 200, register_resp.text
        return t_did


    @staticmethod
    def _create_contract_in_draft(context, contract_name: str):
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
        peer_did = ContractService._local_peer_did(context)
        create_payload = {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        }
        create_resp = post_json(context, contract_create_url(context), create_payload, headers=creator_h)
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")
        retrieve_resp = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        updated_at = retrieve_resp.json().get("updated_at")

        ContractService._ensure_store(context, "contract_dids", {})
        ContractService._ensure_store(context, "contract_updated_at", {})
        ContractService._ensure_store(context, "contract_seed_headers", {})
        context.contract_dids[contract_name] = c_did
        context.contract_updated_at[contract_name] = updated_at
        context.contract_seed_headers[contract_name] = creator_h

    @staticmethod
    def _create_contract_in_negotiation(context, contract_name: str):
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
        peer_did = ContractService._local_peer_did(context)
        create_payload = {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        }
        create_resp = post_json(context, contract_create_url(context), create_payload, headers=creator_h)
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")

        retrieve_resp = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        updated_at = retrieve_resp.json().get("updated_at")

        submit_payload = ContractService._contract_submit_payload(context, c_did, updated_at)
        retrieve_resp = post_json(context, contract_submit_url(context), submit_payload, headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text

        retrieve_resp = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        assert retrieve_resp.json().get("state") == "NEGOTIATION", \
            f'Contract should be in NEGOTIATION state, but it is {retrieve_resp.json().get("state")}'
        updated_at = retrieve_resp.json().get("updated_at")

        ContractService._ensure_store(context, "contract_dids", {})
        ContractService._ensure_store(context, "contract_updated_at", {})
        ContractService._ensure_store(context, "contract_seed_headers", {})
        context.contract_dids[contract_name] = c_did
        context.contract_updated_at[contract_name] = updated_at
        context.contract_seed_headers[contract_name] = creator_h


    @staticmethod
    def _create_approved_template_with_signature_field(context, signatory_name: str) -> str:
        """Like _create_approved_template_for_contract, but the template
        declares a named `dcs:signatureFields` entry (dcs:SignatureField,
        dcs:signatoryName) — the field pdf-core's PAdES signer is expected to
        bind its /T AcroForm field name to (docs/anforderung.md B1: "existing
        signature field by name (/T == signatoryName from the JSON-LD)").
        See docs/semantic-ontology/linkml/tests/valid/signature-fields.jsonld
        for the schema shape this mirrors.
        """
        creator_h = AuthService.get_headers_for_roles(["Template Creator"])
        create_resp = post_json(
            context,
            template_create_url(context),
            {
                "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
                "name": "BDD Signature-Field Source Template",
                "description": "BDD template for real-signing-vertical scenarios",
                "template_data": {
                    "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
                    "@type": "dcs:ContractTemplate",
                    "dcs:metadata": {
                        "@type": "dcs:TemplateMetadata",
                        "dcs:title": "BDD Signature-Field Source Template",
                    },
                    "dcs:signatureFields": [
                        {
                            "@id": "urn:uuid:sig-field-1",
                            "@type": "dcs:SignatureField",
                            "dcs:signatoryName": signatory_name,
                        }
                    ],
                    "dcs:documentStructure": {
                        "@type": "dcs:DocumentStructure",
                        "dcs:blocks": {
                            "@list": [
                                {
                                    "@id": "urn:uuid:block-clause-1",
                                    "@type": "dcs:Clause",
                                    "dcs:content": {"@list": ["Base clause"]},
                                }
                            ]
                        },
                        "dcs:layout": [
                            {
                                "@id": "urn:uuid:block-root",
                                "dcs:isRoot": True,
                                "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                            }
                        ],
                    },
                },
            },
            headers=creator_h,
        )
        assert create_resp.status_code == 200, create_resp.text
        t_did = create_resp.json().get("did")

        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        updated_at = retrieve_resp.json().get("updated_at")

        submit_resp = post_json(
            context,
            template_submit_url(context),
            ContractService._template_submit_payload(context, t_did, updated_at),
            headers=creator_h,
        )
        assert submit_resp.status_code == 200, submit_resp.text

        reviewer_h = AuthService.get_headers_for_roles(["Template Reviewer"])
        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
        updated_at = retrieve_resp.json().get("updated_at")

        verify_resp = post_json(
            context,
            template_verify_url(context),
            {"did": t_did, "updated_at": updated_at},
            headers=reviewer_h,
        )
        assert verify_resp.status_code == 200, verify_resp.text

        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
        updated_at = retrieve_resp.json().get("updated_at")

        review_submit_resp = post_json(
            context,
            template_submit_url(context),
            ContractService._template_reviewer_submit_payload(context, t_did, updated_at),
            headers=reviewer_h,
        )
        assert review_submit_resp.status_code == 200, review_submit_resp.text

        approver_h = AuthService.get_headers_for_roles(["Template Approver"])
        retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=approver_h)
        updated_at = retrieve_resp.json().get("updated_at")
        approve_resp = post_json(
            context,
            template_approve_url(context),
            {"did": t_did, "updated_at": updated_at},
            headers=approver_h,
        )
        assert approve_resp.status_code == 200, approve_resp.text

        manager_h = AuthService.get_headers_for_roles(["Template Manager"])
        register_resp = post_json(
            context,
            template_register_url(context),
            {"did": t_did},
            headers=manager_h,
        )
        assert register_resp.status_code == 200, register_resp.text
        return t_did

    @staticmethod
    def _create_contract_in_draft_with_signature_field(context, contract_name: str, signatory_name: str):
        """Like _create_contract_in_draft, but sourced from a template that
        carries a named dcs:SignatureField (see
        _create_approved_template_with_signature_field) — used by the
        real-signing-vertical (Workstream B) scenarios that assert on the
        PAdES-signed PDF's AcroForm /T field name.
        """
        t_did = ContractService._create_approved_template_with_signature_field(context, signatory_name)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
        peer_did = ContractService._local_peer_did(context)
        create_payload = {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        }
        create_resp = post_json(context, contract_create_url(context), create_payload, headers=creator_h)
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")
        retrieve_resp = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve_resp.status_code == 200, retrieve_resp.text
        updated_at = retrieve_resp.json().get("updated_at")

        ContractService._ensure_store(context, "contract_dids", {})
        ContractService._ensure_store(context, "contract_updated_at", {})
        ContractService._ensure_store(context, "contract_seed_headers", {})
        context.contract_dids[contract_name] = c_did
        context.contract_updated_at[contract_name] = updated_at
        context.contract_seed_headers[contract_name] = creator_h

    @staticmethod
    def _contract_data(context, contract_name: str):
        did = context.contract_dids[contract_name]
        updated_at = context.contract_updated_at[contract_name]
        return did, updated_at

    @staticmethod
    def _refresh_contract(context, contract_name: str):
        did = context.contract_dids[contract_name]
        headers = None
        if hasattr(context, "contract_seed_headers"):
            headers = context.contract_seed_headers.get(contract_name)
        resp = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
        assert resp.status_code == 200, resp.text
        context.contract_updated_at[contract_name] = resp.json().get("updated_at")
        return resp.json()

    @staticmethod
    def _prepare_contract_under_review(context, contract_name: str):
        did, updated_at = ContractService._contract_data(context, contract_name)
        creator_h = context.contract_seed_headers[contract_name]
        submit_to_negotiation = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_submit_payload(context, did, updated_at),
            headers=creator_h,
        )
        assert submit_to_negotiation.status_code == 200, submit_to_negotiation.text
        ContractService._refresh_contract(context, contract_name)

        # Backend workflow transitions Draft -> Negotiation on first submit,
        # then Negotiation -> Submitted on a second creator submit.
        did, updated_at = ContractService._contract_data(context, contract_name)
        submit_to_submitted = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_submit_payload(context, did, updated_at),
            headers=creator_h,
        )
        assert submit_to_submitted.status_code == 200, submit_to_submitted.text
        ContractService._refresh_contract(context, contract_name)

    @staticmethod
    def _prepare_contract_pending_approval(context, contract_name: str):
        did, _ = ContractService._contract_data(context, contract_name)
        ContractService._prepare_contract_under_review(context, contract_name)

        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"])
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        verify = post_json(
            context,
            contract_verify_url(context),
            {"did": did, "updated_at": updated_at},
            headers=reviewer_h,
        )
        assert verify.status_code == 200, verify.text

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        review_submit = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_reviewer_submit_payload(context, did, updated_at),
            headers=reviewer_h,
        )
        assert review_submit.status_code == 200, review_submit.text
        ContractService._refresh_contract(context, contract_name)
