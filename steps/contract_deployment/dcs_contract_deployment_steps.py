"""BDD steps for the 'contract-deployment' requirement (Workstream G,
docs/anforderung.md) — Pruefmittel=BDD ACs only (AC1-AC12).

AC13 (grep-gate per the analyst's Pruefmittel column) is deliberately NOT
implemented here — the verifier checks that against a static grep, not a
Gherkin scenario.

--- ASSUMED endpoint / shape contracts (none of this exists in
backend/design/*.go yet at the time this pack was written; grep
`backend/design -rn "contract/deploy"` returns nothing) ---

1. `POST /contract/deploy` (manual deploy trigger, UC-05-01): payload
   `{"did", "updated_at"}`, gated to `SIGNED` contracts, role "Contract
   Manager" (docs/anforderung.md G2: "a Contract Manager submits a signed
   contract for deployment"). ASSUMED response shape: `{"did",
   "contract_version", "content_hash", "timestamp", "correlation_id",
   "payload": {...the machine-readable JSON-LD, including an "@type":
   "odrl:Set" node somewhere...}}` — i.e. the deploy response itself echoes
   the payload actually sent to the target, which is the only test seam this
   pack has for AC4 (no local HTTP-capture server is set up for the outbound
   call — see the note below on why).

2. `POST /contract/deployment/callback` (target -> DCS, IR-SI-05): payload
   `{"did", "correlation_id", "status", ...}` (ack/status update) or
   `{"did", "correlation_id", "kpi": {"metric", "value"}}` (KPI report),
   protected by a shared-secret header — ASSUMED header name/env var,
   mirroring the already-accepted EUDIPLO-webhook precedent
   (steps/real_signing_vertical/dcs_real_signing_vertical_steps.py): header
   `X-Deployment-Callback-Secret`, env `BDD_DEPLOYMENT_CALLBACK_SECRET`
   (default "bdd-deployment-callback-secret"). Values->Secret wiring on the
   backend side is out of scope here (implementer concern).

3. `GET /contract/retrieve/{did}` is ASSUMED to grow a `"kpis"` field once G4
   lands (list of `{"metric", "value", "observed_at", "violation"}`). No
   violation-flag shape is specified anywhere in docs/anforderung.md beyond
   "a violation flag/alert" — this pack checks for a per-KPI `"violation":
   true` marker OR the metric name appearing in a top-level
   `"kpi_violations"` list, whichever the implementer picks.

4. Archive entries (`GET /archive/search?did=...`, already-existing
   ContractStorageArchive service) are ASSUMED to grow an `"evidence"` JSON
   object carrying a nested `"deployment"` object once G1/G4 land:
   `{"deployment": {"correlation_id", "payload_hash", "receipt_hash",
   "tsa_token", "activated_at"}}` — mirroring the existing `evidence` field
   already produced by `command.BuildArchiveEntry`
   (backend/internal/contractworkflowengine/command/archive.go), which is a
   free-form JSON blob today (`source`, `approved_by`, ... keys) that G1
   ("add an append-evidence path to the archive record") is expected to
   extend rather than replace.

Why no local HTTP-capture server for the outbound deploy-to-target call
(AC4): this BDD suite runs either against a locally-run `air` backend
(dev-stack.sh, same WSL host as this test process — reachable) or against a
Helm/kind-deployed backend pod (run_bdd_helm.sh, a different network
namespace — NOT reachable from a plain `http.server` bound to this test
process's localhost). Since this pack must run in both environments, it
deliberately does not invent a capture server and instead relies on the
deploy endpoint's own response (assumption 1 above) as the test seam for
AC4. AC8 is the genuine end-to-end counterpart: it talks to the actual
shipped ORCE flow directly (a real, independently-reachable service), which
does not have this networking problem — see `BDD_ORCE_TARGET_URL` below.

--- AC2's DB seam ---

AC2 ("an archived + ACTIVE contract still appears in the live list") is
deliberately tested WITHOUT going through the not-yet-existing deploy/ORCE/
callback chain: forcing `state='ACTIVE'` directly via the shared test DB
connection (context.db, see environment.py) isolates the actual behavior
AC2 claims (a query/dashboard filtering bug: archived must not be treated as
inactive) from the deploy mechanism itself, which is separately and more
directly exercised by AC3/AC6/AC7/AC8/AC9/AC10. This mirrors the
already-accepted precedent of direct-DB seams for preconditions the API has
no fast/existing path to establish (steps/peer_trust's `_seed_trusted_peer`,
steps/template_management/contract_state_machine_steps's exp_date backdate).

--- AC8's ORCE reachability ---

`BDD_ORCE_TARGET_URL` (no default) must point at the deployed
contract-target-flow's HTTP-in endpoint (deployment/helm/charts/orce/flows/
contract-target-flow.json). If unset, the AC8 scenario fails fast with an
explicit message naming the missing wiring, the same "open point, not a
defect in this scenario" pattern already used for the two-instance runner
(steps/peer_trust/dcs_peer_trust_steps.py) — this is a single, real,
independently-reachable ORCE service, not a second DCS instance, so it is
NOT tagged @two-instance.
"""

import base64
import hashlib
import json
import os

import requests as _requests
from behave import given, step, then, when

from steps.support.api_client import (
    archive_search_url,
    contract_deploy_url,
    contract_deployment_callback_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
    _apply_signature as _apply_signature_via_ceremony,
)

DEPLOYMENT_CALLBACK_SECRET_HEADER = "X-Deployment-Callback-Secret"


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _callback_secret() -> str:
    return os.getenv("BDD_DEPLOYMENT_CALLBACK_SECRET", "bdd-deployment-callback-secret")


def _archive_entry_for(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    resp = get_with_headers(context, f"{archive_search_url(context)}?did={did}", headers=headers)
    assert resp.status_code == 200, (
        f"Archive search failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    entries = resp.json()
    assert isinstance(entries, list), f"Expected archive search to return a list, got: {entries}"
    for entry in entries:
        if entry.get("did") == did:
            return entry
    return None


def _find_odrl_set(node) -> bool:
    """Recursively search a JSON-serializable structure for an odrl:Set node
    (AC4's "includes the odrl:Set" claim) — the exact key path the deploy
    response nests the machine-readable payload under is ASSUMED (see module
    docstring), so this walks the whole structure instead of relying on one
    specific key path."""
    if isinstance(node, dict):
        if node.get("@type") == "odrl:Set":
            return True
        return any(_find_odrl_set(v) for v in node.values())
    if isinstance(node, list):
        return any(_find_odrl_set(item) for item in node)
    return False


def _kpi_entries(retrieve_json: dict) -> list:
    kpis = retrieve_json.get("kpis")
    return kpis if isinstance(kpis, list) else []


def _kpi_violation_names(retrieve_json: dict) -> list:
    violations = retrieve_json.get("kpi_violations")
    return violations if isinstance(violations, list) else []


# ---------------------------------------------------------------------------
# Given — AC2's DB seam
# ---------------------------------------------------------------------------


@given(
    'contract "{name}" is force-set to state "{state}" directly in the '
    "database (pre-deploy test seam, bypassing the deployment chain)"
)
def step_given_force_state(context, name, state):
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("UPDATE contracts SET state = %s WHERE did = %s", (state.strip().upper(), did))
    context.db.commit()
    cursor.close()
    ContractService._refresh_contract(context, name)


# ---------------------------------------------------------------------------
# Given — AC12: full submit/review/approve/sign chain as a precondition
# ---------------------------------------------------------------------------


@given('contract "{name}" is submitted, reviewed, approved, and signed via the standard workflow')
def step_given_full_workflow_to_signed(context, name):
    # Fix (gherkin-autor, contract-deployment loop): no Given existed with
    # this exact wording (odrl-soundness's dcs_odrl_steps.py only has a
    # @when-registered step with a slightly different wording — "the
    # contract ..." vs "contract ..." here — so it does not match this
    # feature's text either way). Reuses the already-correct,
    # ceremony-aware helpers from contract_state_machine_steps.py /
    # real-signing-vertical rather than re-implementing the submit -> review
    # -> approve -> sign chain a third time.
    _advance_to_approved(context, name)
    _apply_signature_via_ceremony(context, name)


# ---------------------------------------------------------------------------
# Given/When — AC3/AC4/AC5: manual deploy trigger
# ---------------------------------------------------------------------------


@step('an authorized user deploys contract "{name}" to the configured contract target')
def step_when_deploy_contract(context, name):
    # Fix (gherkin-autor, contract-deployment loop): AC7/AC9/AC10/AC11/AC12
    # all use this step as an "And" continuing a *Given* block (asserting an
    # intermediate setup call succeeded before the scenario's actual
    # precondition is fully built) — behave's Given/When/Then decorators
    # register into separate per-type lookup tables, so a step registered
    # only via @when is "undefined" when behave looks it up as a Given.
    # `@step` (still imported from plain `behave`) registers this text under
    # given/when/then alike, which is the correct fix here since this step
    # is also still genuinely used as a real When (AC3/AC4/AC5).
    did, updated_at = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, contract_deploy_url(context), {"did": did, "updated_at": updated_at}, headers=manager_h
    )
    if context.requests_response.status_code == 200:
        body = context.requests_response.json()
        context.deployment_correlation_id = body.get("correlation_id")
        context.deployment_content_hash = body.get("content_hash")
        ContractService._refresh_contract(context, name)


@then("the deployment response includes a correlation ID")
def step_then_deployment_response_has_correlation_id(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the deploy request to succeed, got {resp.status_code}: {resp.text}"
    )
    body = resp.json()
    correlation_id = body.get("correlation_id")
    assert correlation_id, f"Expected a non-empty 'correlation_id' in the deploy response, got: {body}"


@then(
    'the deployment response declares the contract DID, version, content '
    'hash, timestamp, and the odrl:Set policy for "{name}"'
)
def step_then_deployment_payload_declared(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the deploy request to succeed, got {resp.status_code}: {resp.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    body = resp.json()
    assert body.get("did") == did, f"Expected deploy response 'did' to equal {did!r}, got: {body.get('did')!r}"
    assert body.get("contract_version") is not None, f"Expected 'contract_version' in deploy response: {body}"
    content_hash = body.get("content_hash")
    assert content_hash, f"Expected a non-empty 'content_hash' in the deploy response: {body}"
    assert body.get("timestamp"), f"Expected a non-empty 'timestamp' in the deploy response: {body}"
    assert _find_odrl_set(body), (
        "Expected the deployment payload declared in the deploy response to include an "
        f"odrl:Set node (FR-SM-12 / F1's odrl:Set-enclosed policy shape) somewhere: {body}"
    )


# ---------------------------------------------------------------------------
# Given — AC6: automatic, event-driven deployment
# ---------------------------------------------------------------------------


@then('the archive entry for contract "{name}" records an automatic deployment correlation ID')
def step_then_archive_records_auto_deployment(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is not None, (
        f"Expected an archive entry for contract '{name}' after the signing workflow completed "
        "(AC1: archive entry is created on SIGNED) — none was found"
    )
    evidence = entry.get("evidence") or {}
    deployment = evidence.get("deployment") or {}
    correlation_id = deployment.get("correlation_id")
    assert correlation_id, (
        f"Expected the archive entry's evidence.deployment.correlation_id to be populated "
        f"automatically (event-driven, NATS-Outbox-Subscriber per AC6) without any explicit "
        f"POST /contract/deploy call in this scenario, got evidence: {evidence!r}"
    )


# ---------------------------------------------------------------------------
# When/Then — AC7: callback shared-secret auth
# ---------------------------------------------------------------------------


@when('the target sends a deployment callback for contract "{name}" with an invalid shared secret')
def step_when_callback_invalid_secret(context, name):
    did, _ = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
        "status": "ACKNOWLEDGED",
    }
    headers = {
        "Content-Type": "application/json",
        DEPLOYMENT_CALLBACK_SECRET_HEADER: "definitely-not-" + _callback_secret(),
    }
    context.requests_response = post_json(context, contract_deployment_callback_url(context), payload, headers=headers)


@then("the callback request is rejected for the missing or invalid shared secret")
def step_then_callback_rejected(context):
    resp = context.requests_response
    assert resp.status_code in (401, 403), (
        "Expected POST /contract/deployment/callback to reject a request with an invalid "
        f"{DEPLOYMENT_CALLBACK_SECRET_HEADER!r} header, got {resp.status_code}: {resp.text}"
    )


# ---------------------------------------------------------------------------
# When — AC9/AC10/AC11/AC12: valid-secret ack + KPI callbacks
# ---------------------------------------------------------------------------


@step('the target sends a deployment acknowledgement for contract "{name}" with the correct shared secret')
def step_when_callback_valid_ack(context, name):
    # Fix (gherkin-autor, contract-deployment loop): AC11/AC12 use this step
    # as an "And" continuing a Given block for the same reason documented on
    # `step_when_deploy_contract` above — `@step` registers it as
    # given/when/then alike; AC9/AC10's genuine `When` usage is unaffected.
    did, _ = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
        "status": "ACKNOWLEDGED",
        "receipt": {
            "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
            "payload_hash": getattr(context, "deployment_content_hash", None) or "sha256:bdd-unknown-hash",
            "activated_at": "2026-01-01T00:00:00Z",
        },
    }
    headers = {"Content-Type": "application/json", DEPLOYMENT_CALLBACK_SECRET_HEADER: _callback_secret()}
    context.requests_response = post_json(context, contract_deployment_callback_url(context), payload, headers=headers)
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@then('the archive entry for contract "{name}" contains an RFC-3161 TSA timestamp over the execution-evidence receipt')
def step_then_archive_has_tsa_timestamp(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is not None, f"Expected an archive entry for contract '{name}'"
    evidence = entry.get("evidence") or {}
    deployment = evidence.get("deployment") or {}
    receipt_hash = deployment.get("receipt_hash")
    tsa_token_b64 = deployment.get("tsa_token")
    assert receipt_hash, (
        f"Expected the archive entry's evidence.deployment.receipt_hash (canonical hash of the "
        f"execution-evidence receipt) to be populated, got: {deployment!r}"
    )
    assert tsa_token_b64, (
        f"Expected the archive entry's evidence.deployment.tsa_token (RFC-3161 TSA response over "
        f"the receipt's canonical hash) to be populated, got: {deployment!r}"
    )
    try:
        tsa_bytes = base64.b64decode(tsa_token_b64, validate=True)
    except Exception as exc:  # noqa: BLE001
        raise AssertionError(f"evidence.deployment.tsa_token is not valid base64: {exc}") from exc
    first_byte_desc = f"{tsa_bytes[0]:#x}" if tsa_bytes else "empty bytes"
    assert tsa_bytes and tsa_bytes[0] == 0x30, (
        "Expected evidence.deployment.tsa_token to decode to a DER-encoded ASN.1 SEQUENCE "
        f"(RFC 3161 TimeStampResp/Token starts with tag 0x30), got first byte: {first_byte_desc}"
    )


@when('the target reports a KPI value "{metric}" = "{value}" for contract "{name}"')
def step_when_target_reports_kpi(context, metric, value, name):
    did, _ = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
        "kpi": {"metric": metric, "value": value},
    }
    headers = {"Content-Type": "application/json", DEPLOYMENT_CALLBACK_SECRET_HEADER: _callback_secret()}
    context.requests_response = post_json(context, contract_deployment_callback_url(context), payload, headers=headers)


@then('the contract detail for "{name}" shows KPI "{metric}" with value "{value}"')
def step_then_contract_detail_shows_kpi(context, name, metric, value):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    kpis = _kpi_entries(retrieve.json())
    matching = [k for k in kpis if str(k.get("metric")) == metric]
    assert matching, (
        f"Expected contract '{name}' detail to include a KPI entry for metric '{metric}' "
        f"(FR-CWE-31/FR-CWE-09 dashboard), got kpis: {kpis!r}"
    )
    actual_value = str(matching[-1].get("value"))
    assert actual_value == value, (
        f"Expected KPI '{metric}' on contract '{name}' to have value '{value}', got '{actual_value}'"
    )


@then('the contract detail for "{name}" shows a KPI violation flag for "{metric}"')
def step_then_contract_detail_shows_kpi_violation(context, name, metric):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    body = retrieve.json()
    kpis = _kpi_entries(body)
    matching = [k for k in kpis if str(k.get("metric")) == metric]
    flagged_on_entry = any(bool(k.get("violation")) for k in matching)
    flagged_top_level = metric in _kpi_violation_names(body)
    assert flagged_on_entry or flagged_top_level, (
        f"Expected a KPI violation flag/alert for metric '{metric}' on contract '{name}' after a "
        f"reported KPI value crossed its contractually declared SLA threshold (odrl:Set "
        f"rightOperand), got kpis: {kpis!r}, kpi_violations: {body.get('kpi_violations')!r}"
    )


# ---------------------------------------------------------------------------
# Given/When/Then — AC1: archive-entry trigger moved from APPROVED to SIGNED
# ---------------------------------------------------------------------------


@then('the archive has no entry for contract "{name}"')
def step_then_archive_has_no_entry(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is None, (
        f"Expected NO archive entry for contract '{name}' yet (archive-entry creation is gated to "
        f"the SIGNED transition, not APPROVED, per AC1), but found one: {entry!r}"
    )


@then('the archive has an entry for contract "{name}"')
def step_then_archive_has_entry(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is not None, (
        f"Expected an archive entry for contract '{name}' after it reached SIGNED (AC1), found none"
    )


# ---------------------------------------------------------------------------
# Given/When/Then — AC8: the shipped ORCE contract-target-flow directly
# ---------------------------------------------------------------------------


@given("the example ORCE contract-target-flow is reachable")
def step_given_orce_reachable(context):
    orce_url = os.getenv("BDD_ORCE_TARGET_URL", "").strip()
    assert orce_url, (
        "BDD_ORCE_TARGET_URL must be set to the deployed contract-target-flow's HTTP-in endpoint "
        "(deployment/helm/charts/orce/flows/contract-target-flow.json) to run this scenario. This "
        "flow does not exist yet (Workstream G3 deliverable) — this is an open point for G3, not "
        "a defect in this scenario."
    )
    context.orce_target_url = orce_url


@when('a deployment payload for contract "{name}" is posted directly to the ORCE contract-target-flow')
def step_when_post_to_orce_directly(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    correlation_id = f"bdd-orce-{did}-{updated_at}".replace(":", "-").replace(" ", "-")
    inner_payload = {
        "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/"},
        "@type": "dcs:Contract",
        "dcs:contractDid": did,
        "odrl:policy": {"@id": "urn:uuid:bdd-orce-policy-set", "@type": "odrl:Set", "uid": did},
    }
    canonical = json.dumps(inner_payload, sort_keys=True, separators=(",", ":")).encode()
    content_hash = "sha256:" + hashlib.sha256(canonical).hexdigest()
    context.orce_sent_correlation_id = correlation_id
    context.orce_sent_content_hash = content_hash

    body = {
        "contract_did": did,
        "contract_version": 1,
        "correlation_id": correlation_id,
        "content_hash": content_hash,
        "timestamp": "2026-01-01T00:00:00Z",
        "payload": inner_payload,
    }
    context.requests_response = _requests.post(
        context.orce_target_url, json=body, timeout=context.http_timeout_seconds
    )


@then("the ORCE flow acknowledges with correlation_id, payload_hash, and activated_at matching the sent payload")
def step_then_orce_acknowledges(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the ORCE contract-target-flow to acknowledge the deployment payload, got "
        f"{resp.status_code}: {resp.text}"
    )
    ack = resp.json()
    assert ack.get("correlation_id") == context.orce_sent_correlation_id, (
        f"Expected the ORCE ack's correlation_id to echo {context.orce_sent_correlation_id!r}, got "
        f"{ack.get('correlation_id')!r}: {ack!r}"
    )
    assert ack.get("payload_hash") == context.orce_sent_content_hash, (
        f"Expected the ORCE ack's payload_hash to equal the content_hash the flow received and "
        f"verified ({context.orce_sent_content_hash!r}), got {ack.get('payload_hash')!r}: {ack!r}"
    )
    assert ack.get("activated_at"), f"Expected a non-empty 'activated_at' in the ORCE ack: {ack!r}"
