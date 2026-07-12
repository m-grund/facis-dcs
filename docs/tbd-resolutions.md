# Appendix B TBD Resolutions

The SRS (§3.6.7 Deadline) requires that every item in Appendix B "SHOULD be
resolved before the proof-of-concept phase with documented evidence of
resolution stored in the project repository." This document is that
evidence for TBD-A and TBD-B.

## TBD-A: Use of Qualified Electronic Seals in Digital Contracts

**SRS status:** Open. **Resolution criteria (SRS Appendix B):** (1) written
legal advice on whether QSeals are sufficient/complementary for contract
formation under applicable law, and (2) confirmation of the D-Trust seal
solution's technical/certification properties.

**Resolution:** Neither criterion can be closed from within this project —
both require external legal counsel and a vendor engagement that are
outside engineering's authority to obtain. The SRS's own product scope
already anticipates this: it explicitly descopes QES/QSeal and focuses v1 on
Advanced Electronic Signatures (AES), leaving remote QES/TSP integration as
an integrating party's future responsibility. DCS follows that scope
decision:

- Contracts are signed with an **organization's AES key held in an HSM**
  (PKCS#11), which is the QSCD-ready interface the SRS anticipates a future
  QES integration would sit behind — swapping in a certified QSCD instead of
  the current SoftHSM2 dev token is a configuration change, not an
  architecture change.
- Every signature carries a **PID identity binding** (the natural person
  authorizing the org signature), satisfying the traceability concern the
  SRS raises for QES without requiring QES itself.
- The **D-Trust QSeal integration point** is documented, not built: see the
  Signature Manager's signer interface (`backend/internal/base/hsm`) and
  ADR-3, which record where a QSeal/TSP call would be inserted once legal
  advice on TBD-A's first resolution criterion arrives.

TBD-A therefore remains open exactly as the SRS states — it is the SRS's own
acknowledgment that organizational-signature legal form is unsettled — but
DCS's AES-plus-PID-binding path is a complete, shippable v1 that does not
block on it. Tracked as Deviation Register entry 1.

## TBD-B: Use of XFSC PCM as Personal Identity Wallet

**SRS status:** Open. **Resolution criteria (SRS Appendix B):** a status and
availability report from ECO Verband/XFSC wallet experts on XFSC PCM's
(including Cloud PCM) current status, roadmap, and deployment availability.

**Resolution:** That report is external and has not landed. DCS resolves
the *architectural* dependency without waiting for it: the signing ceremony
is built against **EUDIPLO**, an ARF-conformant, OIDF-conformance-tested
wallet-facing layer, using the standard **OID4VP** presentation flow. XFSC
PCM is a wallet *implementation* behind that same OID4VP contract — DCS
does not talk to a specific wallet product, it talks to OID4VP. When PCM's
availability report lands and confirms a deployable PCM (Cloud or
otherwise), it becomes a drop-in wallet choice behind the existing
EUDIPLO/OID4VP integration; no DCS-side signing-ceremony code changes.

TBD-B remains open per the SRS pending that external report, but the choice
of EUDIPLO is DCS's documented answer to "how do we build the wallet-facing
signing flow today, compatibly with whatever PCM becomes." See ADR-3.
