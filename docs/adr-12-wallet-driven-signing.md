# ADR-12: Wallet-driven remote AES — the DCS is the relying party, not the signatory

## Status

Accepted (2026-07-18). **Supersedes the organizational-signature clause of
[ADR-3](adr-3-signing-semantics.md)** (org-key AES over the PDF) and removes
"PAdES contract signatures" from [ADR-1](adr-1-key-custody.md)'s list of DCS
key-custody touchpoints. Everything else in ADR-3 (embed-then-sign ordering,
mandatory PID ceremony, identity binding inside the signed byte range) stands.

## Context

ADR-3 had the DCS's own HSM key produce the contract's AES over the PDF. That
is **legally void under eIDAS**: an Advanced Electronic Signature (Art. 26)
must be created with data the signatory keeps under their **sole control**
(26c) and be **uniquely linked to** and **identify** the signatory (26a, 26b).
A single DCS HSM key signing on behalf of every user is structurally a PAdES
but legally the DCS's signature, not the user's — and it cannot be sole
control when many users share it. An independent, conversation-blind research
pass over the EUDI reference implementations and the consolidated eIDAS text
confirmed both the defect and the corrected direction; Art. 26 is
custody-neutral (Recital 52 permits a remotely-held key under sole control),
and AES needs no QSCD or qualified certificate — that is QES, which is out of
scope (SRS line 199, ADR-3 TBD-A).

The SRS already describes the correct shape: the **wallet** manages the
signer's credentials and performs the signature (SM-06), and the DCS applies
signatures **via an integrated signing service** and enforces integrity
validation upon signing (SM-16), over a standard wallet/TSP interface
(IR-SI-04, IR-SI-10, IR-CI-08).

**Product-scope constraint:** FACIS.DCS does **not** ship a wallet, a QTSP, or
a signature-creation application. Those are separate ecosystem components (a
real EUDI wallet, a real QTSP/RSSP). Our job is to build the DCS correctly and
**prove** it covers the signing requirements against a real remote-signing
counterpart — simplifying only where the simplification is test-only.

## Decision

The DCS is the **OpenID4VP "Document Retrieval" relying party and signature
validator**. It **holds no contract-signing key** and never runs a signature
creation application (SCA). The signatory's key lives in their wallet/QTSP.

1. **Prepare.** The DCS seals the offer into the `odrl:Agreement`, runs the
   closedness/conformance/SHACL and policy gates, embeds the PoA presentation
   and the `ContractSigningSummaryCredential`, and places the AcroForm
   signature field — producing the **to-be-signed** PDF and the canonical
   JSON-LD payload. No signature is applied. (Embed-then-sign, ADR-3, is
   preserved: the wallet's signature covers the already-embedded evidence.)
2. **Publish.** The DCS publishes a **standard OID4VP Document-Retrieval
   request object** (signed JAR, `client_id_scheme=x509_san_dns`) carrying
   `document_digests`, `document_locations`, `response_uri`, and `nonce`, as a
   QR / deep link. `document_digests` is an array: the PDF **and** the JSON-LD
   are offered together, so one ceremony yields both a PAdES and a JAdES over
   the **same content hash** (SM-02, SM-11).
3. **Wallet signs (out of scope).** The wallet fetches the documents, presents
   the required PID/PoA to its QTSP, and drives its own SCA + QTSP to produce
   the signatures. What happens behind the wallet is **invisible to the DCS**.
4. **Receive + validate.** The wallet POSTs the signed documents back to
   `response_uri`. The DCS validates each (DSS `validateSignature`) and applies
   the **sole-control gate** (`dss.Report.AssertValidAES`): the AdES validation
   passed, a signing certificate is present, and that certificate **identifies
   the ceremony's signatory** — a shared or DCS-held key cannot satisfy this.
   It also checks the AdES level (SM-01/-02), the signing time, and credential
   status (SM-18).
5. **Finalize.** The DCS stores the signed PDF, records the signature (PAdES
   hash + JAdES), transitions to SIGNED, and archives — `Applier.finalize`.

## The wallet touch point

The boundary between the DCS (our product) and the wallet/QTSP (ecosystem) is
**exactly** the OID4VP Document-Retrieval interface. Nothing else crosses it.

```
        DCS  (relying party + validator, NO key)   │   Wallet + QTSP + SCA  (signatory, holds the key)
  ────────────────────────────────────────────────┼──────────────────────────────────────────────────
   prepare to-be-signed PDF + JSON-LD              │
   place AcroForm field, embed PoA + summary VC    │
                                                    │
   publish OID4VP request object (signed JAR, QR) ──┼──▶ wallet fetches & parses the request
        client_id_scheme=x509_san_dns              │
        document_digests[] (pdf, json)             │
        document_locations[]  ◀────────────────────┼─── wallet GETs the to-be-signed documents
        response_uri, nonce, state                 │
                                                    │    wallet → QTSP: present PID/PoA (OID4VP),
                                                    │      authorize per-document signature (CSC/rQES)
                                                    │    wallet → SCA (DSS): calculate_hash / assemble
                                                    │    QTSP HSM signs the hash with the signatory's key
   receive signed PDF + JAdES  ◀────────────────────┼─── wallet POSTs signed documents to response_uri
   validate: DSS validateSignature                 │
     + AssertValidAES(signatory)  ← sole control   │
     + credential status + signing time            │
   finalize: store, record, SIGNED, archive        │
```

**Directional invariants (the swap-breakers to never violate):**

- The DCS emits the **standard** OID4VP Document-Retrieval request object, not
  a bespoke JSON shape. This is what makes the test stand-in → real EUDI wallet
  a **configuration swap, not a code change**.
- The DCS hands over a **document** and gets back a **signed document**. It
  never exposes a "sign this hash" API to the wallet, never computes the DTBS,
  never runs the SCA, and never injects or pins the signer certificate — the
  signer certificate flows **from** the wallet/QTSP into the container.
- `client_id` is a DNS-bound X.509 the DCS controls (`x509_san_dns`), so the
  wallet can authenticate the request.

## Test edge (where, and only where, we simplify)

To prove the ceremony without shipping a wallet, a **headless test wallet+QTSP
stand-in** (`testWallet/dcs_wallet/`) plays the counterpart: it consumes the
request, fetches the documents, and produces a **genuinely valid** AES with a
**per-signatory key issued by a test CA** (DSS as the SCA). The DCS treats it
**identically** to a real wallet — same request object, same validation, same
sole-control gate. The simplification is entirely on the far side of the touch
point; the DCS code is production.

Production swap: point the ceremony at a real wallet scheme and a real QTSP/SCA
URL, and trust the QTSP's issuing CA instead of the dev CA. No DCS code change.

## Out of scope (explicitly not built by FACIS.DCS)

- A wallet or signature-creation application (SM-06 is a wallet requirement).
- A QTSP / RSSP or the CSC/rQES credential-authorization service.
- Deploying the EUDI reference QTSP/SCA/verifier stack — the test stand-in
  proves DCS conformance without it.
- QES / qualified certificates (SRS line 199; AES + PoA is the target).

## Consequences

- **Removed:** the DCS-as-signatory path — `signer.ContractSigner` /
  `PDFCoreSigner` / the DSS-as-DCS-signer, the `SIGNER_BACKEND` toggle and the
  PAdES x5chain env/secret, pdf-core `/sign` + the backend `/internal/pades/sign`
  endpoint, and the HSM `dcs-contract-pades` key as a contract signer.
- **Kept (legitimately DCS-signed with its own HSM key, ADR-1):** C2PA claim
  and lifecycle-assertion signatures, the signing-summary VC, OID4VP request
  objects (JAR), and the DCS-to-DCS synchronizer's JAdES transport envelope.
  These are the DCS attesting as itself, not as a contracting party.
- The DCS gains an intermediate `PENDING_SIGNATURE` state between APPROVED and
  SIGNED (the async gap while the wallet signs) and persists the to-be-signed
  document so `document_locations` can serve it and finalize can confirm the
  wallet signed those exact bytes.
- A verifier who trusts a signature trusts the **signatory's** certificate, not
  the DCS's — sole control is provable from the artifact alone.

## SRS coverage

| SRS | Covered by |
| --- | --- |
| SM-01 (SES/AES/QES levels; QES descoped) | AdES level asserted from the DSS report |
| SM-02, SM-11 (PAdES + JAdES, same content hash) | `document_digests[]` — one ceremony, both artifacts |
| SM-03/-04/-05 (identity + PoA validated) | PID/PoA presentation in the ceremony; embedded pre-signature |
| SM-06 (wallet manages credentials + signs) | the wallet is the signatory across the touch point |
| SM-16 (apply via integrated signing service; integrity upon signing) | OID4VP RP + `AssertValidAES` sole-control gate |
| SM-18 (validate container/integrity/timestamp/status) | DSS `validateSignature` + status + signing time |
| SM-08 (signing summary VC in PDF/A-3) | embedded at prepare, inside the signed byte range |
| IR-SI-04, IR-SI-10, IR-CI-08 (remote AES over HTTPS, standard containers) | standard OID4VP + DSS; no key in the DCS |
