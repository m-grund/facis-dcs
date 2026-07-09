"""BDD step definitions for the c2pa-conformance requirement (Workstream D,
docs/anforderung.md Zeilen 270-282).

Covers:
  - AC1/AC2: the public, unauthenticated GET /c2pa/manifest/{did} endpoint
    (raw manifest store bytes by default; ?history=true for a parsed chain
    enumeration).
  - AC3: the embedded manifest's `remote_manifests` claim field referencing
    AC1's own endpoint.
  - AC6: the verify response's four independently named checks, with the
    PDF-signature check honestly reporting "not yet available" rather than
    faking a pass (Workstream B/PAdES does not exist yet).

AC5 (lifecycle banner per state) and the "has reached contract state"/
"is exported and verified as PDF" setup steps are deliberately NOT
redefined here — they already exist in
steps/template_management/contract_state_machine_steps.py and are reused
as-is (see that module's `_reach_state` helper and its
`the C2PA lifecycle_status for contract "{name}" is "{status}"` step).

GET /c2pa/manifest/{contract_did} does not exist in backend/design/ yet
(searched backend/design/*.go — no match) — every request this module
issues against it is expected to 404/fail until Workstream D1 lands. That
is the intended red signal for AC1/AC2/AC3.
"""

import requests as _requests

from behave import then, when

from steps.support.api_client import c2pa_manifest_url
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# When — public manifest requests (deliberately sent WITHOUT any
# Authorization header, proving AC1's "no JWT/auth" requirement honestly —
# not just reusing context.headers with an empty override).
# ---------------------------------------------------------------------------


@when('I request the public C2PA manifest for contract "{name}"')
def step_when_request_public_manifest(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.get(
        c2pa_manifest_url(context, did),
        timeout=context.http_timeout_seconds,
    )


@when('I request the C2PA manifest history for contract "{name}"')
def step_when_request_manifest_history(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.get(
        c2pa_manifest_url(context, did),
        params={"history": "true"},
        timeout=context.http_timeout_seconds,
    )


# ---------------------------------------------------------------------------
# Then — AC1: raw manifest store response shape
# ---------------------------------------------------------------------------


@then('the response has Content-Type "{content_type}"')
def step_then_response_content_type(context, content_type):
    actual = context.requests_response.headers.get("Content-Type", "")
    actual_media_type = actual.split(";")[0].strip()
    assert actual_media_type == content_type, (
        f"Expected Content-Type '{content_type}', got '{actual}'"
    )


@then("the response body is a non-empty C2PA JUMBF manifest store")
def step_then_response_is_jumbf_manifest_store(context):
    body = context.requests_response.content
    assert body, (
        f"Response body is empty (status {context.requests_response.status_code})"
    )
    # 'jumb' is the JUMBF superbox type marker pdf-core's own BDD suite uses
    # to recognize a manifest store (pdf-core/features/steps/
    # dcs_pdf_core_steps.py: "the manifest store response contains the
    # JUMBF marker").
    assert b"jumb" in body, (
        "Response body does not contain the JUMBF superbox marker 'jumb' — "
        f"not a valid C2PA manifest store: {body[:200]!r}"
    )


# ---------------------------------------------------------------------------
# Then — AC2: parsed history enumeration response shape
# ---------------------------------------------------------------------------


@then("the response is a JSON list of manifest labels with dcs.lifecycle assertions")
def step_then_history_response_shape(context):
    body = context.requests_response.json()
    assert isinstance(body, list) and body, (
        f"Expected a non-empty JSON list of manifest chain entries, got: {body}"
    )
    for entry in body:
        assert isinstance(entry, dict) and entry.get("label"), (
            f"Each manifest history entry must carry a non-empty 'label' field: {entry}"
        )
    lifecycle_entries = [e for e in body if e.get("lifecycle") or e.get("dcs.lifecycle")]
    assert lifecycle_entries, (
        "Expected at least one manifest history entry to carry a dcs.lifecycle "
        f"assertion (key 'lifecycle' or 'dcs.lifecycle'), got: {body}"
    )


# ---------------------------------------------------------------------------
# Then — AC3: remote_manifests claim field
# ---------------------------------------------------------------------------


@then(
    'the C2PA manifest response for contract "{name}" declares a remote_manifests field '
    "pointing to its own public manifest endpoint"
)
def step_then_manifest_declares_remote_manifests(context, name):
    did, _ = ContractService._contract_data(context, name)
    manifest_bytes = context.requests_response.content
    assert context.requests_response.status_code == 200, (
        f"C2PA manifest request failed for contract '{name}': "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )
    # "remote_manifests" is the C2PA Claim field name for a remote-manifest
    # URL reference — already established by pdf-core's own BDD suite
    # (pdf-core/features/manifest_url.feature, pdf-core/features/steps/
    # dcs_pdf_core_steps.py:1816-1825), which also documents that c2pa-rs
    # 0.85.1 currently REJECTS this field in V2 claims. Per this task's
    # explicit user decision, AC3 is checked against the literal claim-field
    # approach anyway (see the feature file's header comment).
    assert b"remote_manifests" in manifest_bytes, (
        "C2PA manifest store does not declare a 'remote_manifests' field at all: "
        f"{manifest_bytes[:300]!r}"
    )
    expected_path = f"/c2pa/manifest/{did}".encode()
    assert expected_path in manifest_bytes, (
        "remote_manifests field does not reference this contract's own public "
        f"manifest endpoint path '{expected_path.decode()}' — manifest store: "
        f"{manifest_bytes[:300]!r}"
    )


# ---------------------------------------------------------------------------
# Then — AC6: four independently named verify checks
# ---------------------------------------------------------------------------


@then(
    'the verify response for contract "{name}" includes four named checks: '
    "PDF signature, C2PA manifest, VC signature, and status list"
)
def step_then_verify_four_named_checks(context, name):
    assert context.requests_response.status_code == 200, (
        f"Verify failed for contract '{name}': {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    # c2pa_manifest_found / vc_proof_valid / status_list_status already exist
    # on PDFVerifyResult (backend/design/pdf_generation.go:16-22).
    # pdf_signature_status does NOT exist yet — Workstream B/PAdES has not
    # landed, so there is currently no dedicated "PDF signature" check field
    # distinct from the C2PA COSE signature check. Its absence here is the
    # intended red signal for AC6's fourth named check.
    required_fields = {
        "pdf_signature_status": "PDF signature",
        "c2pa_manifest_found": "C2PA manifest",
        "vc_proof_valid": "VC signature",
        "status_list_status": "status list",
    }
    missing = [
        f"{field} ({label})"
        for field, label in required_fields.items()
        if field not in body
    ]
    assert not missing, (
        f"Verify response for contract '{name}' is missing named check field(s): "
        f"{', '.join(missing)} — response: {body}"
    )


@then(
    'the PDF signature check for contract "{name}" is marked as not yet available '
    "rather than passed"
)
def step_then_pdf_signature_not_available(context, name):
    body = context.requests_response.json()
    status = body.get("pdf_signature_status")
    assert status in ("pending", "not_available"), (
        f"Expected pdf_signature_status to be 'pending' or 'not_available' for "
        f"contract '{name}' (Workstream B/PAdES is not implemented yet — "
        "DCS-OR-C2PA-006 forbids the verifier from falsely showing a passed PDF "
        f"signature check), got: {status!r} in response {body}"
    )
