"""BDD steps for the Process Audit & Compliance Management endpoints (UC-08,
backend/design/process_audit_and_compliance.go): /pac/audit, /pac/report
(GET report + POST incident), /pac/monitor.
"""

from behave import then, when

from steps.support.api_client import pac_audit_url, pac_monitor_url, pac_report_url, post_json
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when('the Auditor triggers a process audit with scope "{scope}"')
def step_when_auditor_triggers_audit(context, scope):
    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = post_json(context, pac_audit_url(context), {"scope": scope}, headers=headers)


@when('I attempt to trigger a process audit with scope "{scope}"')
def step_when_attempt_trigger_audit(context, scope):
    headers = getattr(context, "headers", {})
    context.requests_response = post_json(context, pac_audit_url(context), {"scope": scope}, headers=headers)


@when('the Auditor requests an audit report for scope "{scope}" in format "{fmt}"')
def step_when_auditor_requests_report(context, scope, fmt):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = _requests.get(
        pac_report_url(context),
        params={"scope": scope, "format": fmt},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when("the Compliance Officer requests continuous monitoring")
def step_when_compliance_officer_monitors(context):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Compliance Officer"])
    context.requests_response = _requests.get(
        pac_monitor_url(context), headers=headers, timeout=context.http_timeout_seconds
    )


@when("the Compliance Officer submits a non-compliance incident report")
def step_when_compliance_officer_submits_incident(context):
    headers = AuthService.get_headers_for_roles(["Compliance Officer"])
    context.requests_response = post_json(context, pac_report_url(context), {}, headers=headers)


@then('the process audit response includes an audit trail entry for contract "{name}"')
def step_then_audit_response_includes_contract(context, name):
    # The audit trail is anchored asynchronously by the outbox processor
    # (TSA+IPFS per event) — a contract created moments before the audit
    # trigger may not be anchored yet when the first snapshot is taken. Same
    # polling convention as the contract-audit steps: re-trigger the audit
    # until the entry appears or the deadline expires.
    import time  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    all_dids = []
    deadline = time.monotonic() + 90
    while True:
        body = context.requests_response.json()
        assert isinstance(body, list) and body, f"Expected a non-empty PACAuditResponse list, got: {body}"
        all_dids = [
            entry.get("did")
            for scope_result in body
            for entry in (scope_result.get("audit_trail") or [])
            if isinstance(entry, dict)
        ]
        if did in all_dids:
            return
        if time.monotonic() > deadline:
            break
        time.sleep(2)
        context.requests_response = post_json(
            context, pac_audit_url(context), {"scope": "CONTRACT_WORKFLOW_ENGINE"}, headers=headers
        )
        assert context.requests_response.status_code == 200, (
            f"process audit re-trigger failed: {context.requests_response.status_code} "
            f"{context.requests_response.text}"
        )
    assert did in all_dids, (
        f"Expected the CONTRACT_WORKFLOW_ENGINE audit trail to include an entry for contract "
        f"'{name}' (did={did}), got dids: {all_dids}"
    )
