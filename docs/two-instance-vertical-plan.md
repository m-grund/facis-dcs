# Two-instance negotiation vertical — what we're shipping

Branch: `feat/two_instance_negotiation_vertical`.
Definition of done: **both the BDD (behave on kind) and the Playwright E2E jobs
green on CI.**

## The normative vertical (A = originator ↔ B = counterparty)

One big two-instance Playwright vertical (A + B browser contexts), superseding
the single-instance `full-vertical.spec.ts`. Artifact-verified at **every** hop
on **both** parties (where the artifact exists) via `e2e/verify_artifact.py` —
real **veraPDF** (PDF/A-3a) + **c2patool** (c2pa-rs), the same validators
pdf-core runs.

1. Author a non-trivial **SHACL shape** → Semantic Hub (registered and served).
2. **Component** using that shape + full editor prose (section title, text
   block, the SHACL-backed requirement field, an ODRL policy). Review → approve.
3. **Template** composing the component + custom template-level wrapping.
   Lifecycle → **publish to the Federated Catalogue** (discoverable).
4. **Contract** from the template + a non-trivial editor edit (PDF version bump).
5. **Propose to B.** B receives it in `OFFERED` with a valid embedded C2PA
   manifest, lifecycle banner `proposed`, verified on B's side.
6. **Negotiation ping-pong** (e.g. 20000 → 10000 → 15000, N rounds over the HTTP
   peer exchange). **Every actionable adjustment triggers a new PDF exchange**;
   assert on both A and B: a new PDF version is produced and re-exchanged, both
   converge on it, the C2PA manifest chain grows by one ingredient, banner stays
   `proposed` until settle.
7. **Consolidation / settle.** Mutual agreement flips the banner to `agreed` on
   both sides; **signing is gated until here** (a pre-settle sign attempt is
   refused).
8. **Both sign.** A signs A's field → ships → B signs on top (incremental
   PAdES). The double-signed PDF has two AcroForm signatures, banner `executed`,
   passes veraPDF PDF/A-3a and c2patool, DSS validates both as AES + PAdES-B-T.
9. **Deploy → receipt → KPIs.** Deploy to the target (ORCE); assert the receipt
   proof, the async KPI, its check against the contract's policy, and the
   dashboard OK/violation + performance.
10. **Full audit** across both instances — offer, each counter, settle, both
    signatures, deploy, receipt, KPI — each IPFS-anchored.

Cross-cutting invariant at every artifact hop: PDF/A-3a (veraPDF) + valid C2PA
(c2patool) + embedded canonical JSON-LD associated file + human↔machine hash
match.

## Backend this requires (R5 / R5c)

- **R5** — counter-offers round-trip A↔B (each actionable adjustment →
  `PDF_REGENERATED` → ship, banner `proposed`); the settle/consolidation step
  flips the extrinsic banner to `agreed` and **gates signing** (negotiate →
  settle → sign, per the intrinsic/extrinsic state model, ADR-13).
- **R5c** — the receiver **stores** the received signed PDF rather than
  regenerating it (which would destroy the prior signature); `loadBasePDF`
  already prefers the stored PDF, so B signs on top of A's signature.

## Test primitives

`e2e/multi-dcs-helpers.ts` already has `openInstanceB` / `signOnInstance`. Add:
`verifyArtifact(inst, did, {lifecycle})`, `counterOffer(inst, did,
{field, value})`, `assertReceivedInState(inst, did, state, lifecycle)`,
`assertManifestChainGrew(instA, instB, did)`.
