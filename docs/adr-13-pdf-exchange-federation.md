# ADR-13: DCS-to-DCS federation is a PDF (+ JAdES) exchange, not a state broadcast

## Status

Accepted, rolling out (2026-07-19). **Supersedes the DCS-to-DCS synchronizer**
of [ADR-2](adr-2-contract-state-machine.md) (the origin-as-single-writer /
full-state-broadcast / peer-task-sync model). Builds on
[ADR-12](adr-12-wallet-driven-signing.md) (the signed PDF + JAdES are the
signature artifacts) and [ADR-4](adr-4-c2pa-embedding.md) (the PDF carries the
C2PA provenance chain and the embedded machine-readable JSON-LD).

## Context

The previous federation had the **origin** peer act as the single writer for a
contract: it broadcast the full contract state — the JSON-LD `ContractData`
**plus every review, approval, and negotiation task of every responsible peer**
— to each peer via `POST /peer/contracts/`, and non-origin peers forwarded
their mutations back to the origin via `POST /peer/contracts/action`. Peers
were addressed by putting their did:web DIDs into the contract's
reviewer/approver/negotiator lists (`Responsible.GetUniqueResponsibleList`).

This couples the two organizations' internal workflows: one operator's DCS
creates and owns the other operator's tasks, and role assignment leaks peer
DIDs into what should be internal RBAC. It also re-invents, over a bespoke JSON
protocol, transport of content the **PDF already carries**: pdf-core embeds the
canonical machine-readable JSON-LD (`compiler.ExtractEmbeddedJSONLD`), records
every lifecycle step in the C2PA provenance chain, and holds the signatures
([ADR-4](adr-4-c2pa-embedding.md), [ADR-12](adr-12-wallet-driven-signing.md)).

## Decision

**Two DCS instances exchange exactly two things: the contract PDF, per
lifecycle step, and the JAdES signature artifact, after signing. Nothing
else.** Each DCS runs its **own** workflow and RBAC on its **own** copy; tasks
never cross an instance boundary.

1. **The PDF is the wire format.** A lifecycle step that the counterparty must
   see (offer, a negotiation counter-proposal, a signature) ships the current
   PDF to the counterparty. The receiver asks pdf-core to extract the embedded
   JSON-LD (`POST /manifest/…` sibling — pdf-core owns all byte-level PDF work,
   the DCS never parses PDF bytes) and upserts its local contract copy from it.
   The C2PA provenance chain in the PDF is the authenticated history; the JAdES
   envelope authenticates the sender (did:web + eIDAS, unchanged).

2. **Each DCS is the writer of its own copy.** There is no single-writer
   origin and no forwarded `action`. A party changes terms on its own copy and
   ships the resulting PDF; the counterparty receives it as a proposal.

3. **Negotiation is turn-based document exchange.** To counter, a party edits
   its copy and ships a new PDF version. To accept, a party stops countering
   and **signs** the current version. **The signature is the acceptance** — a
   party only signs a version it agrees to, and a version both parties have
   signed is the executed agreement (two AcroForm signatures over the same
   content). Countering and accepting are the only moves, and both are just a
   PDF (accept additionally carries the JAdES).

4. **Review/approval are internal.** Between agreeing on terms and signing,
   each DCS takes its own copy through its own review/approval gates under its
   own RBAC. These transitions are local and never synced.

5. **Counterparty is a single peer, not a role list.** A contract records the
   one counterparty DCS it is offered to (a did:web), which is where its PDFs
   are shipped. Reviewer/approver/negotiator are internal user roles, isolated
   per instance, and never carry peer DIDs.

## The touch point

```
   DCS A (owns its copy, own RBAC)          │   DCS B (owns its copy, own RBAC)
 ─────────────────────────────────────────────┼──────────────────────────────────────
  offer: ship PDF(v1, terms A)  ──────────────┼──▶ extract JSON-LD, create local copy
                                              │    counter: edit copy, ship PDF(v2) ◀──
  extract, update copy                         │
  counter: edit, ship PDF(v3)   ──────────────┼──▶ extract, update copy
                                              │    accept: internal review/approve,
                                              │      sign v3, ship signed PDF + JAdES ◀─
  see B's signature on v3, sign v3            │
  ship signed PDF + JAdES        ─────────────┼──▶ two signatures over v3 → executed
```

## What is removed

- `POST /peer/contracts/` full-state broadcast (`ContractData` + tasks) and its
  payload types (`remotesync` task/negotiation data).
- `POST /peer/contracts/action` (origin-forwarded mutation) and the
  single-writer-origin rule.
- `GET /peer/contracts/sync` (force a refresh from the origin).
- The peer **task-sync** (`localpeerupdatecmd`/`peerupdaterequestcmd` task
  upserts) — each DCS creates its own tasks locally.
- Peer DIDs in `Responsible` reviewer/approver/negotiator lists and their use
  as the sync recipient list (`GetUniqueResponsibleList` for routing). The
  party set for signature-field seeding is `[origin, counterparty]`.

## What is kept

- did:web + eIDAS peer authentication (challenge-response over the JAdES
  envelope) — the sender of a PDF is a verified, trusted peer.
- The JAdES artifact — now the **signature** exchanged after signing (SM-02),
  not a transport envelope around a JSON broadcast.
- pdf-core as the sole owner of PDF byte work (embed, extract, deterministic
  re-render, C2PA), [ADR-4](adr-4-c2pa-embedding.md); the DCS orchestrates.

## Consequences

- Federation state is derivable from artifacts alone: a received PDF is the
  contract at that step, self-verifying via its C2PA chain and JAdES; there is
  no separate task ledger to reconcile.
- The two operators' internal processes are decoupled — one cannot create or
  observe the other's tasks. This is the correct trust boundary for instances
  run by different organizations.
- The contract state machine ([ADR-2](adr-2-contract-state-machine.md)) keeps
  its per-instance states; the cross-instance transitions (offer, negotiate,
  sign) are driven by shipping/receiving a PDF, and review/approval become
  purely local.

## Implementation state (2026-07-19)

| Piece | State |
| --- | --- |
| ADR + design | this document |
| New peer endpoint carrying the PDF (+ JAdES); receiver extracts JSON-LD via pdf-core | pending |
| Remove `action`, `get_sync`, task-sync payloads + single-writer-origin | pending |
| Counterparty (single did:web) replaces peer-DIDs-in-roles; party set = [origin, counterparty] | pending |
| Localize tasks/RBAC; negotiation = turn-based PDF exchange, sign = accept | pending |
| BDD (peer_trust, real_signing_vertical) + two-instance Playwright vertical on the new base | pending |
