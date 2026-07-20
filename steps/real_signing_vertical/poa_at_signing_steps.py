"""Steps for the Power-of-Attorney signing gate and compliance
(features/22_real_signing_vertical/poa_at_signing.feature; UC-14, DCS-FR-SM-03/
-04/-26).

The signatory presents a fresh PoA credential at the ceremony. The webhook
verifies it authorizes the very party (the participating instance DID) being
signed; a missing or wrong-organization PoA blocks signing (UC-14). The
compliance viewer re-checks every party node in the (possibly peer-synced)
contract and raises a finding for any that signed without a valid PoA, which is
recorded as an audit event (FR-SM-04/-26).
"""

from __future__ import annotations

import json
import uuid

from behave import given, then, when

from steps.support.api_client import (
    post_json,
    signature_compliance_url,
    signature_request_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (
    _build_pid_presentation,
    _complete_ceremony_via_webhook,
)


def _start_party_ceremony(context, name):
    """Start a ceremony for the signing party (the local instance DID) and build
    the PID presentation the webhook needs, without completing it."""
    party_did = ContractService._local_peer_did(context)
    did, _ = ContractService._contract_data(context, name)
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    resp = post_json(context, signature_request_url(context), {"contract_did": did, "field_name": party_did}, headers=signer_h)
    assert resp.status_code == 200, f"/signature/request failed: {resp.status_code} {resp.text}"
    ceremony_id = resp.json()["ceremony_id"]
    presentation, _issuer, _disc, subject_did = _build_pid_presentation(
        given_name="PoA Signatory", family_name="BDD-Testperson", aud="dcs-signature-ceremony", nonce=str(uuid.uuid4()),
    )
    context.poa_ceremony = {"id": ceremony_id, "presentation": presentation, "subject_did": subject_did, "party_did": party_did}


@when('a signing ceremony is started for the signing party of contract "{name}"')
def step_when_start_party_ceremony(context, name):
    _start_party_ceremony(context, name)


@when('the ceremony webhook is completed with no Power of Attorney')
def step_when_webhook_no_poa(context):
    c = context.poa_ceremony
    context.requests_response = _complete_ceremony_via_webhook(
        context, c["id"], c["presentation"], c["subject_did"], "PoA Signatory", "BDD-Testperson",
        poa_organization="",
    )


@when('the ceremony webhook is completed with a Power of Attorney for a different party')
def step_when_webhook_wrong_poa(context):
    c = context.poa_ceremony
    context.requests_response = _complete_ceremony_via_webhook(
        context, c["id"], c["presentation"], c["subject_did"], "PoA Signatory", "BDD-Testperson",
        poa_organization="did:web:some-other-org.example",
    )


@then('the signing request is rejected because the Power of Attorney does not authorize this signature')
def step_then_poa_rejected(context):
    resp = context.requests_response
    assert resp.status_code == 400, f"expected 400 (PoA gate), got {resp.status_code}: {resp.text}"
    assert "power of attorney" in resp.text.lower(), f"expected a power-of-attorney rejection, got: {resp.text}"


def _compliance_findings(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_compliance_url(context), {"did": did}, headers=headers)
    assert resp.status_code == 200, f"/signature/compliance failed: {resp.status_code} {resp.text}"
    return resp.json().get("findings") or []


@then('the signature compliance for contract "{name}" raises no Power of Attorney finding')
def step_then_no_poa_finding(context, name):
    findings = _compliance_findings(context, name)
    offending = [f for f in findings if "power of attorney" in f.lower()]
    assert not offending, f"expected no Power of Attorney finding, got: {offending}"


@then('the signature compliance for contract "{name}" raises a Power of Attorney finding')
def step_then_poa_finding(context, name):
    findings = _compliance_findings(context, name)
    offending = [f for f in findings if "power of attorney" in f.lower()]
    assert offending, f"expected a Power of Attorney finding, got findings: {findings}"
    context.poa_finding = offending[0]


@when('the counterparty Power of Attorney on contract "{name}" is tampered to authorize a different organization')
def step_when_tamper_counterparty_poa(context, name):
    """Simulate a misconfigured/malicious counterparty DCS: inject the party node
    such a peer would have sealed and synced — a signed party (dcs:hasSignatory)
    whose dcs:hasPowerOfAttorney authorizes a different organization than the
    party itself. Compliance must raise a finding for it (FR-SM-04)."""
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("SELECT contract_data FROM contracts WHERE did = %s", (did,))
    row = cursor.fetchone()
    assert row, f"contract {did} not found in the test DB"
    doc = row[0] if isinstance(row[0], dict) else json.loads(row[0])
    parties = doc.get("dcs:parties")
    if not isinstance(parties, list):
        parties = []
        doc["dcs:parties"] = parties
    parties.append({
        "@id": "did:web:counterparty-org.example",
        "@type": "dcs:CompanyParty",
        "dcs:hasSignatory": {"@id": "did:jwk:counterparty-signer"},
        "dcs:hasPowerOfAttorney": {"@id": "did:web:impostor-org.example"},
    })
    cursor.execute("UPDATE contracts SET contract_data = %s WHERE did = %s", (json.dumps(doc), did))
    context.db.commit()


@then('an audit event records the Power of Attorney finding for contract "{name}"')
def step_then_audit_records_poa(context, name):
    import time  # noqa: PLC0415

    import requests  # noqa: PLC0415

    from steps.support.api_client import signature_audit_url  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        resp = requests.get(signature_audit_url(context), params={"did": did}, headers=auditor_h, timeout=context.http_timeout_seconds)
        assert resp.status_code == 200, f"signature audit read failed: {resp.status_code} {resp.text}"
        entries = resp.json()
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if "COMPLIANCE_VALIDATION" in event_types:
            return
        time.sleep(1)
    assert "COMPLIANCE_VALIDATION" in event_types, (
        f"expected a COMPLIANCE_VALIDATION audit event recording the Power of Attorney finding, got: {event_types}"
    )
