"""Seeds the E2E fixtures through the instance's public API, reusing the BDD
suite's service helpers (which need only base_url/http_timeout_seconds from
the behave context). Prints a JSON object consumed by e2e/global-setup.ts.

Usage: python3 e2e/seed_fixtures.py <api_base>
"""

import json
import os
import sys
import uuid
from types import SimpleNamespace

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
sys.path.insert(0, REPO_ROOT)

from steps.support import localhost_resolver  # noqa: E402

localhost_resolver.install()

from steps.support.api_client import (  # noqa: E402
    contract_create_url,
    contract_update_url,
    post_json,
    put_json,
    template_approve_url,
    template_create_url,
    template_register_url,
)
from steps.support.services import odrl_fixture_service as odrl  # noqa: E402
from steps.support.services.auth_service import AuthService  # noqa: E402
from steps.support.services.contract_service import ContractService  # noqa: E402
from steps.support.services.template_service import TemplateService  # noqa: E402

DCS_NS = "https://w3id.org/facis/dcs/ontology/v1#"
XSD_INTEGER = "http://www.w3.org/2001/XMLSchema#integer"


def seed_typed_clause_contract(context) -> str:
    """A registered contract template carrying a hub typed clause
    (dcs:PaymentClause) and a draft contract derived from it — the fixture
    the typed-clause fill spec edits through the real shacl-form UI."""
    block_id = f"urn:uuid:block-{uuid.uuid4()}"
    root_id = f"urn:uuid:block-{uuid.uuid4()}"
    instance = {
        "@id": f"urn:uuid:{uuid.uuid4()}",
        "@type": DCS_NS + "PaymentClause",
        DCS_NS + "amount": {"@value": "100", "@type": XSD_INTEGER},
        DCS_NS + "currency": "EUR",
    }
    template_data = {
        "@type": "dcs:ContractTemplate",
        "dcs:metadata": {
            "@type": "dcs:TemplateMetadata",
            "dcs:title": "E2E Typed Clause Template",
            "dcs:templateType": "dcs:ContractTemplate",
        },
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {
                "@list": [
                    {
                        "@type": "dcs:Clause",
                        "@id": block_id,
                        "dcs:title": "Payment terms",
                        "dcs:content": {"@list": ["amount: 100 · currency: EUR"]},
                        "dcs:typedClause": instance,
                    }
                ]
            },
            "dcs:layout": [
                {
                    "@id": root_id,
                    "@type": "dcs:LayoutNode",
                    "dcs:isRoot": True,
                    "dcs:children": {"@list": [{"@id": block_id}]},
                }
            ],
        },
        "dcs:contractData": [],
        "dcs:policies": {
            "@id": "urn:uuid:policy-set",
            "@type": "odrl:Offer",
            "odrl:profile": {"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
        },
    }
    creator = AuthService.get_headers_for_roles(["Template Creator"])
    created = post_json(
        context,
        template_create_url(context),
        {
            "name": "E2E Typed Clause Template",
            "description": "E2E typed clause fixture",
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "template_data": template_data,
        },
        headers=creator,
    )
    assert created.status_code in (200, 201), f"typed template create failed: {created.text}"
    t_did = created.json()["did"]
    updated_at = TemplateService.fetch_template(context, t_did, headers=creator)["updated_at"]
    updated_at = TemplateService.do_submit(context, t_did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, t_did, updated_at)
    approver = AuthService.get_headers_for_roles(["Template Approver"])
    approved = post_json(
        context, template_approve_url(context), {"did": t_did, "updated_at": updated_at}, headers=approver
    )
    assert approved.status_code == 200, f"typed template approve failed: {approved.text}"
    manager = AuthService.get_headers_for_roles(["Template Manager"])
    registered = post_json(context, template_register_url(context), {"did": t_did}, headers=manager)
    assert registered.status_code == 200, f"typed template register failed: {registered.text}"

    contract_creator = AuthService.get_headers_for_roles(["Contract Creator"])
    peer = ContractService._local_peer_did(context)
    created_contract = post_json(
        context,
        contract_create_url(context),
        {"template_did": t_did, "reviewers": [peer], "negotiators": [peer], "approvers": [peer]},
        headers=contract_creator,
    )
    assert created_contract.status_code == 200, f"typed contract create failed: {created_contract.text}"
    return created_contract.json()["did"]


def main() -> None:
    api_base = sys.argv[1]
    os.environ.setdefault("BDD_DCS_BASE_URL", api_base)
    context = SimpleNamespace(base_url=api_base, http_timeout_seconds=60)

    template_did, _ = TemplateService.create_approved_template(context)

    contract_name = "E2E UI Fixture Contract"
    ContractService._create_contract_in_draft(context, contract_name)
    contract_did = context.contract_dids[contract_name]

    # Give the draft a fillable requirement field bound into an ODRL
    # constraint (the same canonical fixture the BDD ODRL scenarios use), so
    # UI specs can exercise the semantic-value fill flow.
    policies = odrl.odrl_set_policies(contract_did, "coverage", "gteq", 95)
    document = odrl.build_contract_document(contract_did, "coverage", policies, 99)
    update = put_json(
        context,
        contract_update_url(context),
        {
            "did": contract_did,
            "updated_at": context.contract_updated_at[contract_name],
            "contract_data": document,
        },
        headers=context.contract_seed_headers[contract_name],
    )
    assert update.status_code == 200, f"fixture contract update failed: {update.text}"

    typed_contract_did = seed_typed_clause_contract(context)

    print(
        json.dumps(
            {
                "templateDid": template_did,
                "contractDid": contract_did,
                "contractName": contract_name,
                "typedContractDid": typed_contract_did,
            }
        )
    )


if __name__ == "__main__":
    main()
