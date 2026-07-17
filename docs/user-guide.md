# User Guide

## Roles

Every DCS user holds one or more of the following roles, disclosed at
login as a verifiable credential claim
(`backend/internal/base/datatype/userrole/userrole.go`, SRS Tables 4/5):

| Role | What it can do |
|---|---|
| Template Creator | Draft new contract templates. |
| Template Reviewer | Review submitted templates. |
| Template Approver | Approve or reject reviewed templates. |
| Template Manager | Register, archive, and search templates. |
| Contract Creator | Create contracts from approved templates. |
| Contract Negotiator | Negotiate contract terms with a counterparty. |
| Contract Reviewer | Review submitted contracts. |
| Contract Approver | Approve or reject reviewed contracts. |
| Contract Manager | Search, terminate, store, and administer contracts. |
| Contract Signer | Complete the signing ceremony (PID presentation + signature). |
| Contract Observer | Read-only visibility into contracts. |
| Archive Manager | Retrieve and search the tamper-evident archive. |
| Auditor | Read audit trails across templates, contracts, and signatures. |
| Sys. Administrator | System-level configuration and monitoring. |
| Compliance Officer | Process audit and compliance reporting (PACM). |
| Integration Manager | Configure or invoke API integrations and event triggers. |
| Process Orchestrator, Validator | Contract Target System / orchestration-facing roles (ORCE). |

A user can hold several roles; the UI shows only the actions a logged-in
user's current role set permits.

## Logging in

DCS authenticates via **OID4VP**: click "Login," scan the presented QR code
(or use the same-device deep link) with an ARF-conformant wallet, and
present your PID plus the role credential(s) issued to you. There is no
username/password login path. A successful presentation redirects you into
the app with your disclosed roles active.

## The contract lifecycle walkthrough

This is the same end-to-end path SRS §1.2 describes as the core story, and
the one the inter-org demo (`dev-stack.sh` + `dev-stack2.sh`, instance A/B)
exercises directly.

1. **Create a template** (Template Creator, `/templates/new`): author
   clauses and text blocks in the builder, define the ODRL policy (duties,
   permissions, prohibitions) against the SLA ontology catalog, save as
   draft.
2. **Review and approve the template** (Template Reviewer → Template
   Approver, `/templates/review/:did` → `/templates/approve/:did`): the
   template becomes usable for contract creation only after approval.
3. **Create a contract from the approved template** (Contract Creator,
   `/contracts/new`): fill in the contract data — parties, SLA values,
   payment terms — bound to the template's placeholders. The contract
   starts in `DRAFT`.
4. **Offer the contract to a counterparty instance**: the contract moves to
   `OFFERED`. In the two-instance demo this is where instance A's contract
   becomes visible on instance B.
5. **Negotiate** (Contract Negotiator, `/contracts/negotiate/:did`): either
   party can propose changes; the contract moves through `NEGOTIATION`
   until both sides accept, or either side withdraws (`WITHDRAWN`).
6. **Submit, review, approve** (Contract Reviewer → Contract Approver,
   `/contracts/review/:did` → `/contracts/approve/:did`): standard
   `SUBMITTED → REVIEWED → APPROVED` path (ADR-2). A contract with a
   constraint-violating value (e.g. a jurisdiction outside the allowed
   list) is refused server-side at this stage even if it slipped past the
   UI — see ADR-6.
7. **Sign** (Contract Signer, `/signing`): a signing ceremony starts —
   present your PID via the wallet again to authorize the organization's
   signature. The resulting PDF is PAdES-signed, carries an embedded C2PA
   manifest plus a remote-manifest fallback link (ADR-4), and embeds the
   signing-summary VC and PID presentation inside the signed byte range
   (ADR-3). The contract moves to `SIGNED`.
8. **Deploy** (automatic on signing, or triggered manually depending on
   configuration): the signed contract is pushed to the configured
   Contract Target System (the example ORCE flow). On acknowledgment the
   contract becomes `ACTIVE` and KPIs appear on its detail view.
9. **Archive**: archiving happens automatically once signing completes —
   it is tamper-evident preservation, not a terminal state; an `ACTIVE`
   contract normally already has an archive entry (ADR-2's reconciliation,
   Deviation Register #4's cited rationale).
10. **Terminate or let it expire**: `ACTIVE → TERMINATED`/`EXPIRED` ends
    the lifecycle. A signature can also be individually `REVOKED`
    (Archive Manager / Contract Manager), which does not delete the
    archive entry but marks the signature as no longer valid.

## Exporting a contract bundle

From a contract's detail view, "Export Bundle" downloads a single ZIP
containing the contract's JSON-LD document, the signed PDF, embedded VCs,
C2PA manifests, deployment evidence, and — for a child contract in a
frame-agreement hierarchy — the parent chain up to the frame document
under `parents/`. Other members of the hierarchy family (e.g. sibling
contracts under the same frame) appear under `related/` when this instance
holds them and you could open them yourself; contracts held only by other
instances or outside your read authorization are simply absent (see
ADR-7). Every file's SHA-256 is listed in `bundle-manifest.json`; export
is refused with a findings list rather than a partial ZIP if a referenced
component is missing.

## Auditing

The Audit view (`/audit`) surfaces the append-only audit trail for
templates, contracts, and signatures — every state transition, who
performed it, and when — for Auditor and Compliance Officer roles.
