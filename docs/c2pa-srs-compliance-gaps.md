# C2PA SRS Compliance Gaps

Analysis date: 2026-05-29  
SRS reference: [docs/SRS_FACIS_DCS.txt](SRS_FACIS_DCS.txt) §5 "C2PA Content & Life Cycle Credentials for PDF Contracts" (lines 3921–4010)

---

## Summary

The backend C2PA implementation covers the core provenance and VC-binding requirements structurally, but has four concrete compliance gaps versus the SRS and one conformance risk.

---

## Gap 1 — DCS-OR-C2PA-005: Status-list revocation silently skipped for real workflow states

**SRS requirement (line 3972–3976):**
> The system MUST publish current contract status in a verifiable list. It MUST support real-time suspension and termination.

**Root cause:**  
[`status_list.go`](../backend/internal/pdfgeneration/c2pa/status_list.go) matches lowercase state strings in its `switch` at line 137:

```go
case "terminated", "expired", "replaced", "suspended":
    setRevoked(...)
```

The CWE state machine stores and emits **uppercase** values (`TERMINATED`, `EXPIRED`, …) as defined in [`contractstate.go`](../backend/internal/contractworkflowengine/datatype/contractstate/contractstate.go) lines 13–20. These uppercase strings are passed unmodified into `PublishStatus` via [`subscriber.go`](../backend/internal/pdfgeneration/event/subscriber.go) line 161 and [`pdf_generation.go`](../backend/internal/service/pdf_generation.go) line 309.

**Effect:**  
No revocation bit is ever set in the XFSC status list for termination/suspension events, violating the MUST in DCS-OR-C2PA-005.

**Fix:**  
Lower-case the status string before the switch, or add the uppercase variants to the case arms:

```go
switch strings.ToLower(status) {
case "terminated", "expired", "replaced", "suspended":
```

---

## Gap 2 — DCS-OR-C2PA-006: Verifier does not check PDF signatures, C2PA cryptographic validity, VC signature, or status list

**SRS requirement (line 3978–3982):**
> The verifier MUST check PDF signatures, C2PA manifests, the VC signature, and the status list. It MUST show a clear banner: Active, Suspended, Terminated, Replaced, Expired, or Draft.

**Current behaviour:**  
[`verifier.go`](../backend/internal/pdfgeneration/verify/verifier.go) performs only MR/HR content-hash consistency (JSON-LD extraction + re-render comparison) and remote IPFS fallback for stripped manifests. There is no:

- PDF digital signature validation (PAdES/CAdES)
- C2PA COSE_Sign1 signature verification
- VC `Ed25519Signature2020` proof verification
- Status-list query to determine current revocation state
- Banner/status field in the `PDFVerifyResult` response

The API response type in [`design/pdf_generation.go`](../backend/design/pdf_generation.go) lines 11–15 exposes only `match`, `jsonld_hash`, `base_pdf_hash`, `stored_base_pdf_hash`.

**Effect:**  
A caller using the verify endpoint has no way to learn whether the C2PA provenance chain is cryptographically valid, whether the signer is trusted, or whether the contract has been revoked — all of which are required by DCS-OR-C2PA-006.

**Recommended additions to `PDFVerifyResult`:**

| Field | Description |
|---|---|
| `c2pa_signature_valid` | COSE_Sign1 signature checked against x5chain |
| `vc_signature_valid` | Ed25519 proof on embedded lifecycle VC verified |
| `status_list_status` | Current status from XFSC status list (`active`, `suspended`, `terminated`, …) |
| `lifecycle_status` | Status field from latest lifecycle assertion in the manifest |
| `manifest_source` | Already present (`embedded` / `remote` / `none`) |

---

## Gap 3 — DCS-OR-C2PA-008: "Remote manifest" requirement is satisfied by full-PDF IPFS storage, not a distinct manifest object

**SRS requirement (line 3991–3995):**
> A remote C2PA manifest MUST exist for every contract. The verifier MUST fetch it if the embedded manifest is missing or stripped.

**Current behaviour:**  
The implementation stores the entire updated PDF (base layer + C2PA incremental update appended) in IPFS and records the CID in `pdf_ipfs_cid`. When the verifier receives a stripped PDF it fetches the full IPFS copy via [`pdf_generation.go`](../backend/internal/service/pdf_generation.go) lines 253–287.

**Gap:**  
The SRS wording, combined with the C2PA spec's concept of a "remote manifest store", implies the manifest itself (the JUMBF bytes) should be separately addressable — not bundled inside the full PDF. The current approach works in practice but:

1. A verifier that does not hold the IPFS CID mapping (e.g. an external auditor) cannot discover the remote manifest without DCS-specific knowledge.
2. The IPFS CID is not recorded inside the C2PA manifest, so there is no self-contained resolution path within the file.

**Recommended improvement:**  
Store the JUMBF manifest bytes separately in IPFS in addition to the full PDF, and embed the IPFS URI as a remote-manifest reference in the C2PA `c2pa.manifest` box's `remote-manifest` assertion or in a custom assertion field (`ipfs_manifest_cid`).

---

## Gap 4 — DCS-OR-C2PA-003: Lifecycle state vocabulary is wider than SRS-defined states

**SRS requirement (line 3958–3961):**
> The system MUST model lifecycle states as C2PA assertions: **draft, active, amended, suspended, terminated, expired, replaced**.

**Current behaviour:**  
The CWE exposes additional states not in the SRS-defined set: `NEGOTIATION`, `SUBMITTED`, `REVIEWED`, `REJECTED` (see [`contractstate.go`](../backend/internal/contractworkflowengine/datatype/contractstate/contractstate.go) lines 13–20). C2PA assertions are emitted for all of them via the event subscriber.

**Effect:**  
External verifiers and schema validators that implement DCS-OR-C2PA-003 strictly will reject or misclassify these extra states. The SRS measurement criterion "Coverage of all states … in test assets (target 100%)" is not falsifiable while extra states are emitted without a defined mapping.

**Recommended resolution:**  
Define a canonical mapping from internal CWE states to SRS C2PA states (e.g. `SUBMITTED → draft`, `REVIEWED → draft`, `APPROVED → active`, `REJECTED → draft`) and apply it before constructing `LifecycleAssertion.Status`. Document the mapping here and in the lifecycle assertion schema.

---

## Risk — DCS-OR-C2PA-002/010: `/Catalog/AF` grows as an array across incremental updates

**SRS requirement (lines 3942–3956):**
> MUST embed a C2PA manifest … MUST use PDF incremental updates so existing legal signatures remain valid.

**Observation:**  
[`embedder.go`](../backend/internal/pdfgeneration/c2pa/embedder.go) lines 180–190 append each new manifest FileSpec to the existing `/Catalog/AF` array. The C2PA PDF binding specification (§8.2) requires `/Catalog/AF` to reference **only the active (latest) manifest store**; historical manifests should be discoverable via the `/Names/EmbeddedFiles` name tree but not listed in `/AF`.

Tests in [`embedder_test.go`](../backend/internal/pdfgeneration/c2pa/embedder_test.go) line 236 assert `AF` contains an array of refs, which confirms the current accumulating behaviour.

**Risk:**  
`c2patool` and Acrobat's C2PA verification plugin may select the wrong manifest (the first rather than the last) or report "multiple active manifests" errors, failing the SRS verification method "Tool-based C2PA validation" (line 3940).

**Recommended fix:**  
Replace the full `/Catalog/AF` on each increment so it points only to the latest manifest FileSpec. Historical FileSpecs remain discoverable via `/Names/EmbeddedFiles`.

---

## Conformance items already met

| Requirement | Implementation |
|---|---|
| DCS-OR-C2PA-001 — C2PA manifest on every PDF | NATS subscriber appends manifest on every CWE state-change event |
| DCS-OR-C2PA-002 — Incremental PDF update | `writeC2PAIncrement` appends well-formed xref increment; base layer bytes unchanged |
| DCS-OR-C2PA-003 — Required assertion fields | `LifecycleAssertion` struct covers `contract_id`, `file_hash`, `status`, `reason`, `effective_at`, `authority`, `vc_id`, `prev_manifest_hash` |
| DCS-OR-C2PA-003 — Manifest chaining | `PrevManifestHashFrom` reads active manifest payload; mismatch returns hard error |
| DCS-OR-C2PA-004 — W3C VC issuance | `LocalVCIssuer` issues signed `ContractLifecycleCredential` with `contract_id`, `file_hash`, `status`, `reason`, `effective_at` |
| DCS-OR-C2PA-004 — VC embedded in PDF | VC bytes stored as `contract-lifecycle-vc.json` EmbeddedFile alongside JUMBF |
| DCS-OR-C2PA-007 — No private keys in DCS | All signing delegated to Crypto Provider Service (Vault transit); see [`cryptoprovider/client.go`](../backend/internal/cryptoprovider/client.go) |
| DCS-OR-C2PA-008 — Remote fetch fallback | Verifier calls `FetchFn` (IPFS) when no incremental updates present |
| DCS-OR-C2PA-009 — RFC 3161 timestamp | `TSAConfig.URL` triggers `requestTimestamp`; token stored in COSE unprotected header |
| DCS-OR-C2PA-010 — Legal signatures preserved | Incremental update writes only new objects + xref delta; DocMDP path uses FileAttachment annotation instead of touching `/Names/EmbeddedFiles` |
