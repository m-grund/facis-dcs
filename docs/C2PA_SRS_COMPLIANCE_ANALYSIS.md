# C2PA SRS Compliance Analysis (Open Gaps Only)

Analysis date: 2026-06-01  
SRS reference: [docs/SRS_FACIS_DCS.txt](SRS_FACIS_DCS.txt) §5 (lines 3921–4010)

This document intentionally tracks **only remaining gaps**. Completed requirements are not repeated here.

---

## Remaining Open Gaps

### DCS-OR-C2PA-006 (Partial)

SRS requires verifier checks plus standardized lifecycle banner rendering.

Open items:
1. Enforce legal/trust profile checks (trusted anchors/policy constraints) consistently in verification flow.
2. Standardize banner mapping across all verification UI surfaces, not only selected views.

Notes:
1. In dev mode, private CA chains can validate locally while public checkers report untrusted/mismatch statuses due to trust-context differences.

---

### DCS-OR-C2PA-007 (Gap)

SRS requires organizational anchoring, delegation proof (PoA), and key lifecycle controls.

Open items:
1. Verify signer identity anchoring against organization-level trust evidence.
2. Require PoA credential verification for delegated lifecycle transitions.
3. Enforce key rotation/revocation policy at verification/policy layer.

---

## Canonical Gap List

For the canonical, maintained gap register, see:
[docs/c2pa-srs-compliance-gaps.md](c2pa-srs-compliance-gaps.md)
  ```

**Gaps**: None. ✅ **FULLY IMPLEMENTED**

---

## Summary Table

| Requirement | Status | Code Location | Evidence |
|---|---|---|---|
| **DCS-OR-C2PA-001** | ✅ IMPLEMENTED | [manifest.go](backend/internal/pdfgeneration/c2pa/manifest.go#L96) | BuildManifest creates JUMBF with claim + assertions + COSE signature |
| **DCS-OR-C2PA-002** | ✅ IMPLEMENTED | [embedder.go](backend/internal/pdfgeneration/c2pa/embedder.go#L96) | writeC2PAIncrement uses classic xref (WriteXRefStream=false) |
| **DCS-OR-C2PA-003** | ✅ IMPLEMENTED | [lifecycle.go](backend/internal/pdfgeneration/c2pa/lifecycle.go#L8) | LifecycleAssertion includes all 8 required fields + prev_manifest_hash chain |
| **DCS-OR-C2PA-004** | ✅ IMPLEMENTED | [vc_binding.go](backend/internal/pdfgeneration/c2pa/vc_binding.go#L40) | IssueLifecycleVC issues W3C VC with contract_id, file_hash, status, reason, effective_at |
| **DCS-OR-C2PA-005** | ✅ IMPLEMENTED | [status_list.go](backend/internal/pdfgeneration/c2pa/status_list.go#L38) | OCMWStatusListPublisher publishes status; VC includes credentialStatus |
| **DCS-OR-C2PA-006** | ⚠️ PARTIAL | [verify/verifier.go](backend/internal/pdfgeneration/verify/verifier.go) | C2PA/VC verification works; trust-policy validation missing (UI layer) |
| **DCS-OR-C2PA-007** | ⚠️ PARTIAL | [manifest.go](backend/internal/pdfgeneration/c2pa/manifest.go#L138) | x5chain embedded; org trust anchoring & PoA verification missing |
| **DCS-OR-C2PA-008** | ✅ IMPLEMENTED | [embedder.go](backend/internal/pdfgeneration/c2pa/embedder.go#L74-L80) | Both standalone manifest and updated PDF stored in IPFS |
| **DCS-OR-C2PA-009** | ✅ IMPLEMENTED | [manifest.go](backend/internal/pdfgeneration/c2pa/manifest.go#L174) + [event/](backend/internal/contractworkflowengine/event/event.go#L256) | RFC 3161 timestamps in JUMBF; audit log tracks status changes |
| **DCS-OR-C2PA-010** | ✅ IMPLEMENTED | [embedder.go](backend/internal/pdfgeneration/c2pa/embedder.go#L107-L112) | Classic xref + /Prev chaining + incremental-only append preserves signatures |

---

## Closure Status

**Core PDF Artifact Generation**: 7/10 fully implemented, 2/10 partially implemented (gaps are at policy/trust layer, not artifact layer), 0/10 open gaps.

**For PDF Artifact Compliance Testing** (sign → append → verify), the system is production-ready. The two partial requirements (006, 007) require additional policy enforcement at application/verifier layer, not changes to artifact generation.

---

## Recommendations

1. **For DCS-OR-C2PA-006**: Implement policy-driven trust profile validation in verifier (eIDAS/LPID anchor checks, certificate policy OID enforcement). Standardize banner logic in a single reusable component.

2. **For DCS-OR-C2PA-007**: Implement organizational trust root configuration, PoA credential validation gate on status transitions, and key lifecycle governance policy at C2PA manifest builder layer.

3. **Testing**: Current BDD tests in [features/08_audit_compliance/c2pa_provenance.feature](features/08_audit_compliance/c2pa_provenance.feature) should all pass for requirements 001–005, 008–010. Requirements 006–007 need additional policy test scenarios.

