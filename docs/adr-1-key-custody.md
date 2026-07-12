# ADR-1: PKCS#11/HSM as the single key-custody mechanism

## Context

DCS signs five distinct kinds of artifact: PAdES contract signatures, C2PA
claim signatures, C2PA lifecycle-assertion signatures, the PID-binding
signing-summary VC, and OID4VP request objects (JAR). Each needs an
asymmetric key and a signing operation, and each was, at different points,
tempted toward its own key-storage mechanism (a Vault transit engine for
VCs — see [adr-ocmw-vc-signing.md](adr-ocmw-vc-signing.md) — raw PEM files
checked into `backend/certs/dev/` for early PAdES/C2PA prototyping).

The SRS requires a standardized, swappable key-custody interface
(IR-HI-01) and eIDAS-aligned crypto-agility with rotation and revocation
(DCS-OR-C2PA-007). Two (or five) different custody mechanisms cannot be
rotated or revoked as one operation, and a raw PEM file checked into git is
not a credible key-custody story at all.

## Decision

All five signing touchpoints resolve to **one PKCS#11 interface**
(`backend/internal/base/hsm`), backed by SoftHSM2 in dev/CI and swappable
for a real HSM in production by configuration only (module path + token
label). Committed PEM private keys are not used anywhere in the shipped
product (clean-product sweep item 4).

## Consequences

- One rotation/revocation drill (A5) covers every signature kind in the
  system, not five separate ones.
- Swapping SoftHSM2 for a production HSM is a `PKCS11_MODULE_PATH` /
  `PKCS11_TOKEN_LABEL` change, not a code change.
- The Vault-based VC signer described in the original OCM-W ADR was
  retired in favor of this single mechanism; see that ADR's updated
  "Decision" section for the migration note.
- Trust anchoring (which certificate chain a verifier trusts) is a
  separate, swappable concern from key *custody* (where the private key
  lives) — dev uses a custom CA with CRL, production is designed to swap to
  the EU LOTL (`eutrustpool.go`) by configuration.
