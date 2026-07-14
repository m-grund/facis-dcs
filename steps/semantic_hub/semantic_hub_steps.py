"""BDD steps for the Semantic Hub (DCS-FR-TR-03, UC-02-08,
backend/design/semantic_hub.go): versioned schema storage
(/semantic/schema/...), public context resolution (/semantic/context/...),
document anchoring (dcs:schemaRefs injected by the normalization layer), and
ontology-prefix enforcement at template creation."""

import json

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
    template_create_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService


def _hub_url(context, path: str) -> str:
    return f"{context.base_url}{path}"


@when('the active "{kind}" schema "{name}" is retrieved from the Semantic Hub')
def step_when_retrieve_schema(context, kind, name):
    context.requests_response = _requests.get(
        _hub_url(context, "/semantic/schema/retrieve"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )


@then('the retrieved schema is version {version:d}, active, of kind "{kind}"')
def step_then_schema_version(context, version, kind):
    body = context.requests_response.json()
    assert body.get("version") == version, f"Expected version {version}, got: {body.get('version')}"
    assert body.get("active") is True, f"Expected the retrieved schema to be active: {body}"
    assert body.get("kind") == kind, f"Expected kind {kind!r}, got: {body.get('kind')!r}"
    assert body.get("content"), "Expected non-empty schema content"


@then('the retrieved schema content declares the "{prefix}" ontology IRI "{iri}"')
def step_then_schema_declares_iri(context, prefix, iri):
    content = json.loads(context.requests_response.json().get("content"))
    declared = (content.get("@context") or {}).get(prefix)
    assert declared == iri, f"Expected @context.{prefix} == {iri!r}, got: {declared!r}"


@when('the JSON-LD context "{name}" is resolved from the Semantic Hub without authentication')
def step_when_resolve_context(context, name):
    context.requests_response = _requests.get(
        _hub_url(context, f"/semantic/context/{name}"),
        timeout=context.http_timeout_seconds,
    )


@then('the resolved document carries a JSON-LD "@context" object')
def step_then_resolved_context(context):
    body = context.requests_response.json()
    assert isinstance(body.get("@context"), dict) and body["@context"], (
        f"Expected the resolved document to carry a non-empty @context object, got: {body}"
    )


@when('the Template Manager registers a new active version of the "{kind}" schema "{name}" extending the genesis context')
def step_when_register_schema(context, kind, name):
    # Fetch the genesis content and extend it — a plausible new version, not
    # a fabricated unrelated document. The hub's registered versions persist
    # across suite runs (the BDD deployment is not re-seeded per run), so
    # remember how many versions existed beforehand and assert RELATIVE to
    # that, never against absolute version numbers.
    before = _requests.get(
        _hub_url(context, "/semantic/schema/versions"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )
    assert before.status_code == 200, f"versions listing failed: {before.status_code} {before.text}"
    context.hub_versions_before = [v["version"] for v in before.json()]

    genesis = _requests.get(
        _hub_url(context, "/semantic/schema/retrieve"),
        params={"name": name, "kind": kind, "version": 1},
        timeout=context.http_timeout_seconds,
    )
    assert genesis.status_code == 200, f"could not fetch genesis schema: {genesis.text}"
    content = json.loads(genesis.json()["content"])
    content["@context"]["bddExt"] = "https://example.org/bdd-extension#"
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": name,
            "kind": kind,
            "media_type": "application/ld+json",
            "content": json.dumps(content),
            "activate": True,
        },
        headers=headers,
    )


@then("the schema registration reports a version above the genesis version as active")
def step_then_registration_version(context):
    body = context.requests_response.json()
    expected = max(context.hub_versions_before) + 1
    assert body.get("version") == expected, (
        f"Expected the registration to mint version {expected} "
        f"(pre-existing: {sorted(context.hub_versions_before)}), got: {body}"
    )
    assert body.get("active") is True, f"Expected the registered version to be active: {body}"
    context.hub_registered_version = body["version"]


@then('the Semantic Hub lists the registered version of the "{kind}" schema "{name}" as the single active one')
def step_then_versions_listing_registered_active(context, kind, name):
    _assert_versions_listing(
        context, kind, name,
        expect_active=context.hub_registered_version,
        expect_count=len(context.hub_versions_before) + 1,
    )


@then('the Semantic Hub lists version {active:d} of the "{kind}" schema "{name}" as the single active one')
def step_then_versions_listing_absolute_active(context, active, kind, name):
    _assert_versions_listing(
        context, kind, name,
        expect_active=active,
        expect_count=len(context.hub_versions_before) + 1,
    )


def _assert_versions_listing(context, kind, name, *, expect_active, expect_count):
    resp = _requests.get(
        _hub_url(context, "/semantic/schema/versions"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"versions listing failed: {resp.status_code} {resp.text}"
    versions = resp.json()
    assert len(versions) == expect_count, (
        f"Expected {expect_count} versions, got: {[v.get('version') for v in versions]}"
    )
    actives = [v["version"] for v in versions if v.get("active")]
    assert actives == [expect_active], f"Expected exactly version {expect_active} active, got: {actives}"


@when('the Template Manager rolls the "{kind}" schema "{name}" back to version {version:d}')
def step_when_rollback(context, kind, name, version):
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/rollback"),
        {"name": name, "kind": kind, "version": version},
        headers=headers,
    )


@when("I attempt to register a Semantic Hub schema version with my current role")
def step_when_attempt_register(context):
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": "bdd-unauthorized",
            "kind": "profile",
            "media_type": "text/plain",
            "content": "unauthorized",
        },
        headers=getattr(context, "headers", {}),
    )


@then('the contract "{name}" carries a Semantic Hub schema anchor')
def step_then_contract_anchored(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    refs = (retrieve.json().get("contract_data") or {}).get("dcs:schemaRefs") or {}
    anchor = refs.get("dcs:jsonLdContext")
    assert anchor and "/semantic/context/" in anchor, (
        f"Expected a hub-served dcs:schemaRefs.dcs:jsonLdContext anchor, got: {refs}"
    )
    context.hub_anchor_url = anchor


@then('the contract "{name}"\'s JSON-LD context anchor resolves to the hub\'s registered context')
def step_then_anchor_resolves(context, name):
    anchor = context.hub_anchor_url
    # DCS_PUBLIC_URL is unset in the BDD deployment (same convention as the
    # C2PA remote_manifests field), so the anchor is API-root-relative.
    url = anchor if anchor.startswith("http") else f"{context.base_url}{anchor}"
    resp = _requests.get(url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, (
        f"Expected the schema anchor {anchor!r} to resolve against the Semantic Hub, got "
        f"{resp.status_code}: {resp.text}"
    )
    body = resp.json()
    assert isinstance(body.get("@context"), dict), (
        f"Expected the resolved anchor to be the registered JSON-LD context, got: {body}"
    )


@when('a template is created whose "@context" redefines the "{prefix}" prefix to "{iri}"')
def step_when_create_conflicting_template(context, prefix, iri):
    doc = TemplateService.canonical_document_data("BDD Conflicting Ontology Template")
    doc["@context"][prefix] = iri
    headers = AuthService.get_headers_for_roles(["Template Creator"])
    context.requests_response = post_json(
        context,
        template_create_url(context),
        {
            "template_type": TemplateService.template_type_for_category("legal"),
            "name": "BDD Conflicting Ontology Template",
            "description": "must be rejected",
            "template_data": doc,
        },
        headers=headers,
    )


@then("the rejection names the Semantic Hub's active context")
def step_then_rejection_names_hub(context):
    assert "Semantic Hub" in context.requests_response.text, (
        f"Expected the rejection to name the Semantic Hub's active context, got: "
        f"{context.requests_response.text}"
    )
