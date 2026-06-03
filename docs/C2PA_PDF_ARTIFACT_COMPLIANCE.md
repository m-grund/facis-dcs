# C2PA PDF Artifact Compliance (Open Gaps Only)

Analysis date: 2026-06-01  
SRS reference: [docs/SRS_FACIS_DCS.txt](SRS_FACIS_DCS.txt) §5 (lines 3921–4010)

This file intentionally tracks only unresolved SRS gaps related to PDF/C2PA compliance posture.

---

## Open Gaps

### DCS-OR-C2PA-006 (Partial)

Remaining gaps:
1. Enforce strict trust-policy checks consistently in verifier policy layer.
2. Guarantee standardized lifecycle banner rendering across all verification-facing UI routes.

### DCS-OR-C2PA-007 (Gap)

Remaining gaps:
1. Organizational anchoring verification for signer identity.
2. PoA delegation proof enforcement for status-changing operations.
3. Key rotation/revocation policy enforcement in verification decisions.

---

## Notes

1. Dev/private CA signatures may validate with local trust configuration but fail in public checkers that do not trust the dev CA.
2. This behavior is expected for private trust roots and should not be interpreted as proof of payload tampering by itself.

---

## Canonical Source

Canonical maintained gap register: [docs/c2pa-srs-compliance-gaps.md](c2pa-srs-compliance-gaps.md)
