"""BDD steps for the Process Audit & Compliance Management endpoints (UC-08,
backend/design/process_audit_and_compliance.go): /pac/audit, /pac/report
(GET report + POST incident), /pac/monitor.
"""

from behave import given, then, when

from steps.support.api_client import (
    contract_retrieve_url,
    get_with_headers,
    pac_audit_url,
    pac_monitor_url,
    pac_report_url,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when('the Auditor triggers a process audit with scope "{scope}"')
def step_when_auditor_triggers_audit(context, scope):
    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = post_json(context, pac_audit_url(context), {"scope": scope, "justification": "BDD process audit"}, headers=headers)


@when('I attempt to trigger a process audit with scope "{scope}"')
def step_when_attempt_trigger_audit(context, scope):
    headers = getattr(context, "headers", {})
    context.requests_response = post_json(context, pac_audit_url(context), {"scope": scope, "justification": "BDD process audit"}, headers=headers)


@when('the Auditor requests an audit report for scope "{scope}" in format "{fmt}"')
def step_when_auditor_requests_report(context, scope, fmt):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = _requests.get(
        pac_report_url(context),
        params={"scope": scope, "format": fmt, "justification": "BDD audit report"},
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


@given('contract "{name}" still has an open required approval task')
def step_given_open_approval_task(context, name):
    """Asserts the precondition the monitor sweep is supposed to flag: the
    contract (driven to REVIEWED = pending approval by the previous Given)
    still carries an OPEN approval task, observed via GET /contract/retrieve's
    approval_tasks list. Approvers are responsible peers (see
    backend/internal/contractworkflowengine/db package doc), so the missing
    approval is attributed to a peer DID, not an individual user."""
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Approver"])
    resp = get_with_headers(context, contract_retrieve_url(context), headers=headers)
    assert resp.status_code == 200, f"contract retrieve failed: {resp.status_code} {resp.text}"
    tasks = resp.json().get("approval_tasks") or []
    open_tasks = [
        t for t in tasks
        if t.get("did") == did and str(t.get("state", "")).upper() == "OPEN"
    ]
    assert open_tasks, (
        f"Expected contract '{name}' (did={did}) to still have an OPEN approval task "
        f"as the monitoring precondition, got approval tasks: {tasks}"
    )


@then('the monitoring sweep flags contract "{name}" with a "{risk_type}" compliance risk')
def step_then_monitor_flags_risk(context, name, risk_type):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/monitor, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    body = context.requests_response.json()
    risks = body.get("risks")
    assert isinstance(risks, list), f"Expected a 'risks' list in the monitor response, got: {body}"
    matching = [r for r in risks if r.get("did") == did and r.get("risk_type") == risk_type]
    assert matching, (
        f"Expected /pac/monitor to flag contract '{name}' (did={did}) with a "
        f"{risk_type} risk, got risks: {risks}"
    )
    assert matching[0].get("detail"), f"Expected the flagged risk to carry a detail message, got: {matching[0]}"


@then('the flagged risk for contract "{name}" is recorded in the PAC audit trail')
def step_then_flagged_risk_audited(context, name):
    # Each flagged risk is anchored per affected contract as a
    # PAC_COMPLIANCE_RISK event (querymonitor.go). The sweep-level
    # PAC_COMPLIANCE_MONITOR event carries no resource DID and only enters
    # the global chain, so the per-contract risk event is the auditable
    # artifact a PAC-scope read can prove. Anchoring is async (outbox ->
    # TSA -> IPFS), hence the poll.
    import time  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    found = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(
            context, pac_audit_url(context), {"scope": "PROCESS_AUDIT_AND_COMPLIANCE"}, headers=headers
        )
        assert resp.status_code == 200, f"PAC-scope audit failed: {resp.status_code} {resp.text}"
        body = resp.json()
        found = [
            (entry.get("event_type"), entry.get("did"))
            for scope_result in body
            for entry in (scope_result.get("audit_trail") or [])
            if isinstance(entry, dict)
        ]
        if ("PAC_COMPLIANCE_RISK", did) in found:
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected a PAC_COMPLIANCE_RISK audit event for contract '{name}' (did={did}) "
        f"in the PROCESS_AUDIT_AND_COMPLIANCE trail, got entries: {found}"
    )


@then("the monitoring response reports a checked_at timestamp and a risks list")
def step_then_monitor_response_shape(context):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/monitor, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("checked_at"), f"Expected a checked_at timestamp in the monitor response, got: {body}"
    assert isinstance(body.get("risks"), list), (
        f"Expected the monitor response to carry a risks list (empty when compliant), got: {body}"
    )


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
