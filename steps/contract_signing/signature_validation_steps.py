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


# The validate endpoint's findings list mixes defect findings with positive
# confirmations (signingmanagement/db/pg/contractrepository.go's
# CollectValidationFindings appends "Document integrity check passed" on
# success; signingmanagement/query/validate.go appends the PID cross-check
# confirmation). A healthy signature therefore reports *only* entries from
# this confirmation set — an empty list would itself be a bug.
_PASSING_VALIDATION_FINDINGS = {
    "Document integrity check passed",
    "Embedded PID presentation re-verified and cross-checked against the signature record",
    "Validation passed",
}


@then('the signature validation for contract "{name}" reports only passing checks')
def step_then_validation_no_findings(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    assert findings, (
        f"Expected the signature validation of freshly-signed contract '{name}' to report "
        f"its passing confirmations (e.g. the document-integrity check), got an empty findings list"
    )
    negative = [f for f in findings if f not in _PASSING_VALIDATION_FINDINGS]
    assert not negative, (
        f"Expected a freshly-signed contract '{name}' to report only passing validation "
        f"confirmations, got defect findings: {negative} (full list: {findings})"
    )
    assert "Document integrity check passed" in findings, (
        f"Expected the MR/HR document-integrity check confirmation for contract '{name}', "
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
    deadline = time.monotonic() + 90
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


def _exported_pdf_bytes(context, name):
    pdf_store = getattr(context, "pdf_bytes", {}) or {}
    assert name in pdf_store, (
        f"No exported PDF bytes recorded for contract '{name}' — was "
        f"'contract \"{name}\" has an exported PDF' run first?"
    )
    return pdf_store[name]


@then('the exported PDF for contract "{name}" declares PDF/A-3 conformance in its XMP metadata')
def step_then_pdf_declares_pdfa(context, name):
    # DCS-FR-SM-27 / ISO 19005-3 clause 6.6.4: PDF/A version and conformance
    # level are declared via the pdfaid XMP extension schema. pdf-core
    # compiles part=3, conformance=A (compiler/compiler_pdf.go).
    pdf = _exported_pdf_bytes(context, name)
    assert b'pdfaid:part="3"' in pdf, (
        f"Expected the exported PDF of '{name}' to declare pdfaid:part=\"3\" "
        f"(PDF/A-3) in its XMP metadata"
    )
    assert b'pdfaid:conformance="A"' in pdf, (
        f"Expected the exported PDF of '{name}' to declare pdfaid:conformance=\"A\" "
        f"in its XMP metadata"
    )


@then('the exported PDF for contract "{name}" embeds the canonical JSON-LD payload as an associated file')
def step_then_pdf_embeds_jsonld(context, name):
    # The machine-readable payload rides inside the PDF/A-3 container as an
    # associated file: Filespec (contract.jsonld) with AFRelationship /Source
    # and an application/ld+json embedded file stream.
    pdf = _exported_pdf_bytes(context, name)
    assert b"(contract.jsonld)" in pdf, (
        f"Expected a (contract.jsonld) Filespec in the exported PDF of '{name}'"
    )
    assert b"/AFRelationship /Source" in pdf, (
        f"Expected the contract.jsonld attachment of '{name}' to be an associated "
        f"file with AFRelationship /Source"
    )
    assert b"application#2Fld+json" in pdf, (
        f"Expected an application/ld+json embedded file stream in the exported PDF of '{name}'"
    )
