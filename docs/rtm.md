# Requirements Traceability Matrix

This RTM maps every requirement ID in the SRS (§3.1–§3.5: `DCS-FR-*`,
`DCS-NFR-*`, `DCS-IR-*`) to where it is evidenced in the codebase, generated
semi-automatically per the SRS's own instruction (§6): requirement IDs are
grepped across `backend/`, `pdf-core/`, `features/`, and
`frontend/ClientApp/src/`, and reconciled against the full ID list extracted
from the SRS text.

## Methodology and how to read "Untagged"

**Tagged** means the requirement ID appears verbatim as a comment or
Gherkin tag next to the implementing code or the BDD scenario that exercises
it — a mechanically verifiable claim, not an assertion.

**Untagged** means grep found no such citation. It does **not** mean the
requirement is unimplemented — most of this codebase's business logic
predates the convention of inline requirement-ID comments, and Non-Functional
Requirements in particular (security, performance, scalability qualities)
rarely have a single file:line to point to by nature. Untagged rows are
exactly the rows a manual audit still needs to classify individually; this
RTM does not fabricate that classification. Known gaps among the untagged
rows are already captured in [deviation-register.md](deviation-register.md)
(cross-referenced by category below) — an untagged row with no deviation
register entry should be read as "likely implemented, not yet traced,"
not as "missing."

## Coverage by category

| Category | Tagged | Total | Notes |
|---|---|---|---|
| FR-CWE | 22 | 31 | Contract Workflow Engine — core lifecycle, well covered by feature tags. |
| FR-TR | 6 | 28 | Template Repository — many requirements covered by shared template CRUD code without per-ID tags. |
| FR-SM | 11 | 27 | Signature Management — QES/JAdES/CAdES-related IDs genuinely untagged per Deviation Register #1–#3. |
| FR-CSA | 4 | 26 | Contract Storage & Archive — archive/audit endpoints exist (IR-CSA is 6/6) but individual FR-level behaviors are largely untagged. |
| NFR-SEC | 2 | 18 | Security NFRs are enforced throughout (HSM/PKCS#11, RBAC, revocation checks, TSA/C2PA integrity) but rarely carry an inline ID; recommend a dedicated manual security-NFR audit as follow-up. |
| FR-UC | 3 | 14 | Use-case-level requirements; covered indirectly by the UC-tagged BDD scenarios across `features/`, not all individually ID-tagged. |
| IR-CWE | 13 | 13 | Fully tagged — every contract-workflow interface requirement is cited in `backend/design/contract_workflow_engine.go`. |
| IR-SI | 4 | 12 | System-integration interfaces; ORCE/deployment (IR-SI-02/05) tagged, several external-integration IDs untagged. |
| IR-CI | 0 | 10 | Counterparty-integration interfaces — see Deviation Register #11 (catalogue registration) for the main known gap; the rest need manual audit. |
| NFR-BR | 2 | 9 | Business-rule NFRs; two-instance federation (NFR-BR-08) and deployment SLAs (NFR-BR-03) tagged. |
| IR-SM | 8 | 8 | Fully tagged — `backend/design/signature_management.go`. |
| IR-TR | 8 | 8 | Fully tagged — `backend/design/template_repository.go`. |
| NFR-SQ | 0 | 8 | Scalability/quality NFRs — architectural (stateless services, DB-backed persistence) but untagged; not measured/load-tested in this codebase. |
| FR-PACM | 3 | 7 | Process Audit & Compliance; core audit/report/monitor endpoints exist (IR-PACM is 4/4). |
| IR-CSA | 6 | 6 | Fully tagged — `backend/design/contract_storage_archive.go`. |
| IR-PACM | 4 | 4 | Fully tagged — `backend/design/process_audit_and_compliance.go`. |
| IR-HI | 1 | 3 | HSM interface; IR-HI-01 (PKCS#11) tagged, IR-HI-02/03 (FIDO2/TPM) are Deviation Register #7. |
| NFR-PER | 0 | 3 | Performance NFRs — no load-testing harness in this codebase; untagged, not measured. |
| NFR-SF | 0 | 3 | Safety NFRs — untagged, needs manual audit. |
| NFR-COMP | 0 | 1 | Compliance NFR — untagged, needs manual audit. |

## Endpoint audit (SRS §3.1.1, Workstream E1a)

38 of the 39 endpoints SRS §3.1.1 names per module exist in
`backend/design/`. The sole gap is `POST /archive/terminate` — deliberately
not built, since archiving is preservation rather than a terminal action in
this system's state model (ADR-2) and the endpoint's semantics would
collide with the existing `POST /contract/terminate`.

## Full requirement-ID appendix

| Requirement ID | Status | Evidence |
|---|---|---|
| DCS-FR-CSA-01 | Untagged | — |
| DCS-FR-CSA-02 | Untagged | — |
| DCS-FR-CSA-03 | Untagged | — |
| DCS-FR-CSA-04 | Untagged | — |
| DCS-FR-CSA-05 | Untagged | — |
| DCS-FR-CSA-06 | Tagged | `backend/design/pdf_generation.go` |
| DCS-FR-CSA-07 | Untagged | — |
| DCS-FR-CSA-08 | Untagged | — |
| DCS-FR-CSA-09 | Untagged | — |
| DCS-FR-CSA-10 | Tagged | `backend/internal/semantic/mapper/profile.go` |
| DCS-FR-CSA-11 | Untagged | — |
| DCS-FR-CSA-12 | Untagged | — |
| DCS-FR-CSA-13 | Untagged | — |
| DCS-FR-CSA-14 | Untagged | — |
| DCS-FR-CSA-15 | Untagged | — |
| DCS-FR-CSA-16 | Untagged | — |
| DCS-FR-CSA-17 | Untagged | — |
| DCS-FR-CSA-18 | Tagged | `backend/design/pdf_generation.go`, `backend/internal/bundleexport/bundler.go`, `backend/internal/contractworkflowengine/datatype/eventtype/eventtype.go`, +2 more |
| DCS-FR-CSA-19 | Untagged | — |
| DCS-FR-CSA-20 | Untagged | — |
| DCS-FR-CSA-21 | Untagged | — |
| DCS-FR-CSA-22 | Untagged | — |
| DCS-FR-CSA-23 | Untagged | — |
| DCS-FR-CSA-24 | Untagged | — |
| DCS-FR-CSA-25 | Untagged | — |
| DCS-FR-CSA-26 | Tagged | `features/20_contract_hierarchy_bundle_export/contract_hierarchy_bundle_export.feature` |
| DCS-FR-CWE-01 | Untagged | — |
| DCS-FR-CWE-02 | Tagged | `backend/internal/base/validation/documentdata.go`, `features/20_contract_hierarchy_bundle_export/contract_hierarchy_bundle_export.feature` |
| DCS-FR-CWE-03 | Tagged | `features/03_contract_creation/contract_creation.feature` |
| DCS-FR-CWE-04 | Tagged | `backend/design/pdf_generation.go`, `backend/design/signature_management.go`, `backend/gen/http/cli/dcs/cli.go`, +9 more |
| DCS-FR-CWE-05 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +1 more |
| DCS-FR-CWE-06 | Tagged | `backend/cmd/dcs/main.go`, `backend/internal/contractworkflowengine/deployevent/subscriber.go`, `features/05_contract_deployment/contract_deployment.feature`, +1 more |
| DCS-FR-CWE-07 | Tagged | `features/03_contract_creation/contract_creation.feature`, `features/03_contract_creation/contract_negotiation.feature` |
| DCS-FR-CWE-08 | Tagged | `features/03_contract_creation/contract_negotiation.feature` |
| DCS-FR-CWE-09 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_workflow_engine/service.go`, `backend/gen/http/contract_workflow_engine/client/types.go`, +9 more |
| DCS-FR-CWE-10 | Untagged | — |
| DCS-FR-CWE-11 | Tagged | `features/06_contract_lifecycle/contract_termination.feature` |
| DCS-FR-CWE-12 | Tagged | `features/06_contract_lifecycle/contract_termination.feature` |
| DCS-FR-CWE-13 | Tagged | `features/03_contract_creation/contract_creation.feature`, `features/12_system_based_contract_management/contract_lifecycle_via_api.feature` |
| DCS-FR-CWE-14 | Tagged | `features/03_contract_creation/contract_negotiation.feature` |
| DCS-FR-CWE-15 | Tagged | `features/03_contract_creation/contract_approval.feature` |
| DCS-FR-CWE-16 | Tagged | `features/03_contract_creation/contract_approval.feature` |
| DCS-FR-CWE-17 | Untagged | — |
| DCS-FR-CWE-18 | Tagged | `features/03_contract_creation/contract_negotiation.feature` |
| DCS-FR-CWE-19 | Untagged | — |
| DCS-FR-CWE-20 | Tagged | `backend/internal/contractworkflowengine/command/archive.go`, `backend/internal/signingmanagement/command/apply.go`, `features/05_contract_deployment/contract_deployment.feature` |
| DCS-FR-CWE-21 | Untagged | — |
| DCS-FR-CWE-22 | Tagged | `features/06_contract_lifecycle/contract_termination.feature` |
| DCS-FR-CWE-23 | Untagged | — |
| DCS-FR-CWE-24 | Untagged | — |
| DCS-FR-CWE-25 | Tagged | `features/03_contract_creation/contract_approval.feature` |
| DCS-FR-CWE-26 | Untagged | — |
| DCS-FR-CWE-27 | Untagged | — |
| DCS-FR-CWE-28 | Tagged | `features/12_system_based_contract_management/contract_lifecycle_via_api.feature` |
| DCS-FR-CWE-29 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_workflow_engine/service.go`, `features/20_contract_hierarchy_bundle_export/contract_hierarchy_bundle_export.feature` |
| DCS-FR-CWE-30 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +3 more |
| DCS-FR-CWE-31 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_workflow_engine/service.go`, `backend/gen/http/contract_workflow_engine/client/types.go`, +7 more |
| DCS-FR-PACM-01 | Untagged | — |
| DCS-FR-PACM-02 | Tagged | `features/03_contract_creation/contract_approval.feature` |
| DCS-FR-PACM-03 | Tagged | `backend/internal/base/validation/templateprovenance.go`, `features/03_contract_creation/contract_approval.feature`, `features/18_odrl_soundness/odrl_soundness.feature` |
| DCS-FR-PACM-04 | Untagged | — |
| DCS-FR-PACM-05 | Untagged | — |
| DCS-FR-PACM-06 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +2 more |
| DCS-FR-PACM-07 | Untagged | — |
| DCS-FR-SM-01 | Untagged | — |
| DCS-FR-SM-02 | Untagged | — |
| DCS-FR-SM-03 | Tagged | `features/14_credential_acquisition/poa_credential_verification.feature` |
| DCS-FR-SM-04 | Tagged | `features/14_credential_acquisition/poa_credential_verification.feature` |
| DCS-FR-SM-05 | Untagged | — |
| DCS-FR-SM-06 | Untagged | — |
| DCS-FR-SM-07 | Untagged | — |
| DCS-FR-SM-08 | Tagged | `backend/internal/pdfgeneration/provenance/signing_summary.go`, `features/22_real_signing_vertical/real_signing_vertical.feature`, `pdf-core/compiler/signing_evidence.go`, +1 more |
| DCS-FR-SM-09 | Untagged | — |
| DCS-FR-SM-10 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_storage_archive/service.go`, `backend/gen/contract_workflow_engine/service.go`, +6 more |
| DCS-FR-SM-11 | Untagged | — |
| DCS-FR-SM-12 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_storage_archive/service.go`, `backend/gen/contract_workflow_engine/service.go`, +8 more |
| DCS-FR-SM-13 | Untagged | — |
| DCS-FR-SM-14 | Tagged | `backend/design/signature_management.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/signature_management/service.go`, +3 more |
| DCS-FR-SM-15 | Untagged | — |
| DCS-FR-SM-16 | Tagged | `backend/design/signature_management.go`, `backend/internal/pdfgeneration/query/common.go`, `backend/internal/signingmanagement/command/apply.go`, +2 more |
| DCS-FR-SM-17 | Untagged | — |
| DCS-FR-SM-18 | Tagged | `features/22_real_signing_vertical/real_signing_vertical.feature` |
| DCS-FR-SM-19 | Untagged | — |
| DCS-FR-SM-20 | Tagged | `backend/internal/contractworkflowengine/datatype/eventtype/eventtype.go`, `features/15_access_revocation/signature_revocation_state.feature` |
| DCS-FR-SM-21 | Untagged | — |
| DCS-FR-SM-22 | Untagged | — |
| DCS-FR-SM-23 | Untagged | — |
| DCS-FR-SM-24 | Untagged | — |
| DCS-FR-SM-25 | Tagged | `backend/internal/signingmanagement/command/apply.go`, `features/22_real_signing_vertical/real_signing_vertical.feature` |
| DCS-FR-SM-26 | Untagged | — |
| DCS-FR-SM-27 | Tagged | `backend/design/pdf_generation.go` |
| DCS-FR-TR-01 | Untagged | — |
| DCS-FR-TR-02 | Tagged | `backend/internal/base/validation/documentdata.go`, `features/20_contract_hierarchy_bundle_export/contract_hierarchy_bundle_export.feature` |
| DCS-FR-TR-03 | Untagged | — |
| DCS-FR-TR-04 | Untagged | — |
| DCS-FR-TR-05 | Untagged | — |
| DCS-FR-TR-06 | Untagged | — |
| DCS-FR-TR-07 | Untagged | — |
| DCS-FR-TR-08 | Untagged | — |
| DCS-FR-TR-09 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +1 more |
| DCS-FR-TR-10 | Untagged | — |
| DCS-FR-TR-11 | Tagged | `features/02_template_management/template_identity.feature` |
| DCS-FR-TR-12 | Untagged | — |
| DCS-FR-TR-13 | Untagged | — |
| DCS-FR-TR-14 | Untagged | — |
| DCS-FR-TR-15 | Untagged | — |
| DCS-FR-TR-16 | Untagged | — |
| DCS-FR-TR-17 | Untagged | — |
| DCS-FR-TR-18 | Untagged | — |
| DCS-FR-TR-19 | Tagged | `backend/design/template_repository.go` |
| DCS-FR-TR-20 | Untagged | — |
| DCS-FR-TR-21 | Untagged | — |
| DCS-FR-TR-22 | Untagged | — |
| DCS-FR-TR-23 | Untagged | — |
| DCS-FR-TR-24 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +2 more |
| DCS-FR-TR-25 | Untagged | — |
| DCS-FR-TR-26 | Tagged | `backend/design/pdf_generation.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/pdf_generation/service.go`, +1 more |
| DCS-FR-TR-27 | Untagged | — |
| DCS-FR-TR-28 | Untagged | — |
| DCS-FR-UC-01 | Tagged | `features/01_authentication_authorization/auth_and_access_control.feature`, `features/01_authentication_authorization/authentication_authorization.feature` |
| DCS-FR-UC-02 | Untagged | — |
| DCS-FR-UC-03 | Untagged | — |
| DCS-FR-UC-04 | Untagged | — |
| DCS-FR-UC-05 | Tagged | `features/05_contract_deployment/contract_deployment.feature` |
| DCS-FR-UC-06 | Untagged | — |
| DCS-FR-UC-07 | Untagged | — |
| DCS-FR-UC-08 | Untagged | — |
| DCS-FR-UC-09 | Untagged | — |
| DCS-FR-UC-10 | Untagged | — |
| DCS-FR-UC-11 | Untagged | — |
| DCS-FR-UC-12 | Untagged | — |
| DCS-FR-UC-13 | Tagged | `features/05_contract_deployment/contract_deployment.feature` |
| DCS-FR-UC-14 | Untagged | — |
| DCS-IR-CI-01 | Untagged | — |
| DCS-IR-CI-02 | Untagged | — |
| DCS-IR-CI-03 | Untagged | — |
| DCS-IR-CI-04 | Untagged | — |
| DCS-IR-CI-05 | Untagged | — |
| DCS-IR-CI-06 | Untagged | — |
| DCS-IR-CI-07 | Untagged | — |
| DCS-IR-CI-08 | Untagged | — |
| DCS-IR-CI-09 | Untagged | — |
| DCS-IR-CI-10 | Untagged | — |
| DCS-IR-CSA-01 | Tagged | `backend/design/contract_storage_archive.go`, `features/07_contract_storage_security/archive_management.feature` |
| DCS-IR-CSA-02 | Tagged | `backend/design/contract_storage_archive.go` |
| DCS-IR-CSA-03 | Tagged | `backend/design/contract_storage_archive.go` |
| DCS-IR-CSA-04 | Tagged | `backend/design/contract_storage_archive.go`, `features/07_contract_storage_security/archive_management.feature` |
| DCS-IR-CSA-05 | Tagged | `backend/design/contract_storage_archive.go`, `features/07_contract_storage_security/archive_management.feature` |
| DCS-IR-CSA-06 | Tagged | `backend/design/contract_storage_archive.go` |
| DCS-IR-CWE-01 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-02 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-03 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-04 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-05 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-06 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-07 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-08 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-09 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-10 | Tagged | `backend/design/contract_workflow_engine.go`, `features/03_contract_creation/contract_state_machine_refactor.feature` |
| DCS-IR-CWE-11 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-12 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-CWE-13 | Tagged | `backend/design/contract_workflow_engine.go` |
| DCS-IR-HI-01 | Tagged | `backend/cmd/dcs/main.go`, `backend/design/internal_signing.go`, `backend/gen/http/cli/dcs/cli.go`, +13 more |
| DCS-IR-HI-02 | Untagged | — |
| DCS-IR-HI-03 | Untagged | — |
| DCS-IR-PACM-01 | Tagged | `backend/design/process_audit_and_compliance.go`, `features/08_audit_compliance/process_audit_and_compliance.feature` |
| DCS-IR-PACM-02 | Tagged | `backend/design/process_audit_and_compliance.go`, `features/08_audit_compliance/process_audit_and_compliance.feature` |
| DCS-IR-PACM-03 | Tagged | `backend/design/process_audit_and_compliance.go`, `features/08_audit_compliance/process_audit_and_compliance.feature` |
| DCS-IR-PACM-04 | Tagged | `backend/design/process_audit_and_compliance.go`, `features/08_audit_compliance/process_audit_and_compliance.feature` |
| DCS-IR-SI-01 | Tagged | `backend/design/template_catalogue_integration.go` |
| DCS-IR-SI-02 | Tagged | `features/05_contract_deployment/contract_deployment.feature` |
| DCS-IR-SI-03 | Untagged | — |
| DCS-IR-SI-04 | Untagged | — |
| DCS-IR-SI-05 | Tagged | `backend/design/contract_workflow_engine.go`, `backend/gen/contract_workflow_engine/service.go`, `backend/gen/http/cli/dcs/cli.go`, +2 more |
| DCS-IR-SI-06 | Untagged | — |
| DCS-IR-SI-07 | Untagged | — |
| DCS-IR-SI-08 | Untagged | — |
| DCS-IR-SI-09 | Untagged | — |
| DCS-IR-SI-10 | Tagged | `backend/cmd/dcs/main.go`, `backend/design/internal_signing.go`, `backend/internal/pdfgeneration/provenance/vc_issuer.go`, +3 more |
| DCS-IR-SI-11 | Untagged | — |
| DCS-IR-SI-12 | Untagged | — |
| DCS-IR-SM-01 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-02 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-03 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-04 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-05 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-06 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-07 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-SM-08 | Tagged | `backend/design/signature_management.go` |
| DCS-IR-TR-01 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-02 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-03 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-04 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-05 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-06 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-07 | Tagged | `backend/design/template_repository.go` |
| DCS-IR-TR-08 | Tagged | `backend/design/template_repository.go` |
| DCS-NFR-BR-01 | Untagged | — |
| DCS-NFR-BR-02 | Untagged | — |
| DCS-NFR-BR-03 | Tagged | `features/05_contract_deployment/contract_deployment.feature` |
| DCS-NFR-BR-04 | Untagged | — |
| DCS-NFR-BR-05 | Untagged | — |
| DCS-NFR-BR-06 | Untagged | — |
| DCS-NFR-BR-07 | Untagged | — |
| DCS-NFR-BR-08 | Tagged | `backend/cmd/dcs/main.go`, `features/03_contract_creation/contract_state_machine_refactor.feature`, `features/17_peer_trust/two_instance_peer_trust.feature` |
| DCS-NFR-BR-09 | Untagged | — |
| DCS-NFR-COMP-01 | Untagged | — |
| DCS-NFR-PER-01 | Untagged | — |
| DCS-NFR-PER-02 | Untagged | — |
| DCS-NFR-PER-03 | Untagged | — |
| DCS-NFR-SEC-01 | Untagged | — |
| DCS-NFR-SEC-02 | Tagged | `backend/internal/base/hsm/hsm.go`, `features/21_pki_consolidation_pkcs11/pki_consolidation_pkcs11.feature` |
| DCS-NFR-SEC-03 | Untagged | — |
| DCS-NFR-SEC-04 | Untagged | — |
| DCS-NFR-SEC-05 | Untagged | — |
| DCS-NFR-SEC-06 | Untagged | — |
| DCS-NFR-SEC-07 | Untagged | — |
| DCS-NFR-SEC-08 | Untagged | — |
| DCS-NFR-SEC-09 | Untagged | — |
| DCS-NFR-SEC-10 | Untagged | — |
| DCS-NFR-SEC-11 | Untagged | — |
| DCS-NFR-SEC-12 | Untagged | — |
| DCS-NFR-SEC-13 | Untagged | — |
| DCS-NFR-SEC-14 | Untagged | — |
| DCS-NFR-SEC-15 | Untagged | — |
| DCS-NFR-SEC-16 | Untagged | — |
| DCS-NFR-SEC-17 | Untagged | — |
| DCS-NFR-SEC-18 | Tagged | `backend/design/signature_management.go`, `backend/gen/http/cli/dcs/cli.go`, `backend/gen/signature_management/service.go`, +2 more |
| DCS-NFR-SF-01 | Untagged | — |
| DCS-NFR-SF-02 | Untagged | — |
| DCS-NFR-SF-03 | Untagged | — |
| DCS-NFR-SQ-01 | Untagged | — |
| DCS-NFR-SQ-02 | Untagged | — |
| DCS-NFR-SQ-03 | Untagged | — |
| DCS-NFR-SQ-04 | Untagged | — |
| DCS-NFR-SQ-05 | Untagged | — |
| DCS-NFR-SQ-06 | Untagged | — |
| DCS-NFR-SQ-07 | Untagged | — |
| DCS-NFR-SQ-08 | Untagged | — |