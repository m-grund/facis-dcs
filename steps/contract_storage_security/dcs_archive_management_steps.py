"""BDD steps for the Contract Storage & Archive endpoints (UC-07,
backend/design/contract_storage_archive.go): /archive/retrieve,
/archive/search, /archive/audit, /archive/delete. Archive-entry
creation/evidence content is covered by
05_contract_deployment/contract_deployment.feature — this module only
exercises the archive endpoints themselves.
"""

import time

from behave import given, then, when

from steps.support.api_client import (
    pac_audit_url,
    archive_annotate_url,
    archive_audit_url,
    archive_delete_url,
    archive_retrieve_url,
    archive_search_url,
    delete_with_params,
    get_with_headers,
    post_json,
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
    context.requests_response = get_with_headers(context, archive_audit_url(context) + "?justification=BDD%20archive%20audit%20review", headers=headers)


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
    # The audit trail is anchored asynchronously by the outbox processor
    # (~1s poll interval, see conf.OutboxProcessorTimeOut) — a contract that
    # reached SIGNED (and so wrote its archive-store event) moments before
    # this call may not be anchored yet. Same re-trigger-and-poll convention
    # as steps/audit_compliance/dcs_process_audit_steps.py's
    # step_then_audit_response_includes_contract.
    headers = AuthService.get_headers_for_roles(["Auditor"])
    body = []
    deadline = time.monotonic() + 90
    while True:
        body = context.requests_response.json()
        if isinstance(body, list) and len(body) > 0:
            return
        if time.monotonic() > deadline:
            break
        time.sleep(2)
        context.requests_response = get_with_headers(context, archive_audit_url(context) + "?justification=BDD%20archive%20audit%20review", headers=headers)
        assert context.requests_response.status_code == 200, (
            f"archive audit re-trigger failed: {context.requests_response.status_code} "
            f"{context.requests_response.text}"
        )
    assert isinstance(body, list) and len(body) > 0, (
        f"Expected /archive/audit to return a non-empty list of audit log entries, got: {body}"
    )


@when('the Archive Manager deletes the archived contract "{name}" with justification "{justification}"')
def step_when_archive_manager_deletes(context, name, justification):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = delete_with_params(
        context, archive_delete_url(context), {"did": did, "justification": justification}, headers=headers
    )


@when('I attempt to delete the archived contract "{name}" with my current role')
def step_when_attempt_delete_archive(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = getattr(context, "headers", {})
    context.requests_response = delete_with_params(
        context, archive_delete_url(context), {"did": did, "justification": "BDD unauthorized deletion attempt"}, headers=headers
    )


@then('the archive deletion of contract "{name}" is recorded in the archive audit log')
def step_then_archive_deletion_audited(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    event_types_for_did = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        # Workflow events (delete/annotate) live in the PAC audit trail under
        # the CONTRACT_STORAGE_ARCHIVE component; /archive/audit serves the
        # archive-integrity view (entries + notary-chain checks).
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "CONTRACT_STORAGE_ARCHIVE", "justification": "BDD archive audit review"},
            headers=headers,
        )
        assert resp.status_code == 200, f"Archive audit query failed: {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of audit scopes, got: {entries}"
        event_types_for_did = [
            str(entry.get("event_type", "")).upper()
            for scope_result in entries
            if isinstance(scope_result, dict)
            for entry in (scope_result.get("audit_trail") or [])
            if isinstance(entry, dict) and entry.get("did") == did
        ]
        if "DELETE_ARCHIVED_CONTRACT" in event_types_for_did:
            return
        time.sleep(2)
    assert "DELETE_ARCHIVED_CONTRACT" in event_types_for_did, (
        f"Expected a DELETE_ARCHIVED_CONTRACT audit event for contract '{name}' ({did}), "
        f"got event types for this DID: {event_types_for_did}"
    )


@when('the Archive Manager searches the archive with full-text query "{query}"')
def step_when_archive_fulltext_search(context, query):
    """DCS-FR-CSA-13: /archive/search?contract_data=... queries the stored
    tsvector over the whole contract JSON-LD (search_vector), not the
    name/description metadata columns."""
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"contract_data": query},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('the Archive Manager searches the archive by tag "{tag}"')
def step_when_archive_tag_search(context, tag):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"tag": tag},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@then('the archive search result does not include contract "{name}"')
def step_then_archive_search_excludes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did not in dids, (
        f"Expected archive search to NOT include contract '{name}' (did={did}), got dids: {dids}"
    )


def _annotate_archived_contract(context, name, payload_extra, headers):
    did, _ = ContractService._contract_data(context, name)
    payload = {"did": did, **payload_extra}
    context.requests_response = post_json(context, archive_annotate_url(context), payload, headers=headers)


@when('the Archive Manager annotates the archived contract "{name}" with summary "{summary}" and tags "{tags}"')
def step_when_archive_manager_annotates(context, name, summary, tags):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    _annotate_archived_contract(
        context, name, {"summary": summary, "tags": tags.split(",")}, headers
    )


@given('the Archive Manager annotates the archived contract "{name}" with summary "{summary}" and tags "{tags}"')
def step_given_archive_manager_annotates(context, name, summary, tags):
    # Given-position variant (setup for the tag-search scenario): a failure
    # here is a broken precondition, so assert success immediately.
    step_when_archive_manager_annotates(context, name, summary, tags)
    assert context.requests_response.status_code == 200, (
        f"Annotating archived contract '{name}' as a scenario precondition failed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )


@when('the Archive Manager annotates the archived contract "{name}" with tags "{tags}" and no summary')
def step_when_archive_manager_annotates_no_summary(context, name, tags):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    _annotate_archived_contract(context, name, {"tags": tags.split(",")}, headers)


@when('I attempt to annotate the archived contract "{name}" with my current role')
def step_when_attempt_annotate_archive(context, name):
    headers = getattr(context, "headers", {})
    _annotate_archived_contract(
        context, name, {"summary": "BDD unauthorized annotation attempt"}, headers
    )


def _archive_entry_for(context, name):
    """Fetch the archive entry for the named contract via a DID-filtered
    archive search, so the assertion reads what the API serves (not what the
    annotate call echoed back)."""
    import requests as _requests  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    resp = _requests.get(
        archive_search_url(context),
        params={"did": did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"archive search for '{name}' failed: {resp.status_code} {resp.text}"
    entries = resp.json()
    assert isinstance(entries, list) and len(entries) > 0, (
        f"Expected an archive entry for contract '{name}' (did={did}), got: {entries}"
    )
    return entries[0]


@then('the archive entry for contract "{name}" carries summary "{summary}" and tags "{tags}"')
def step_then_archive_entry_annotation(context, name, summary, tags):
    entry = _archive_entry_for(context, name)
    assert entry.get("archive_summary") == summary, (
        f"Expected archive_summary {summary!r}, got: {entry.get('archive_summary')!r}"
    )
    expected_tags = tags.split(",")
    assert entry.get("archive_tags") == expected_tags, (
        f"Expected archive_tags {expected_tags}, got: {entry.get('archive_tags')}"
    )


@then('the archive entry for contract "{name}" carries a generated summary derived from its version and state')
def step_then_archive_entry_generated_summary(context, name):
    # No summary was supplied, so the system derives one from the archived
    # contract's own metadata (name, version, state, creator). The contract's
    # stored name is its template-derived title, so the deterministic anchors
    # to assert are the version and lifecycle state.
    entry = _archive_entry_for(context, name)
    generated = entry.get("archive_summary") or ""
    # The contract reached SIGNED; signing completion auto-deploys to ACTIVE
    # (DCS-FR-CWE-06/SM-12), so by archive time the summary's state token is
    # legitimately either. The invariant is that the derived summary names the
    # version and the current lifecycle state.
    assert "version" in generated.lower() and ("SIGNED" in generated or "ACTIVE" in generated), (
        f"Expected a metadata-derived summary naming the version and state, got: {generated!r}"
    )


@then('the archive annotation of contract "{name}" is recorded in the archive audit log')
def step_then_archive_annotation_audited(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    event_types_for_did = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        # Workflow events (delete/annotate) live in the PAC audit trail under
        # the CONTRACT_STORAGE_ARCHIVE component; /archive/audit serves the
        # archive-integrity view (entries + notary-chain checks).
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "CONTRACT_STORAGE_ARCHIVE", "justification": "BDD archive audit review"},
            headers=headers,
        )
        assert resp.status_code == 200, f"Archive audit query failed: {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of audit scopes, got: {entries}"
        event_types_for_did = [
            str(entry.get("event_type", "")).upper()
            for scope_result in entries
            if isinstance(scope_result, dict)
            for entry in (scope_result.get("audit_trail") or [])
            if isinstance(entry, dict) and entry.get("did") == did
        ]
        if "ANNOTATE_ARCHIVED_CONTRACT" in event_types_for_did:
            return
        time.sleep(2)
    assert "ANNOTATE_ARCHIVED_CONTRACT" in event_types_for_did, (
        f"Expected an ANNOTATE_ARCHIVED_CONTRACT audit event for contract '{name}' ({did}), "
        f"got event types for this DID: {event_types_for_did}"
    )
