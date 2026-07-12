"""BDD steps for the Contract Storage & Archive endpoints (UC-07,
backend/design/contract_storage_archive.go): /archive/retrieve,
/archive/search, /archive/audit. Archive-entry creation/evidence content is
covered by 05_contract_deployment/contract_deployment.feature — this module
only exercises the archive endpoints themselves.
"""

from behave import given, then, when

from steps.support.api_client import (
    archive_audit_url,
    archive_retrieve_url,
    archive_search_url,
    get_with_headers,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when("the Archive Manager retrieves the archive")
def step_when_archive_manager_retrieves(context):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = get_with_headers(context, archive_retrieve_url(context), headers=headers)


@when('the Archive Manager searches the archive with state filter "{state}"')
def step_when_archive_manager_searches(context, state):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"state": state},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when("I attempt to retrieve the archive with my current role")
def step_when_attempt_retrieve_archive(context):
    headers = getattr(context, "headers", {})
    context.requests_response = get_with_headers(context, archive_retrieve_url(context), headers=headers)


@when("the Auditor retrieves the archive audit log")
def step_when_auditor_retrieves_archive_audit(context):
    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = get_with_headers(context, archive_audit_url(context), headers=headers)


def _contract_dids_in_response(context, name):
    body = context.requests_response.json()
    entries = body.get("contracts") if isinstance(body, dict) else body
    assert isinstance(entries, list), f"Expected a list of archive entries, got: {body}"
    did, _ = ContractService._contract_data(context, name)
    return entries, did


@then('the archive retrieval result includes contract "{name}"')
def step_then_archive_retrieval_includes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did in dids, f"Expected archive retrieval to include contract '{name}' (did={did}), got dids: {dids}"


@then('the archive search result includes contract "{name}"')
def step_then_archive_search_includes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did in dids, f"Expected archive search to include contract '{name}' (did={did}), got dids: {dids}"


@then("the archive audit log is a non-empty list")
def step_then_archive_audit_nonempty(context):
    body = context.requests_response.json()
    assert isinstance(body, list) and len(body) > 0, (
        f"Expected /archive/audit to return a non-empty list of audit log entries, got: {body}"
    )
