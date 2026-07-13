"""BDD steps for the signature validate/audit/compliance endpoints
(DCS-FR-SM-18, DCS-FR-SM-19, DCS-FR-SM-21, UC-04): POST /signature/validate,
GET /signature/audit, POST /signature/compliance
(backend/design/signature_management.go) - all three are already implemented
(backend/internal/service/signature_management.go); this module drives them
against a contract that has actually gone through the real signing ceremony
(contract_state_machine_steps.py's "has reached contract state \"SIGNED\""
Given, reused rather than re-invented here).
"""

import time

import requests
from behave import then, when

from steps.support.api_client import (
    post_json,
    signature_audit_url,
    signature_compliance_url,
    signature_validate_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when('the contract manager validates the signature for contract "{name}"')
def step_when_validate_signature(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)


@when('the contract manager requests a compliance check for contract "{name}"')
def step_when_compliance_check(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, signature_compliance_url(context), {"did": did}, headers=manager_h
    )


@then('the signature validation for contract "{name}" reports no findings')
def step_then_validation_no_findings(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    assert findings == [], (
        f"Expected a freshly-signed contract '{name}' to report no signature validation findings, "
        f"got: {findings}"
    )


@then('the compliance check for contract "{name}" returns no findings')
def step_then_compliance_no_findings(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    # The compliance endpoint's own design description
    # (backend/design/signature_management.go's "compliance" method) is
    # explicit that it records the compliance-check request and emits a
    # ComplianceValidationEvent, but does not itself compute findings: "the
    # response's findings list is currently always empty". Asserting emptiness
    # here is therefore the accurate claim, not an under-test of the endpoint.
    assert findings == [], f"Expected an empty findings list from /signature/compliance, got: {findings}"


@then('the signature audit log for contract "{name}" includes an action of type "{event_type}"')
def step_then_signature_audit_includes(context, name, event_type):
    # The audit trail is anchored asynchronously by the outbox processor
    # (~1s poll interval) — same polling convention as
    # contract_state_machine_steps.py's audit-event step.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        resp = requests.get(
            signature_audit_url(context),
            params={"did": did},
            headers=auditor_h,
            timeout=context.http_timeout_seconds,
        )
        assert resp.status_code == 200, f"Signature audit query failed for '{name}': {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of signature audit entries, got: {entries}"
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if event_type.upper() in event_types:
            return
        time.sleep(1)
    assert event_type.upper() in event_types, (
        f"Expected a '{event_type}' signature audit event for contract '{name}', got event types: {event_types}"
    )
