"""BDD steps for two-instance peer trust (features/17_peer_trust; SRS
NFR-BR-08, DCS-FR-CWE-01/-15).

Untrusted-peer single-instance testing technique
------------------------------------------------
The `post_sync` and `action` peer endpoints authenticate the calling peer via a
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
    (`req.FromPeerDid == localPeer`, dcs_to_dcs.go), which would
    otherwise reject self-simulated same-DID requests for an unrelated
    reason and make the untrusted-peer test dishonest; and
  - it can be independently seeded into or kept absent from
    the local `trusted_peers` table, exercising exactly the third trust
    layer trustedpeercheck.go documents (allowlist, distinct from
    cryptographic validity).

This technique is the natural single-instance extension of the self-peer
simulation used by the contract-state-machine pack (see
steps/template_management/contract_state_machine_steps.py,
`_self_peer_action_credentials`), adapted here to also cover the PostSync
same-peer guard.
"""

import base64
import json
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
    did_document_url,
    get_with_headers,
    origin_url,
    post_json,
    signature_apply_url,
    signature_request_url,
    signature_revoke_url,
    signature_view_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _dev_signing_token_dir,
    _did_web_to_hostname,
    _sign_secret_value_with_dev_key,
)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _own_identity(context):
    """Fetch this instance's own did:web document and derive the matching
    checked-in dev signing key path (see contract_state_machine_steps for the
    port-to-token-dir mapping and its documented limitation to the two
    checked-in dev identities, backend/certs/dev/did-8991.json / did-8992.json)."""
    did_url = did_document_url(context.base_url)
    resp = _requests.get(
        did_url,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, (
        f"could not fetch this instance's own did:web document from "
        f"{did_url}: {resp.status_code} {resp.text}"
    )
    real_did = resp.json().get("id")
    assert real_did, f"own did.json response has no 'id' field: {resp.text}"
    hostname = _did_web_to_hostname(real_did)
    token_dir = _dev_signing_token_dir(hostname)
    return real_did, token_dir


def _synthetic_peer_credentials(context, marker: str):
    """Build a syntactically valid, cryptographically genuine did:web peer
    identity that is NOT this instance's own DID string (see module
    docstring) and a matching challenge-response signature over a fresh
    secret_value."""
    real_did, token_dir = _own_identity(context)
    synthetic_did = f"{real_did}:{marker}-{uuid.uuid4()}"
    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, secret_value)
    secret_hash = base64.b64encode(signature).decode()
    return synthetic_did, secret_value, secret_hash


def _seed_trusted_peer(context, peer_did: str):
    """Insert peer_did into trusted_peers directly via the test DB
    connection (context.db, see environment.py) rather than relying on the
    env-var-based seeding mechanism (DCS_TRUSTED_PEERS) — this keeps the
    scenario independent of how that mechanism is wired."""
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
    # Every broadcast must carry the sender's JAdES over the canonical
    # contract representation (DCS-FR-SM-02) — sign with this instance's own
    # key, exactly like the challenge-response secret below.
    jades_payload = _canonical_jades_payload(contract_did, 1, payload["contract"]["contract_data"])
    payload["jades_signature"] = _jades_sign_as_own_instance(context, jades_payload)
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
    # authenticated user's JWT sub: entering a raw peer DID as participant
    # must work without a JWT-sub binding (see
    # frontend/ClientApp/src/utils/participant-selection.ts).
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
        f"expected a raw did:web peer identity, got '{peer_did}'"
    )
    for role_key in ("reviewers", "approvers", "negotiators"):
        assert peer_did in (responsible.get(role_key) or []), (
            f"Expected raw peer DID '{peer_did}' among '{role_key}': {responsible}"
        )


# ---------------------------------------------------------------------------
# Genuine two-instance scenarios (@two-instance)
#
# These require a SECOND real DCS process (instance B) that trusts, and is
# trusted by, instance A, targeting BDD_DCS_BASE_URL_A / BDD_DCS_BASE_URL_B
# instead of the single-instance BDD_DCS_BASE_URL. The runners providing
# that: dev-stack2.sh locally, tests/bdd/scripts/run_bdd_helm.sh (dcs-a /
# dcs-b releases) in CI. If the URLs are unset, the scenarios fail fast
# with an explicit message naming the missing wiring.
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
    base_url_a = os.getenv("BDD_DCS_BASE_URL_A", "http://localhost:5173/api").rstrip("/")
    base_url_b = os.getenv("BDD_DCS_BASE_URL_B", "http://localhost:5174/api").rstrip("/")
    assert base_url_a and base_url_b, (
        "BDD_DCS_BASE_URL_A and BDD_DCS_BASE_URL_B must both be set to run this @two-instance "
        "scenario: a second DCS instance with reciprocal DCS_TRUSTED_PEERS seeding against "
        "instance A (dev-stack2.sh locally, tests/bdd/scripts/run_bdd_helm.sh in CI)."
    )
    context.base_url_a = base_url_a
    context.base_url_b = base_url_b

    did_a = _requests.get(did_document_url(base_url_a), timeout=context.http_timeout_seconds)
    assert did_a.status_code == 200, f"instance A did.json unreachable: {did_a.status_code} {did_a.text}"
    did_b = _requests.get(did_document_url(base_url_b), timeout=context.http_timeout_seconds)
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
        # URL in the two-instance dev setup).
        # Flagging this here rather than silently relying on it: if the
        # two-instance runner ever assigns A a different URL than the
        # single-instance default, this helper needs an api_base-aware
        # variant of ContractService's template setup.
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)
        # Reviewer = A's own identity (Origin == localPeer, so review can
        # complete locally); negotiator/approver = instance B.
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
        # same pattern as the single-instance contract-state-machine pack).
        # This exercises the Offered -> Negotiation edge of the transition
        # table (contractstate/transition.go, Offered branch).
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
    # `Origin != localPeer` forwarding check already proven by the
    # cross-instance offer replication) — no manual peer-action signing
    # needed here, unlike the untrusted-peer simulation scenarios.
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
        # Accept as a DIFFERENT organization than the one that proposed the
        # change above (negotiator_h) — acceptnegotiation.go's
        # conflict-of-interest guard (FR-CWE-07) now rejects a respond call
        # whose participant identity (the OID4VP credential's organization
        # claim) matches the negotiation's created_by, and both would
        # otherwise default to the same organization ("Acme Corp") since
        # neither call overrides it. This is orthogonal to the peer-DID
        # authorization (IsValidNegotiator/CauserDID) this scenario is
        # actually testing, which is unaffected by which organization the
        # accepting credential carries.
        accepting_negotiator_h = AuthService.get_headers_for_roles(
            ["Contract Negotiator"], api_base=context.base_url_b, organization="TechVendor Inc"
        )
        respond_resp = post_json(
            context,
            f"{context.base_url_b}/contract/respond",
            {"id": negotiation_id, "did": c_did, "action_flag": "ACCEPTING"},
            headers=accepting_negotiator_h,
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


# ---------------------------------------------------------------------------
# Revocation propagation across instances (DCS-NFR-BR-06)
# ---------------------------------------------------------------------------


@when("instance A applies a ceremony-backed signature to the contract")
def step_when_sign_cross_instance(context):
    # Reuses the real-signing pack's ceremony machinery verbatim — every URL
    # builder reads context.base_url, which _as_instance swaps to A.
    from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (  # noqa: PLC0415
        _build_pid_presentation,
        _complete_ceremony_via_webhook,
    )

    with _as_instance(context, context.base_url_a):
        c_did = context.cross_instance_contract_did
        signer_h = AuthService.get_headers_for_roles(["Contract Signer"], api_base=context.base_url_a)
        start = post_json(
            context,
            signature_request_url(context),
            {"contract_did": c_did, "field_name": "PeerRevocationSigner"},
            headers=signer_h,
        )
        assert start.status_code == 200, (
            f"POST /signature/request failed on instance A: {start.status_code} {start.text}"
        )
        ceremony_id = start.json().get("ceremony_id")
        assert ceremony_id, f"/signature/request response has no ceremony_id: {start.text}"

        given_name, family_name = "PeerRevocation", "BDD-Testperson"
        presentation, _issuer_jwt, _disclosures, subject_did = _build_pid_presentation(
            given_name=given_name, family_name=family_name,
            aud="dcs-signature-ceremony", nonce=str(uuid.uuid4()),
        )
        webhook = _complete_ceremony_via_webhook(
            context, ceremony_id, presentation, subject_did, given_name, family_name
        )
        assert webhook.status_code == 200, (
            f"ceremony webhook failed on instance A: {webhook.status_code} {webhook.text}"
        )

        manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_a)
        retrieve = get_with_headers(
            context, contract_retrieve_by_id_url(context, c_did), headers=manager_h
        )
        assert retrieve.status_code == 200, retrieve.text
        apply_resp = post_json(
            context,
            signature_apply_url(context),
            {
                "did": c_did,
                "signer_did": subject_did,
                "credential_type": "AES",
                "updated_at": retrieve.json().get("updated_at"),
            },
            headers=signer_h,
        )
        assert apply_resp.status_code == 200, (
            f"signature apply failed on instance A: {apply_resp.status_code} {apply_resp.text}"
        )
        context.requests_response = apply_resp


@when("instance A revokes the applied signature of the cross-instance contract")
def step_when_revoke_cross_instance(context):
    with _as_instance(context, context.base_url_a):
        c_did = context.cross_instance_contract_did
        manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_a)
        view = _requests.get(
            signature_view_url(context), params={"did": c_did}, headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        assert view.status_code == 200, f"signature view failed on instance A: {view.status_code} {view.text}"
        signatures = view.json().get("signatures") or []
        assert signatures, f"Expected an applied signature to revoke, got: {view.json()}"
        revoke = post_json(
            context,
            signature_revoke_url(context),
            {"did": c_did, "signer_did": signatures[0]["signer_did"]},
            headers=manager_h,
        )
        assert revoke.status_code == 200, (
            f"signature revoke failed on instance A: {revoke.status_code} {revoke.text}"
        )
        context.requests_response = revoke


@then('the contract state "{state}" is replicated on both instance A and instance B')
def step_then_state_replicated_both(context, state):
    c_did = context.cross_instance_contract_did
    expected = state.upper()
    for label, base_url in (("A", context.base_url_a), ("B", context.base_url_b)):
        manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=base_url)
        deadline = time.monotonic() + 15
        actual_state = None
        last_resp = None
        while time.monotonic() < deadline:
            last_resp = _requests.get(
                f"{base_url}/contract/retrieve/{c_did}", headers=manager_h,
                timeout=context.http_timeout_seconds,
            )
            if last_resp.status_code == 200:
                actual_state = str(last_resp.json().get("state", "")).upper()
                if actual_state == expected:
                    break
            time.sleep(1)
        assert actual_state == expected, (
            f"Expected contract state {expected} to be replicated on instance {label}, last "
            f"observed state: '{actual_state}' (last response: "
            f"{last_resp.status_code if last_resp else 'n/a'} {last_resp.text if last_resp else ''})"
        )


# ---------------------------------------------------------------------------
# Approval quorum with two distinct approver peers (DCS-FR-CWE-15/25)
# ---------------------------------------------------------------------------


@when("the initiator on instance A creates and offers a contract requiring approval from both instances")
def step_when_create_offer_dual_approver(context):
    with _as_instance(context, context.base_url_a):
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)
        # Reviewer and negotiator = A's own peer so the pre-approval drive
        # stays local; approvers = BOTH peers so the quorum needs two
        # observably distinct CauserDIDs (the point of this scenario).
        create_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
                "reviewers": [context.peer_did_a],
                "negotiators": [context.peer_did_a],
                "approvers": [context.peer_did_a, context.peer_did_b],
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


@when("instance A drives the contract to the approval stage")
def step_when_drive_to_approval_stage(context):
    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        creator_h = context.cross_instance_creator_headers
        submit_payload = {
            "did": c_did,
            "reviewers": [context.peer_did_a],
            "approvers": [context.peer_did_a, context.peer_did_b],
            "negotiators": [context.peer_did_a],
        }
        # OFFERED -> NEGOTIATION -> SUBMITTED: two creator submits (A is the
        # sole negotiator and there are no open negotiation decisions, same
        # pattern as the single-instance state-machine pack).
        for _ in range(2):
            retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
            assert retrieve.status_code == 200, retrieve.text
            submit_payload["updated_at"] = retrieve.json().get("updated_at")
            resp = post_json(context, f"{context.base_url_a}/contract/submit", submit_payload, headers=creator_h)
            assert resp.status_code == 200, f"submit failed: {resp.status_code} {resp.text}"

        # SUBMITTED -> REVIEWED: reviewer forwards to approval.
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        review_submit = post_json(
            context,
            f"{context.base_url_a}/contract/submit",
            {"did": c_did, "updated_at": retrieve.json().get("updated_at"), "forward_to": "approval"},
            headers=reviewer_h,
        )
        assert review_submit.status_code == 200, (
            f"reviewer forward-to-approval failed: {review_submit.status_code} {review_submit.text}"
        )


def _approve_from_instance(context, base_url):
    c_did = context.cross_instance_contract_did
    creator_h = context.cross_instance_creator_headers
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=base_url)
    # Always read the authoritative updated_at from A (the origin) — B's
    # replica catches up asynchronously (same convention as the
    # APPROVED-replication scenario).
    retrieve = get_with_headers(
        context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h
    )
    assert retrieve.status_code == 200, retrieve.text
    resp = post_json(
        context,
        f"{base_url}/contract/approve",
        {"did": c_did, "updated_at": retrieve.json().get("updated_at")},
        headers=approver_h,
    )
    context.requests_response = resp
    assert resp.status_code == 200, f"approve via {base_url} failed: {resp.status_code} {resp.text}"


@when("instance A's approver approves the contract")
def step_when_approver_a_approves(context):
    _approve_from_instance(context, context.base_url_a)


@when("instance B's approver approves the contract")
def step_when_approver_b_approves(context):
    _approve_from_instance(context, context.base_url_b)


@then("the contract is still not APPROVED because instance B's required approval is open")
def step_then_partial_quorum_holds(context):
    c_did = context.cross_instance_contract_did
    creator_h = context.cross_instance_creator_headers
    retrieve = get_with_headers(
        context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h
    )
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state != "APPROVED", (
        "Quorum violation: the contract reached APPROVED after only instance A's approval, "
        "although instance B's approval task must still be OPEN (approve.go AnyTasksInState guard)"
    )
    assert state == "REVIEWED", (
        f"Expected the contract to remain in REVIEWED awaiting instance B's approval, got '{state}'"
    )


@then("both peers' approval decisions are recorded on the contract's approval tasks")
def step_then_both_approvals_recorded(context):
    # GET /contract/retrieve lists the approval tasks assigned to the VIEWING
    # instance's own peer DID, so each peer's recorded decision is asserted
    # against its own instance. B's task state arrives via the async
    # post_sync broadcast from A (the origin) — poll briefly.
    c_did = context.cross_instance_contract_did
    for label, base_url, peer_did in (
        ("A", context.base_url_a, context.peer_did_a),
        ("B", context.base_url_b, context.peer_did_b),
    ):
        approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=base_url)
        states = {}
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            resp = get_with_headers(context, f"{base_url}/contract/retrieve", headers=approver_h)
            assert resp.status_code == 200, (
                f"contract retrieve on instance {label} failed: {resp.status_code} {resp.text}"
            )
            tasks = [t for t in (resp.json().get("approval_tasks") or []) if t.get("did") == c_did]
            states = {t.get("approver"): str(t.get("state", "")).upper() for t in tasks}
            if states.get(peer_did) == "APPROVED":
                break
            time.sleep(1)
        assert states.get(peer_did) == "APPROVED", (
            f"Expected instance {label}'s own approval task (approver={peer_did}) to be "
            f"recorded APPROVED on instance {label}, got tasks: {states}"
        )


# ---------------------------------------------------------------------------
# JAdES sync provenance (DCS-FR-SM-02)
# ---------------------------------------------------------------------------


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def _canonical_jades_payload(contract_did: str, contract_version: int, contract_document: dict) -> bytes:
    """The canonical contract representation the backend signs
    (internal/base/jades.BuildContractPayload): recursively key-sorted,
    compact, no ASCII escaping."""
    payload = {
        "dcs:contractDid": contract_did,
        "dcs:contractVersion": contract_version,
        "dcs:contractDocument": contract_document,
    }
    return json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def _der_to_jose(der: bytes) -> bytes:
    """Convert an ASN.1 DER ECDSA signature (what hsmsign emits, mirroring
    DIDDocument.Sign) into the 64-byte r||s form JWS ES256 requires."""
    assert der[0] == 0x30, "expected a DER SEQUENCE"
    idx = 2
    if der[1] & 0x80:
        idx = 2 + (der[1] & 0x7F)
    assert der[idx] == 0x02, "expected DER INTEGER (r)"
    rlen = der[idx + 1]
    r = der[idx + 2 : idx + 2 + rlen]
    idx = idx + 2 + rlen
    assert der[idx] == 0x02, "expected DER INTEGER (s)"
    slen = der[idx + 1]
    s = der[idx + 2 : idx + 2 + slen]
    r = r.lstrip(b"\x00").rjust(32, b"\x00")
    s = s.lstrip(b"\x00").rjust(32, b"\x00")
    return r + s


def _own_x5c(context):
    did_url = did_document_url(context.base_url)
    resp = _requests.get(did_url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, f"could not fetch own did.json: {resp.status_code} {resp.text}"
    methods = resp.json().get("verificationMethod") or []
    assert methods, "own did.json has no verificationMethod"
    x5c = (methods[0].get("publicKeyJwk") or {}).get("x5c") or []
    if isinstance(x5c, str):
        x5c = [x5c]
    assert x5c, "own did.json carries no x5c certificate chain"
    return x5c


def _jades_sign_as_own_instance(context, payload_bytes: bytes) -> str:
    """Produce a genuine JAdES baseline-B compact JWS with this instance's
    own dev/HSM key and x5c chain — the same trick the synthetic-peer
    challenge-response signature uses (the synthetic DID resolves to this
    instance's own did.json and key)."""
    _real_did, token_dir = _own_identity(context)
    header = {
        "alg": "ES256",
        "typ": "jose",
        "cty": "application/json",
        "x5c": _own_x5c(context),
        "sigT": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "crit": ["sigT"],
    }
    signing_input = _b64url(json.dumps(header, separators=(",", ":")).encode()) + "." + _b64url(payload_bytes)
    der = _sign_secret_value_with_dev_key(token_dir, signing_input)
    return signing_input + "." + _b64url(_der_to_jose(der))


@when("that peer posts a full-state sync whose JAdES signature covers a different contract document")
def step_when_post_sync_tampered_jades(context):
    """The challenge-response secret and trust listing are VALID here — only
    the JAdES payload binding is wrong (it signs a different contract
    document than the one being synced), so a rejection can only come from
    the receiver's JAdES payload check (DCS-FR-SM-02)."""
    contract_did = f"did:example:bdd-peer-sync-{uuid.uuid4()}"
    context.peer_sync_contract_did = contract_did
    payload = _minimal_remote_contract_payload(context.peer_from_did, contract_did)
    tampered_document = {"@type": "dcs:Contract", "dcs:name": "a different document than the synced one"}
    jades_payload = _canonical_jades_payload(contract_did, 1, tampered_document)
    payload["jades_signature"] = _jades_sign_as_own_instance(context, jades_payload)
    payload["from_peer_did"] = context.peer_from_did
    payload["secret_value"] = context.peer_secret_value
    payload["secret_hash"] = context.peer_secret_hash
    context.requests_response = post_json(context, contract_peer_post_sync_url(context), payload, headers={})


@then("the post_sync request is rejected because the JAdES payload does not match")
def step_then_post_sync_rejected_jades(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected post_sync with a mismatching JAdES payload to be rejected with 400, got "
        f"{resp.status_code}: {resp.text}"
    )
    assert "jades" in resp.text.lower(), (
        f"Expected the rejection to name the JAdES check, got: {resp.text}"
    )


@then("the contract's schemaRefs anchor, as stored on instance B, resolves against instance A's Semantic Hub")
def step_then_schema_ref_resolves_against_a(context):
    """Phase 4 (DCS-to-DCS): dcs:schemaRefs.dcs:shaclShapes is set once, at
    production time on instance A, and synced verbatim — it never gets
    re-anchored to instance B's own hub. This confirms it's still resolvable
    from outside instance A (the reachability precondition
    validation.VerifyAgainstOriginatorHub, called from post_sync, depends
    on): host-relative anchors (no DCS_PUBLIC_URL configured, the BDD
    default) are resolved against instance A's origin, never instance B's."""
    c_did = context.cross_instance_contract_did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    retrieve = _requests.get(
        f"{context.base_url_b}/contract/retrieve/{c_did}",
        headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert retrieve.status_code == 200, retrieve.text
    refs = (retrieve.json().get("contract_data") or {}).get("dcs:schemaRefs") or {}
    anchor = refs.get("dcs:shaclShapes")
    assert anchor, f"Expected the contract stored on instance B to carry a dcs:schemaRefs.dcs:shaclShapes anchor, got: {refs}"

    url = anchor if anchor.startswith("http") else f"{origin_url(context.base_url_a)}{anchor}"
    resp = _requests.get(url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, (
        f"Expected the schemaRefs anchor {anchor!r} to resolve against instance A's Semantic Hub "
        f"({url}), got {resp.status_code}: {resp.text}"
    )
    body = resp.json()
    assert body.get("content"), f"Expected instance A's hub to return SHACL shape content, got: {body}"


@then("instance B stores a JAdES sync-provenance artifact for that contract signed by instance A")
def step_then_provenance_on_b(context):
    """GET /peer/contracts/provenance on instance B (DCS-FR-SM-02): the
    stored artifact must be a structurally valid JAdES baseline-B compact
    JWS from instance A whose payload binds exactly the synced contract.
    (Cryptographic verification already happened server-side — the sync
    would have been rejected otherwise; see the tampered-JAdES scenario.)"""
    c_did = context.cross_instance_contract_did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    resp = None
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        resp = _requests.get(
            f"{context.base_url_b}/peer/contracts/provenance",
            params={"did": c_did},
            headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        if resp.status_code == 200:
            break
        time.sleep(1)
    assert resp is not None and resp.status_code == 200, (
        f"Expected instance B to store sync provenance for {c_did}, got "
        f"{resp.status_code if resp else 'n/a'}: {resp.text if resp else ''}"
    )
    body = resp.json()
    assert body.get("did") == c_did
    assert body.get("from_peer_did") == context.peer_did_a, (
        f"Expected the provenance to name instance A ({context.peer_did_a}) as signer, got: "
        f"{body.get('from_peer_did')}"
    )
    jws = body.get("jades_signature") or ""
    parts = jws.split(".")
    assert len(parts) == 3, f"Expected a compact JWS with three segments, got: {jws[:120]}"

    def _b64url_decode(segment: str) -> bytes:
        return base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4))

    header = json.loads(_b64url_decode(parts[0]))
    assert header.get("alg") == "ES256", f"Expected alg ES256, got: {header.get('alg')}"
    assert header.get("sigT"), "Expected a sigT claimed-signing-time header"
    assert header.get("crit") == ["sigT"], f"Expected crit [sigT], got: {header.get('crit')}"
    assert header.get("x5c"), "Expected an x5c certificate chain in the protected header"

    payload = json.loads(_b64url_decode(parts[1]))
    assert payload.get("dcs:contractDid") == c_did, (
        f"Expected the JAdES payload to bind contract {c_did}, got: {payload.get('dcs:contractDid')}"
    )
    assert payload.get("dcs:contractVersion") == body.get("contract_version"), (
        "Expected the JAdES payload's version to match the stored provenance version"
    )
    assert "dcs:contractDocument" in payload, "Expected the JAdES payload to embed the contract document"
