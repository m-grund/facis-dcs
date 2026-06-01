# C2PA SRS Remaining Compliance Gaps

Analysis date: 2026-06-01  
SRS reference: [docs/SRS_FACIS_DCS.txt](SRS_FACIS_DCS.txt) §5 "C2PA Content & Life Cycle Credentials for PDF Contracts" (lines 3921–4010)

---

## Remaining Gaps Summary

| Requirement | Status | Gap Focus |
|---|---|---|
| DCS-OR-C2PA-006 | ⚠️ Partial | Deep legal/trust profile enforcement and consistent SRS banner rendering across UI surfaces |
| DCS-OR-C2PA-007 | ❌ Gap | Organization key anchoring, PoA delegation proof, and key rotation/revocation controls |

---

## Gap 1 — DCS-OR-C2PA-006 (Verification Depth + Banner Coverage)

**SRS requirement (lines 3978–3982):**
> The verifier MUST check PDF signatures, C2PA manifests, the VC signature, and the status list. It MUST show a clear banner: Active, Suspended, Terminated, Replaced, Expired, or Draft.

### What is already in place

- Cryptographic COSE signature validation for C2PA manifests is implemented in [backend/internal/pdfgeneration/c2pa/verifier.go](../backend/internal/pdfgeneration/c2pa/verifier.go).
- Detached PDF signature cryptographic verification is implemented in [backend/internal/pdfgeneration/verify/pdf_signature.go](../backend/internal/pdfgeneration/verify/pdf_signature.go).
- Verifier output carries lifecycle/status-list fields in [backend/internal/pdfgeneration/verify/verifier.go](../backend/internal/pdfgeneration/verify/verifier.go).
- Signing dashboard consumes verifier-driven status in [frontend/ClientApp/src/views/signing/SigningDashboardView.vue](../frontend/ClientApp/src/views/signing/SigningDashboardView.vue).

### Remaining gap

1. **Legal-profile depth is not fully enforced**
   - Current checks are cryptographic and structural.
   - Missing strict policy-level validation expected in legal/eIDAS contexts (trusted roots policy, profile constraints, long-term validation evidence, certificate policy OID enforcement).

2. **Banner behavior is not yet uniformly enforced across all verification UX paths**
   - The required banner vocabulary is implemented in the signing dashboard path.
   - Other verification/review surfaces are not yet guaranteed to use the same standardized banner logic.

### Closure criteria

1. Enforce trust-policy/legal-profile validation in verifier flow for PDF signatures and signer trust chain.
2. Standardize and reuse one banner mapping component that derives only from verifier fields (`lifecycle_status`, `status_list_status`) in all verification-facing UI routes.

---

## Gap 2 — DCS-OR-C2PA-007 (Issuer Anchoring + PoA + Key Lifecycle)

**SRS requirement (lines 3984–3989):**
> Issuer keys MUST be anchored to the organization (e.g., org DID with LPID/eIDAS data or Qualified eSeal/QES). Delegation for status changes MUST be proven by a PoA credential. Keys MUST support rotation and revocation.

### Current behavior

- Signing is delegated to Crypto Provider Service (Vault transit), and x5chain is embedded during signing in [backend/internal/pdfgeneration/c2pa/manifest.go](../backend/internal/pdfgeneration/c2pa/manifest.go).
- `authority` is recorded in assertions but not validated against organizational credential anchoring rules.

### Remaining gap

1. **No verified organizational anchoring of signer identity**
   - No enforced binding between issuer DID and LPID/eIDAS/QES-or-eSeal trust evidence.

2. **No PoA delegation verification gate on lifecycle-changing operations**
   - Status transitions are not currently blocked on validated PoA credential chain.

3. **No full key lifecycle controls for compliance posture**
   - Rotation/revocation governance for signing keys is not enforced end-to-end at the C2PA policy layer.

### Closure criteria

1. Validate signer certificate chain against configured organizational trust anchors and policy constraints.
2. Require and verify PoA credentials for delegated lifecycle status transitions before assertion append.
3. Implement key lifecycle policy (rotation metadata, revocation signaling, and verifier rejection behavior for disallowed keys).

---

## Scope Note

This file intentionally tracks **only remaining open gaps**. Items already implemented and verified were removed from this document per request.

## Trust Context Note

Public C2PA checkers and local verification tooling may report different outcomes when private development trust anchors are used.

1. A private/dev CA chain can validate locally when the CA is configured as trust anchor.
2. The same artifact can be reported as untrusted or signature-mismatch by public checkers that do not trust that CA.

This trust-context difference is expected and does not by itself prove payload tampering.
