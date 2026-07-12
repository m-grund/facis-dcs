"""Shared HTTP and URL helpers for executable BDD scenarios."""

import requests


def origin_url(base_url: str) -> str:
    """Scheme+host only, stripping any path (e.g. route.basePath + '/api').

    did.json is mounted at the bare origin root per the did:web spec
    (backend/cmd/dcs/http.go: didsvr.Mount(mux, didServer) uses the
    unprefixed base mux, not the DCS_API_PATH-prefixed apiMux) — appending
    '/.well-known/did.json' directly to a base_url that already carries
    route.basePath (non-empty in every values.bdd.yml/kind-CI deployment)
    produces a path Goa never registers. Use this helper, not string
    concatenation, wherever the well-known DID document is fetched.
    """
    return "/".join(base_url.split("/", 3)[:3])


def did_document_url(base_url: str) -> str:
    return f"{origin_url(base_url)}/.well-known/did.json"


# URL builders

def contract_create_url(context) -> str:
    return f"{context.base_url}/contract/create"


def contract_update_url(context) -> str:
    return f"{context.base_url}/contract/update"


def contract_submit_url(context) -> str:
    return f"{context.base_url}/contract/submit"


def contract_negotiate_url(context) -> str:
    return f"{context.base_url}/contract/negotiate"


def contract_review_url(context) -> str:
    return f"{context.base_url}/contract/review"


def contract_approve_url(context) -> str:
    return f"{context.base_url}/contract/approve"


def contract_reject_url(context) -> str:
    return f"{context.base_url}/contract/reject"


def contract_retrieve_url(context) -> str:
    return f"{context.base_url}/contract/retrieve"


def contract_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/contract/retrieve/{did}"


def contract_verify_url(context) -> str:
    return f"{context.base_url}/contract/verify"


def contract_offer_url(context) -> str:
    return f"{context.base_url}/contract/offer"


def contract_withdraw_url(context) -> str:
    return f"{context.base_url}/contract/withdraw"


def contract_terminate_url(context) -> str:
    return f"{context.base_url}/contract/terminate"


def contract_search_url(context) -> str:
    return f"{context.base_url}/contract/search"


def contract_audit_url(context) -> str:
    return f"{context.base_url}/contract/audit"


# Deployment (Workstream G, docs/anforderung.md G2): ASSUMED endpoint shapes —
# neither exists in backend/design/*.go yet (grep backend/design -rn "deploy"
# only finds the pre-existing "approve" method's descriptive Meta string and
# the EventDeploy state-machine edge, see contractstate/transition.go). Path/
# names taken verbatim from docs/anforderung.md G2 ("POST /contract/deploy",
# "POST /contract/deployment/callback").

def contract_deploy_url(context) -> str:
    return f"{context.base_url}/contract/deploy"


def contract_deployment_callback_url(context) -> str:
    return f"{context.base_url}/contract/deployment/callback"


def archive_search_url(context) -> str:
    return f"{context.base_url}/archive/search"


def archive_retrieve_url(context) -> str:
    return f"{context.base_url}/archive/retrieve"


def archive_audit_url(context) -> str:
    return f"{context.base_url}/archive/audit"


def pac_audit_url(context) -> str:
    return f"{context.base_url}/pac/audit"


def pac_report_url(context) -> str:
    return f"{context.base_url}/pac/report"


def pac_monitor_url(context) -> str:
    return f"{context.base_url}/pac/monitor"


def contract_peer_action_url(context) -> str:
    return f"{context.base_url}/peer/contracts/action"


def contract_peer_post_sync_url(context) -> str:
    return f"{context.base_url}/peer/contracts/"


def signature_apply_url(context) -> str:
    return f"{context.base_url}/signature/apply"


def signature_revoke_url(context) -> str:
    return f"{context.base_url}/signature/revoke"


def signature_validate_url(context) -> str:
    return f"{context.base_url}/signature/validate"


def signature_retrieve_url(context, did: str) -> str:
    return f"{context.base_url}/signature/retrieve/{did}"


# Signing-ceremony endpoints (Workstream B3, docs/anforderung.md B3): an
# ASSUMED endpoint contract — none of these exist in backend/design/*.go yet
# (grep backend/design -rn "signature/request" returns nothing at the time
# this pack was written). Path/shape taken verbatim from the anforderung.md
# B3 section ("name the start endpoint POST /signature/request: that is the
# SRS's own vocabulary").

def signature_request_url(context) -> str:
    return f"{context.base_url}/signature/request"


def signature_request_by_id_url(context, ceremony_id: str) -> str:
    return f"{context.base_url}/signature/request/{ceremony_id}"


def signature_request_webhook_url(context) -> str:
    return f"{context.base_url}/signature/request/webhook"


# ASSUMED endpoint contract for the PKI-consolidation refactor (Workstream A,
# docs/anforderung.md AC6 / A2.3): a NEW, authenticated, non-public backend
# endpoint that signs a COSE Sig_structure via hsm.Signer("dcs-c2pa") for
# pdf-core. Does not exist in backend/design/*.go yet - see
# features/21_pki_consolidation_pkcs11/pki_consolidation_pkcs11.feature's
# header comment (binding decision 1) for the exact assumed payload shape and
# why the path/shape may need to be adjusted once the architect confirms it.

def c2pa_internal_sign_url(context) -> str:
    return f"{context.base_url}/internal/c2pa/sign"


def template_create_url(context) -> str:
    return f"{context.base_url}/template/create"


def template_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/template/retrieve/{did}"


def template_retrieve_url(context) -> str:
    return f"{context.base_url}/template/retrieve"


def template_submit_url(context) -> str:
    return f"{context.base_url}/template/submit"


def template_update_url(context) -> str:
    return f"{context.base_url}/template/update"


def template_verify_url(context) -> str:
    return f"{context.base_url}/template/verify"


def template_approve_url(context) -> str:
    return f"{context.base_url}/template/approve"


def template_reject_url(context) -> str:
    return f"{context.base_url}/template/reject"


def template_register_url(context) -> str:
    return f"{context.base_url}/template/register"


def template_archive_url(context) -> str:
    return f"{context.base_url}/template/archive"


def template_audit_url(context) -> str:
    return f"{context.base_url}/template/audit"


def template_search_url(context) -> str:
    return f"{context.base_url}/template/search"


# HTTP helpers

def post_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.post(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def put_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.put(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def get_with_headers(context, url: str, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.get(
        url,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


# PDF generation URL builders

def contract_export_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/export/contract/{did}"


def template_export_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/export/template/{did}"


def contract_verify_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/verify/contract/{did}"


def template_verify_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/verify/template/{did}"


# C2PA remote-manifest URL: a public, unauthenticated sibling of
# GET /.well-known/did.json (DCS-OR-C2PA-008).

def c2pa_manifest_url(context, did: str) -> str:
    return f"{context.base_url}/c2pa/manifest/{did}"


# Bundle export URLs: one ZIP per contract/template with an integrity
# manifest (FR-TR-24, FR-CWE-30).

def contract_export_url(context, did: str) -> str:
    return f"{context.base_url}/contract/export/{did}"


def template_export_url(context, did: str) -> str:
    return f"{context.base_url}/template/export/{did}"
