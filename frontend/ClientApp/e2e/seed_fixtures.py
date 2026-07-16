"""Seeds the E2E fixtures through the instance's public API, reusing the BDD
suite's service helpers (which need only base_url/http_timeout_seconds from
the behave context). Prints a JSON object consumed by e2e/global-setup.ts.

Usage: python3 e2e/seed_fixtures.py <api_base>
"""

import json
import os
import sys
from types import SimpleNamespace

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
sys.path.insert(0, REPO_ROOT)

from steps.support.api_client import contract_update_url, put_json  # noqa: E402
from steps.support.services import odrl_fixture_service as odrl  # noqa: E402
from steps.support.services.contract_service import ContractService  # noqa: E402
from steps.support.services.template_service import TemplateService  # noqa: E402


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

    print(
        json.dumps(
            {
                "templateDid": template_did,
                "contractDid": contract_did,
                "contractName": contract_name,
            }
        )
    )


if __name__ == "__main__":
    main()
