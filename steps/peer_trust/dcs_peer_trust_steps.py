"""BDD steps for the two-instance-peer-trust requirement (Workstream C1-C3,
docs/anforderung.md).

Covers only the BDD-testable ACs (AC2, AC3, AC4, AC6). AC1 and AC5 are
"manueller-Drill" per the analyst's Pruefmittel column and are deliberately
NOT implemented here — the verifier checks those against the recorded manual
demo evidence, not a Gherkin scenario.

AC2/AC3 single-instance testing technique
------------------------------------------
AC2 (`post_sync`) and AC3 (`action`) both authenticate the calling peer via a
did:web challenge-response signature (see backend/internal/service/
dcs_to_dcs.go): the caller signs a fresh `secret_value` with its private key,
and the receiving instance resolves `https://<hostname>/.well-known/did.json`
(hostname derived ONLY from the did:web host component — see
`identity.DIDWebToHostname`, which stops at the first ":" after the "did:web:"
prefix and ignores everything after) to fetch the matching public key.

That means a did:web identifier of the shape
`<this-instance's-own-did:web-id>:<arbitrary-suffix>` resolves, hostname-wise,
to THIS SAME running instance's own `/.well-known/did.json` and dev private
key (`backend/certs/dev/signing-<port>.key`) — so signing with that key
produces a genuinely valid signature for that synthetic identifier, without
needing a second real DCS process. Crucially the synthetic identifier is a
DIFFERENT STRING than the instance's real DID id, so:
  - it does not trip PostSync's separate same-peer guard
    (`req.FromPeerDid == localPeer`, dcs_to_dcs.go ~line 378), which would
    otherwise reject self-simulated same-DID requests for an unrelated
    reason and make an AC2 test dishonest; and
  - it can be independently seeded into (AC4) or kept absent from (AC2/AC3)
    the local `trusted_peers` table, exercising exactly the third trust
    layer trustedpeercheck.go documents (allowlist, distinct from
    cryptographic validity).

This technique is the natural single-instance extension of the self-peer
simulation already used for AC4 of the contract-state-machine-refactor
requirement (see steps/template_management/contract_state_machine_steps.py,
`_self_peer_action_credentials`), adapted here to also cover the PostSync
same-peer guard.
"""

import base64
import os
import time
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    contract_create_url,
    contract_offer_url,
    contract_peer_action_url,
    contract_peer_post_sync_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _dev_signing_key_path,
    _did_web_to_hostname,
    _sign_secret_value_with_dev_key,
)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _own_identity(context):
    """Fetch this instance's own did:web document and derive the matching
    checked-in dev signing key path (see contract_state_machine_steps for the
    port-to-key mapping and its documented limitation to the two checked-in
    dev identities, backend/certs/dev/did-8991.json / did-8992.json)."""
    resp = _requests.get(
        f"{context.base_url}/.well-known/did.json",
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, (
        f"could not fetch this instance's own did:web document from "
        f"{context.base_url}/.well-known/did.json: {resp.status_code} {resp.text}"
    )
    real_did = resp.json().get("id")
    assert real_did, f"own did.json response has no 'id' field: {resp.text}"
    hostname = _did_web_to_hostname(real_did)
    key_path = _dev_signing_key_path(hostname)
    return real_did, key_path


def _synthetic_peer_credentials(context, marker: str):
    """Build a syntactically valid, cryptographically genuine did:web peer
    identity that is NOT this instance's own DID string (see module
    docstring) and a matching challenge-response signature over a fresh
    secret_value."""
    real_did, key_path = _own_identity(context)
    synthetic_did = f"{real_did}:{marker}-{uuid.uuid4()}"
    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(key_path, secret_value)
    secret_hash = base64.b64encode(signature).decode()
    return synthetic_did, secret_value, secret_hash


def _seed_trusted_peer(context, peer_did: str):
    """Insert peer_did into trusted_peers directly via the test DB
    connection (context.db, see environment.py) rather than relying on any
    particular env-var-based seeding mechanism the implementer may still be
    building (e.g. DCS_TRUSTED_PEERS, docs/anforderung.md C1) — this keeps
    the scenario robust regardless of how that mechanism ends up wired."""
    cursor = context.db.cursor()
    cursor.execute(
        "INSERT INTO trusted_peers (peer_did) VALUES (%s) ON CONFLICT (peer_did) DO NOTHING",
        (peer_did,),
    )
    context.db.commit()
    cursor.close()


def _minimal_remote_contract_payload(from_peer_did: str, contract_did: str) -> dict:
    """A minimal, schema-valid DCSToDCSContractItem (see
    backend/design/dcs_to_dcs.go) plus empty task/negotiation lists —
    enough to exercise PostSync's trust checks and RemoteCreate path without
    depending on unrelated fields this requirement doesn't cover."""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return {
        "contract": {
            "did": contract_did,
            "contract_version": 1,
            "state": "DRAFT",
            "created_by": from_peer_did,
            "created_at": now,
            "updated_at": now,
            "template_did": "urn:uuid:bdd-peer-trust-remote-template",
            "template_version": 1,
            "responsible": {
                "creator": from_peer_did,
                "approvers": [],
                "reviewers": [],
                "negotiators": [],
            },
            "contract_data": {
                "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
                "@type": "dcs:Contract",
            },
            "origin": from_peer_did,
        },
        "review_tasks": [],
        "approval_tasks": [],
        "negotiation_tasks": [],
        "negotiation_items": [],
        "negotiation_decisions": [],
    }


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given("a cryptographically valid peer DID that is not listed in trusted_peers")
def step_given_untrusted_peer(context):
    synthetic_did, secret_value, secret_hash = _synthetic_peer_credentials(context, "bdd-untrusted-peer")
    context.peer_from_did = synthetic_did
    context.peer_secret_value = secret_value
    context.peer_secret_hash = secret_hash


@given("a cryptographically valid peer DID that is listed in trusted_peers")
def step_given_trusted_peer(context):
    synthetic_did, secret_value, secret_hash = _synthetic_peer_credentials(context, "bdd-trusted-peer")
    _seed_trusted_peer(context, synthetic_did)
    context.peer_from_did = synthetic_did
    context.peer_secret_value = secret_value
    context.peer_secret_hash = secret_hash


@given('contract "{name}" exists locally, created by this instance')
def step_given_local_contract(context, name):
    ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# When
# ---------------------------------------------------------------------------


@when("that peer posts a full-state sync for a brand-new contract to this instance")
def step_when_post_sync_new_contract(context):
    contract_did = f"did:example:bdd-peer-sync-{uuid.uuid4()}"
    context.peer_sync_contract_did = contract_did
    payload = _minimal_remote_contract_payload(context.peer_from_did, contract_did)
    payload["from_peer_did"] = context.peer_from_did
    payload["secret_value"] = context.peer_secret_value
    payload["secret_hash"] = context.peer_secret_hash
    context.requests_response = post_json(context, contract_peer_post_sync_url(context), payload, headers={})


@when('that peer attempts to approve contract "{name}" via the peer action endpoint')
def step_when_peer_action_approve(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    payload = {
        "action": "approve",
        "component": "CONTRACT_WORKFLOW_ENGINE",
        "from_peer_did": context.peer_from_did,
        "payload": {"did": did, "updated_at": updated_at},
        "secret_value": context.peer_secret_value,
        "secret_hash": context.peer_secret_hash,
    }
    context.requests_response = post_json(context, contract_peer_action_url(context), payload, headers={})


@when("the initiator creates a contract with a raw peer DID as reviewer, approver, and negotiator")
def step_when_create_contract_raw_peer_did(context):
    t_did = ContractService._create_approved_template_for_contract(context)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    # A raw did:web peer identity (this instance's own, fetched from its
    # public did.json) — deliberately NOT a username and NOT any
    # authenticated user's JWT sub, since AC6 claims this must work without
    # a JWT-sub binding (see the stale comment at
    # frontend/ClientApp/src/utils/participant-selection.ts:1).
    peer_did = ContractService._local_peer_did(context)
    context.raw_peer_did_used = peer_did
    context.contract_creator_headers = creator_h
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
        headers=creator_h,
    )


# ---------------------------------------------------------------------------
# Then
# ---------------------------------------------------------------------------


def _assert_rejected_for_trust_reason(context):
    resp = context.requests_response
    assert resp.status_code != 200, (
        "Expected the request from a cryptographically valid but unlisted peer DID to be "
        f"rejected, got 200: {resp.text}"
    )
    body_text = resp.text.lower()
    assert "trust" in body_text or "untrusted" in body_text or "allow" in body_text, (
        "Expected the rejection to name the trusted_peers allowlist (SRS NFR-BR-08) as the "
        "reason — not a different failure that happens to also be non-200 (e.g. PostSync's "
        "unrelated same-peer guard, a decode/validation error, or a transition-table "
        f"rejection) — got {resp.status_code}: {resp.text}"
    )


@then("the post_sync request is rejected because the peer is not in trusted_peers")
def step_then_post_sync_rejected_untrusted(context):
    _assert_rejected_for_trust_reason(context)


@then("the peer action request is rejected because the peer is not in trusted_peers")
def step_then_peer_action_rejected_untrusted(context):
    _assert_rejected_for_trust_reason(context)


@then('the contract "{name}" was not modified by the untrusted peer action')
def step_then_contract_unmodified(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    actual_state = str(retrieve.json().get("state", "")).upper()
    assert actual_state == "DRAFT", (
        f"Expected contract '{name}' to remain unmodified (DRAFT) after the rejected untrusted "
        f"peer action, got '{actual_state}'"
    )


@then('the contract data is accepted and stored locally with state "{state}"')
def step_then_post_sync_accepted_and_stored(context, state):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the trusted peer's post_sync to be accepted, got {resp.status_code}: {resp.text}"
    )
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(
        context, contract_retrieve_by_id_url(context, context.peer_sync_contract_did), headers=manager_h
    )
    assert retrieve.status_code == 200, (
        f"Expected the synced contract to be retrievable locally after post_sync, got "
        f"{retrieve.status_code}: {retrieve.text}"
    )
    actual_state = str(retrieve.json().get("state", "")).upper()
    assert actual_state == state.strip().upper(), (
        f"Expected the locally-stored contract state to be '{state}' after accepting the "
        f"trusted peer's post_sync, got '{actual_state}'"
    )


@then("the contract is created with that raw peer DID recorded as reviewer, approver, and negotiator")
def step_then_raw_peer_did_recorded(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        "Expected contract creation with a raw peer DID as reviewer/approver/negotiator to "
        f"succeed without a JWT-sub binding check, got {resp.status_code}: {resp.text}"
    )
    c_did = resp.json().get("did")
    assert c_did, f"contract create response has no 'did': {resp.text}"
    retrieve = get_with_headers(
        context, contract_retrieve_by_id_url(context, c_did), headers=context.contract_creator_headers
    )
    assert retrieve.status_code == 200, retrieve.text
    responsible = retrieve.json().get("responsible") or {}
    peer_did = context.raw_peer_did_used
    assert peer_did.startswith("did:web:"), (
        f"expected a raw did:web peer identity for AC6, got '{peer_did}'"
    )
    for role_key in ("reviewers", "approvers", "negotiators"):
        assert peer_did in (responsible.get(role_key) or []), (
            f"Expected raw peer DID '{peer_did}' among '{role_key}': {responsible}"
        )


# ---------------------------------------------------------------------------
# AC7 / AC8 — genuine two-instance scenarios (@two-instance)
#
# These require a SECOND real DCS process (instance B) that trusts, and is
# trusted by, instance A — i.e. Workstream C2 ("Second-instance runner",
# docs/anforderung.md) plus C1's reciprocal trusted_peers seeding. Neither
# exists yet at the time this pack was written. Per the architect's guidance
# these scenarios are still written now (targeting BDD_DCS_BASE_URL_A /
# BDD_DCS_BASE_URL_B, not the single-instance BDD_DCS_BASE_URL) so that they
# are ready to run the moment C2 lands; until then they fail fast with an
# explicit message naming the missing runner, which is the expected/correct
# red state — not a defect in this BDD pack.
#
# A genuine backend gap surfaced while writing AC8: the C4 transition table
# (backend/internal/contractworkflowengine/datatype/contractstate/
# transition.go) only allows Offered -> {Withdrawn, Terminated} — there is
# currently NO declared path from Offered back into Negotiation/Submitted/
# Reviewed/Approved. That means AC8's "submit/review/approve complete on
# both sides after Offer" is not reachable yet even on a single instance,
# independent of the two-instance runner. This scenario intentionally
# exercises that real path (rather than working around it) so it stays red
# for the right reason until the table is extended — flagged here for the
# analyst/architect rather than silently patched.
# ---------------------------------------------------------------------------


@contextmanager
def _as_instance(context, base_url):
    """Temporarily point context.base_url at a different running DCS
    instance so the existing single-instance ContractService/AuthService
    helpers (which all read context.base_url via steps.support.api_client)
    can be reused against instance A or instance B without duplicating their
    ~80 lines of template/contract setup logic for two-instance scenarios."""
    previous = context.base_url
    context.base_url = base_url
    try:
        yield
    finally:
        context.base_url = previous


@given("instance A and instance B are both running and trust each other")
def step_given_two_instances_running(context):
    base_url_a = os.getenv("BDD_DCS_BASE_URL_A", "").rstrip("/")
    base_url_b = os.getenv("BDD_DCS_BASE_URL_B", "").rstrip("/")
    assert base_url_a and base_url_b, (
        "BDD_DCS_BASE_URL_A and BDD_DCS_BASE_URL_B must both be set to run this @two-instance "
        "scenario. This requires the second-instance runner (docs/anforderung.md Workstream "
        "C2: extend dev-stack.sh to optionally launch a second DCS instance on :8992 with "
        "reciprocal DCS_TRUSTED_PEERS seeding against instance A) — which does not exist yet. "
        "This is an open point for C1/C2, not a defect in this scenario."
    )
    context.base_url_a = base_url_a
    context.base_url_b = base_url_b

    did_a = _requests.get(f"{base_url_a}/.well-known/did.json", timeout=context.http_timeout_seconds)
    assert did_a.status_code == 200, f"instance A did.json unreachable: {did_a.status_code} {did_a.text}"
    did_b = _requests.get(f"{base_url_b}/.well-known/did.json", timeout=context.http_timeout_seconds)
    assert did_b.status_code == 200, f"instance B did.json unreachable: {did_b.status_code} {did_b.text}"

    context.peer_did_a = did_a.json().get("id")
    context.peer_did_b = did_b.json().get("id")
    assert context.peer_did_a, f"instance A did.json has no 'id': {did_a.text}"
    assert context.peer_did_b, f"instance B did.json has no 'id': {did_b.text}"


@when(
    "the initiator on instance A creates and offers a contract with instance B "
    "as negotiator and approver"
)
def step_when_create_and_offer_cross_instance(context):
    with _as_instance(context, context.base_url_a):
        # NOTE: ContractService._create_approved_template_for_contract calls
        # AuthService.get_headers_for_roles(...) internally WITHOUT an
        # explicit api_base, so those internal template-lifecycle calls
        # authenticate against os.getenv("BDD_DCS_BASE_URL") rather than the
        # context.base_url swapped in by _as_instance. This only produces a
        # correct evidence trail if BDD_DCS_BASE_URL_A == BDD_DCS_BASE_URL
        # (i.e. instance A is conventionally "the" default single-instance
        # URL in the two-instance dev setup, per docs/anforderung.md C2).
        # Flagging this here rather than silently relying on it: if the
        # two-instance runner ever assigns A a different URL than the
        # single-instance default, this helper needs an api_base-aware
        # variant of ContractService's template setup.
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)
        # Reviewer = A's own identity (Origin == localPeer, so review can
        # complete locally without depending on the still-open C1/C2 points);
        # negotiator/approver = B, per AC7's own wording ("B als Negotiator +
        # Approver").
        create_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
                "reviewers": [context.peer_did_a],
                "negotiators": [context.peer_did_b],
                "approvers": [context.peer_did_b],
            },
            headers=creator_h,
        )
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")
        context.cross_instance_contract_did = c_did
        context.cross_instance_creator_headers = creator_h

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        offer_resp = post_json(
            context, contract_offer_url(context), {"did": c_did, "updated_at": updated_at}, headers=creator_h
        )
        context.requests_response = offer_resp
        assert offer_resp.status_code == 200, offer_resp.text


@then("the contract appears on instance B in state OFFERED within a few seconds")
def step_then_contract_offered_on_b(context):
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    deadline = time.monotonic() + 15
    actual_state = None
    last_resp = None
    while time.monotonic() < deadline:
        last_resp = _requests.get(
            f"{context.base_url_b}/contract/retrieve/{context.cross_instance_contract_did}",
            headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        if last_resp.status_code == 200:
            actual_state = str(last_resp.json().get("state", "")).upper()
            if actual_state == "OFFERED":
                return
        time.sleep(1)
    assert actual_state == "OFFERED", (
        "Expected the contract created on instance A to replicate to instance B as OFFERED "
        f"within a few seconds; last observed state: '{actual_state}' (last response: "
        f"{last_resp.status_code if last_resp else 'n/a'} {last_resp.text if last_resp else ''})"
    )


@when(
    "the parties complete negotiation acceptance, submit, review, and approval on both sides"
)
def step_when_full_approval_cross_instance(context):
    c_did = context.cross_instance_contract_did

    with _as_instance(context, context.base_url_a):
        creator_h = context.cross_instance_creator_headers

        # Draft/Offered -> Negotiation -> Submitted (creator submits twice,
        # same pattern as the single-instance contract-state-machine-refactor
        # pack). NOTE: per the module-level comment above, the transition
        # table does not (yet) declare Offered -> Negotiation as a legal
        # outcome — this call is expected to surface that gap honestly.
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        submit_payload = {
            "did": c_did,
            "updated_at": updated_at,
            "reviewers": [context.peer_did_a],
            "approvers": [context.peer_did_b],
            "negotiators": [context.peer_did_b],
        }
        first_submit = post_json(context, f"{context.base_url_a}/contract/submit", submit_payload, headers=creator_h)
        context.requests_response = first_submit
        if first_submit.status_code != 200:
            return  # surfaced to the Then step below

    # Negotiation -> Submitted is gated by submit.go's Negotiation branch on
    # IsValidNegotiator(cmd.CauserDID) — the CAUSER must itself be a listed
    # negotiator. Since B (not A) is the sole negotiator here, this can only
    # be satisfied by B itself calling negotiate/respond/submit against its
    # own endpoint; B's local copy has Origin=A, so each of these calls
    # transparently forwards to A via the existing peer-action machinery
    # (negotiate.go / acceptnegotiation.go both do the same
    # `Origin != localPeer` forwarding check already proven by AC7's offer
    # replication) — no manual peer-action signing needed here, unlike the
    # untrusted-peer simulation in AC2/AC3.
    with _as_instance(context, context.base_url_b):
        negotiator_h = AuthService.get_headers_for_roles(["Contract Negotiator"], api_base=context.base_url_b)

        retrieve_b = get_with_headers(context, f"{context.base_url_b}/contract/retrieve/{c_did}", headers=negotiator_h)
        assert retrieve_b.status_code == 200, (
            f"Expected instance B to already have the contract replicated before negotiating: "
            f"{retrieve_b.status_code} {retrieve_b.text}"
        )

        # negotiate() forwards to A (the origin) since B isn't origin, so the
        # optimistic-concurrency check there compares against A's actual
        # updated_at — read that directly rather than risking a stale value
        # from B's replica (which only catches up asynchronously via
        # post_sync).
        retrieve_fresh = get_with_headers(context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h)
        assert retrieve_fresh.status_code == 200, retrieve_fresh.text
        updated_at_fresh = retrieve_fresh.json().get("updated_at")

        negotiate_resp = post_json(
            context,
            f"{context.base_url_b}/contract/negotiate",
            {
                "did": c_did,
                "updated_at": updated_at_fresh,
                "negotiated_by": "instance-b-negotiator",
                # negotiationmerging.ChangeRequest is a struct of optional
                # pointer fields — an empty object is a valid "no actual
                # changes, just closing out the negotiation round" proposal.
                "change_request": {},
            },
            headers=negotiator_h,
        )
        context.requests_response = negotiate_resp
        if negotiate_resp.status_code != 200:
            return

    # The negotiation record (and its id, needed to respond to it) only
    # exists authoritatively on A (the origin) — read it back from there.
    with _as_instance(context, context.base_url_a):
        retrieve_a = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve_a.status_code == 200, retrieve_a.text
        negotiations = retrieve_a.json().get("negotiations") or []
        assert negotiations, f"Expected at least one negotiation record on instance A after B's negotiate call, got: {retrieve_a.text}"
        negotiation_id = negotiations[-1].get("id")
        assert negotiation_id, f"Negotiation record has no id: {negotiations[-1]}"

    with _as_instance(context, context.base_url_b):
        respond_resp = post_json(
            context,
            f"{context.base_url_b}/contract/respond",
            {"id": negotiation_id, "did": c_did, "action_flag": "ACCEPTING"},
            headers=negotiator_h,
        )
        context.requests_response = respond_resp
        if respond_resp.status_code != 200:
            return

        # Forwarded writes mutate A's (the origin's) row directly; B's own
        # local replica only catches up later via the async post_sync
        # broadcast, so read the authoritative updated_at from A itself
        # (a plain f-string URL, not the context.base_url-reading
        # contract_retrieve_by_id_url helper, since context.base_url is
        # currently swapped to B inside this _as_instance block).
        retrieve_fresh = get_with_headers(context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h)
        assert retrieve_fresh.status_code == 200, retrieve_fresh.text
        updated_at_fresh = retrieve_fresh.json().get("updated_at")

        negotiator_submit_payload = {
            "did": c_did,
            "updated_at": updated_at_fresh,
            "reviewers": [context.peer_did_a],
            "approvers": [context.peer_did_b],
            "negotiators": [context.peer_did_b],
        }
        negotiator_submit = post_json(
            context, f"{context.base_url_b}/contract/submit", negotiator_submit_payload, headers=negotiator_h,
        )
        context.requests_response = negotiator_submit
        if negotiator_submit.status_code != 200:
            return

        # Since a real negotiation record existed for this contract_version,
        # submit.go's Negotiation branch merges the accepted change and bumps
        # contract_version instead of advancing to Submitted (see
        # negotiationmerging.MergeChangeRequests) — the round stays in
        # Negotiation. The new version has no negotiation record of its own
        # yet, so submitting once more (still as B, the only negotiator)
        # finds no open negotiations and actually advances to Submitted.
        retrieve_after_merge = get_with_headers(context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h)
        assert retrieve_after_merge.status_code == 200, retrieve_after_merge.text
        after_merge_body = retrieve_after_merge.json()
        if after_merge_body.get("state", "").upper() == "NEGOTIATION":
            negotiator_submit_payload["updated_at"] = after_merge_body.get("updated_at")
            negotiator_submit_2 = post_json(
                context, f"{context.base_url_b}/contract/submit", negotiator_submit_payload, headers=negotiator_h,
            )
            context.requests_response = negotiator_submit_2
            if negotiator_submit_2.status_code != 200:
                return

    with _as_instance(context, context.base_url_a):
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=reviewer_h)
        updated_at = retrieve.json().get("updated_at")
        review_submit = post_json(
            context,
            f"{context.base_url_a}/contract/submit",
            {"did": c_did, "updated_at": updated_at, "forward_to": "approval"},
            headers=reviewer_h,
        )
        context.requests_response = review_submit
        if review_submit.status_code != 200:
            return

    with _as_instance(context, context.base_url_b):
        approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=context.base_url_b)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=approver_h)
        assert retrieve.status_code == 200, (
            f"Expected the contract to already be replicated to instance B before B's approver "
            f"acts, got {retrieve.status_code}: {retrieve.text}"
        )
        # As above: B's approve forwards to A (the origin), so use A's
        # authoritative updated_at rather than B's possibly-stale replica
        # (the post_sync catch-up from the reviewer's submit on A may not
        # have landed on B yet).
        retrieve_fresh = get_with_headers(context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h)
        assert retrieve_fresh.status_code == 200, retrieve_fresh.text
        updated_at = retrieve_fresh.json().get("updated_at")
        approve = post_json(
            context, f"{context.base_url_b}/contract/approve", {"did": c_did, "updated_at": updated_at},
            headers=approver_h,
        )
        context.requests_response = approve


@then("the contract state APPROVED is replicated on both instance A and instance B")
def step_then_approved_replicated_both(context):
    assert context.requests_response.status_code == 200, (
        "Expected the full submit/review/approve sequence to complete, got "
        f"{context.requests_response.status_code}: {context.requests_response.text}"
    )
    c_did = context.cross_instance_contract_did
    manager_h_a = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_a)
    manager_h_b = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)

    for label, base_url, headers in (
        ("A", context.base_url_a, manager_h_a),
        ("B", context.base_url_b, manager_h_b),
    ):
        deadline = time.monotonic() + 15
        actual_state = None
        last_resp = None
        while time.monotonic() < deadline:
            last_resp = _requests.get(
                f"{base_url}/contract/retrieve/{c_did}", headers=headers, timeout=context.http_timeout_seconds
            )
            if last_resp.status_code == 200:
                actual_state = str(last_resp.json().get("state", "")).upper()
                if actual_state == "APPROVED":
                    break
            time.sleep(1)
        assert actual_state == "APPROVED", (
            f"Expected contract state APPROVED to be replicated on instance {label}, last "
            f"observed state: '{actual_state}' (last response: "
            f"{last_resp.status_code if last_resp else 'n/a'} {last_resp.text if last_resp else ''})"
        )
