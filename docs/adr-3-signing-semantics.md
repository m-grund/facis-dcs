# ADR-3: Signing semantics — org-key AES with PID identity binding under the signature

> **Superseded in part by [ADR-12](adr-12-wallet-driven-signing.md)
> (2026-07-18).** The "organizational signature produced by the org's HSM-held
> key over the PDF" decision below is **withdrawn**: a DCS-held key cannot
> satisfy eIDAS sole control. The contract AES is now **wallet-driven** — the
> DCS is the OID4VP relying party and validator, and the signatory's key lives
> in their wallet/QTSP. The rest of this ADR — the mandatory PID ceremony, and
> embedding the PID presentation + signing-summary VC **inside** the signed
> byte range before signing — is unchanged and still binding.

## Context

SRS §1.2 describes the signing act as: "the company will sign the DCS
usage contract via OID4VP, with the user signing it using their AES PID."
That sentence packs two different signatures into one legal act — an
organizational signature and a natural person's identity assertion — and
the SRS does not specify how they compose. Getting the composition wrong
(e.g., a PID presentation stored alongside the PDF rather than *inside* the
cryptographically sealed region) would produce a document whose identity
binding could be stripped without invalidating the signature — legally
worthless.

TBD-A and TBD-B (SRS Appendix B) are the
SRS's own acknowledgment that both the legal form of the organizational
signature (QES vs. AES) and the wallet ecosystem for the PID presentation
are externally unsettled.

## Decision

- The **organizational signature** is an Advanced Electronic Signature
  (AES), produced by the org's HSM-held key (ADR-1) over the PDF via
  PAdES.
- Before that signing operation runs, a completed OID4VP PID presentation
  ceremony (via EUDIPLO) is **mandatory** — `apply.go` hard-fails with
  `ErrCeremonyRequired` if no completed ceremony exists.
- The PID presentation and a `ContractSigningSummaryCredential` (a VC
  attesting who authorized the signature, when, and under what
  presentation) are embedded into the PDF's C2PA manifest **before** the
  PAdES signature is computed — inside the signed byte range, not
  alongside it. Stripping the identity binding would break the PAdES
  signature.
- QES/QSeal is out of scope for v1 (TBD-A); the PKCS#11 interface is the
  QSCD-ready integration point for when legal advice on TBD-A's
  resolution criteria arrives.
- The wallet-facing layer is EUDIPLO over standard OID4VP, not a specific
  wallet product (TBD-B) — see ADR-1's sibling reasoning in
  [adr-ocmw-vc-signing.md](adr-ocmw-vc-signing.md).

## Consequences

- A verifier who trusts the PAdES signature transitively trusts the
  identity binding — there is no way to have one without the other.
- This ordering (embed-then-sign, never sign-then-mutate) is also what
  makes the PDF pass external PAdES validators (pyHanko, Adobe): any
  post-signature mutation of the signed byte range is illegal under PAdES,
  so every C2PA/lifecycle stamp must land before the signing call, not
  after.
