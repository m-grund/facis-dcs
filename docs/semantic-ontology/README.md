# FACIS DCS Semantic Ontology Architecture

Status: production-near PoC profile  
Canonical runtime format: JSON-LD  
Interoperability format: RDF/RDFS/OWL Turtle  
Validation format: JSON Schema now, SHACL later/externalized  
Primary integration points: Template Repository, Contract Workflow Engine, Signature Management, Contract Storage and Archive, Process Audit and Compliance, Node-RED/XFSC orchestration

## SRS Alignment

This profile was derived from `docs/SRS_FACIS_DCS.txt`, especially these requirement clusters:

| SRS area | Requirement impact |
| --- | --- |
| 2.2.1 / DCS-FR-TR-03 | Semantic Hub stores versioned JSON-LD contexts and SHACL shapes. |
| DCS-FR-TR-04/05/08/09 | Templates keep machine/human links, versioning, and VC-verifiable provenance. |
| DCS-FR-TR-12/23/24/25/26 | Placeholders, structural dependency mapping, and exportable assembled structures are modeled. |
| 2.2.2 / DCS-FR-CWE-03/04/06/08/09 | Contract assembly, MR/HR synchronization, lifecycle events, versioning, and SLA monitoring are first-class. |
| DCS-FR-CWE-18 and Contract Adjustment | Clause-level adjustments create new contract versions under the same DID/UUID. |
| DCS-FR-SM-03/04/05/10/11/12 | Identity, PoA credentials, proof of execution, signature linkage, and deployment trigger are represented. |
| DCS-FR-CSA-01/03/06/07/10 | Tamper-evident storage, proof-of-existence, machine-readable storage, compliance checks, and metadata indexes are supported. |
| UC-10/11/12/13 | Node-RED orchestration, API/system integration, system-based contract management, and target deployment consume the same JSON-LD profile. |
| DCS-OR-C2PA-001..010 | C2PA lifecycle assertions and status VC binding are included without changing legal signature payloads. |

## 1. Ontologie-Architektur

The FACIS DCS ontology is a pragmatic semantic profile for machine-readable contracts. It does not replace the current DCS data model. It gives existing payloads a stable semantic layer:

```text
ContractTemplate.template_data JSONB
  documentOutline
  documentBlocks
  semanticConditions
  customMetaData
  subTemplateSnapshots
  templateDataVersion
       |
       | JSON-LD context + semantic profile
       v
Contract.contract_data JSONB
  documentOutline
  documentBlocks
  semanticConditions
  semanticConditionValues
  subTemplateSnapshots
  templateDataVersion
  semanticProfile
  lifecycle
  parties
  sla
  deployment
  provenance
```

The architecture follows the SRS:

| SRS capability | Ontology support |
| --- | --- |
| Machine-readable and human-readable template linking | `hasMachineReadableArtifact`, `hasHumanReadableArtifact`, `contentHash` |
| Semantic Hub for schemas | versioned JSON-LD contexts, Turtle ontology, SHACL shapes |
| Template placeholders and SLA rules | `TemplateVariable`, `Parameter`, `PlaceholderBinding`, `SLAAgreement`, `SLO`, `SLI` |
| Contract adjustment | `ContractAdjustment` with clause-level operations and version links |
| Workflow lifecycle | `ContractLifecycleState`, `WorkflowHook`, `Approval`, `Negotiation`, `Deployment` |
| Identity and PoA credentials | `Party`, `Signatory`, `CredentialReference`, `DIDReference`, `UUIDReference` |
| Deployment to target systems | `Deployment`, `DeploymentTarget`, `DeploymentReceipt`, `policyBundle` |
| Compliance and audit | `SemanticRule`, `ValidationReport`, `ProvenanceEvent`, `PolicyDecision` |
| C2PA lifecycle assertions | `c2paManifest`, `statusCredential`, `fileHash`, `previousManifestHash` |

Runtime rule: JSON-LD is the source of record for semantic payloads. RDF export is generated for interoperability and SHACL validation. No OWL inference is required at runtime.

Architecture diagram descriptions:

```text
Diagram A - Runtime data flow
Template Repository -> Contract Workflow Engine -> Signature Management
  -> Contract Storage and Archive -> Target System
Each step carries the same JSON-LD contract envelope and appends validation,
signature, deployment, or provenance evidence.

Diagram B - Semantic validation boundary
Frontend fast checks -> Backend runtime validator -> optional Semantic Hub SHACL
  -> PACM validation report -> archive evidence hash.
The workflow never depends on OWL inference; SHACL is an external gate.

Diagram C - Policy deployment boundary
Signed contract JSON-LD -> policy bundle extractor -> Node-RED deployment flow
  -> API gateway/ERP/target system -> deployment receipt -> PACM/CSA.
```

## 2. Modul-Struktur

```text
docs/semantic-ontology/
  README.md
  ontology/
    facis-dcs-ontology.ttl
  shapes/
    facis-dcs-shapes.ttl
  contexts/
    facis-dcs-context.jsonld
  examples/
    sla-template.jsonld
    machine-readable-contract.jsonld
  types/
    facis-dcs-semantic.ts
```

Recommended future implementation modules:

```text
backend/internal/semantic/
  context/              JSON-LD context loading and version registry
  model/                Go structs mirroring TypeScript interfaces
  mapper/               template_data <-> semanticProfile mapping
  validation/           fast runtime validators and SHACL adapter boundary
  policy/               policy bundle extraction for target systems
  event/                semantic CloudEvents/NATS event builders
```

## 3. Design-Rationale

The profile is intentionally small:

| Decision | Rationale |
| --- | --- |
| JSON-LD-first | Existing APIs already expose JSON payloads. JSON-LD adds semantics without changing Goa endpoints. |
| Additive profile | `semanticConditions`, `templateDataJSON`, `contract_data`, and `change_request` remain compatible. |
| Shallow RDFS/OWL | Classes and properties document meaning. Runtime validation uses deterministic checks. |
| SHACL-ready | Shapes are separate and can run in Semantic Hub/PACM or CI, not inside critical workflow paths. |
| Clause-level versioning | Contract adjustments can target `Clause`/`documentBlocks.blockId` without regenerating the whole contract. |
| Workflow-aware | Lifecycle, approvals, negotiation, signing, deployment, archive, and revocation are first-class events. |
| Policy-ready | Rules can be exported as ODRL-like constraints or target-specific policy bundles. |

## 4. OWL/RDFS Turtle Ontologie

The ontology is in [facis-dcs-ontology.ttl](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/ontology/facis-dcs-ontology.ttl). It declares the required classes:

Core: `Contract`, `ContractTemplate`, `ContractObject`, `ContractVersion`, `Clause`, `Section`, `ContractCondition`, `SemanticCondition`.

SLA: `SLAAgreement`, `Service`, `SLO`, `SLI`, `MeasurementMetric`, `MeasurementRule`, `Remedy`, `ServiceCredit`, `ClaimPolicy`, `ExclusionEvent`.

Workflow: `ContractLifecycleState`, `ContractAdjustment`, `Approval`, `Negotiation`, `Deployment`.

Identity: `Party`, `Signatory`, `CredentialReference`, `DIDReference`, `UUIDReference`.

Template: `TemplateVariable`, `Parameter`, `ParameterConstraint`, `PlaceholderBinding`.

Rules: `SemanticRule`, `Constraint`, `Operator`, `ThresholdRule`, `DateConstraintRule`.

## 5. JSON-LD Context

The runtime context is in [facis-dcs-context.jsonld](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/contexts/facis-dcs-context.jsonld). The profile uses compact DCS field names already close to the current frontend/backend model:

```json
{
  "@context": "https://w3id.org/facis/dcs/context/v1",
  "@type": "Contract",
  "did": "did:web:dcs.example:contract:123",
  "contractData": {
    "semanticConditions": [],
    "semanticConditionValues": []
  }
}
```

The concrete PoC may serve this context statically from the backend, e.g. `GET /semantic/context/v1`.

## 6. semanticConditions Mapping

Current DCS payload:

```json
{
  "conditionId": "sc-uptime",
  "conditionName": "Uptime SLA",
  "schemaVersion": "v1",
  "parameters": [
    {
      "parameterName": "uptime",
      "type": "decimal",
      "isRequired": true,
      "operators": [
        { "operate": "greaterThanOrEqual", "targets": ["99.95"] }
      ],
      "value": null
    }
  ]
}
```

Semantic mapping:

| Existing field | Semantic class/property | Notes |
| --- | --- | --- |
| `conditionId` | `dcs:conditionId`, `@id` suffix | Stable local ID, e.g. `urn:uuid:...#sc-uptime`. |
| `conditionName` | `dcs:name` | Human-readable label. |
| `schemaVersion` | `dcs:schemaVersion` | Keep `v1`; add `semanticProfile` at envelope level. |
| `parameters[]` | `dcs:Parameter` | Runtime parameter definition. |
| `parameterName` | `dcs:parameterName` | Also used by placeholders. |
| `type` | `dcs:parameterType` | `string`, `decimal`, `integer`, `boolean`, `date`, `enum`. |
| `isRequired` | `dcs:required` | Boolean validation flag. |
| `operators[]` | `dcs:Constraint` / `dcs:Operator` | Existing values are normalized to ontology operators. |
| `targets[]` | `dcs:rightOperand` | Literal or placeholder reference, e.g. `{{contractEndDate}}`. |
| `semanticConditionValues[]` | `dcs:ParameterValue` | Contract instance values keyed by `blockId`, `conditionId`, `parameterName`. |
| `documentBlocks[].conditionIds` | `dcs:appliesToClause` | Links a semantic rule to a clause. |

Operator compatibility:

| Current operator | Ontology operator |
| --- | --- |
| `equal` | `Equals` |
| `notEqual` | `NotEquals` |
| `greaterThan` | `GreaterThan` |
| `greaterThanOrEqual` | `GreaterThanOrEqual` |
| `lessThan` | `LessThan` |
| `lessThanOrEqual` | `LessThanOrEqual` |
| new | `Between`, `Contains`, `MatchesRegex` |

## 7. Beispiel-Regeln

The rule engine should evaluate the profile deterministically, without semantic reasoning:

```json
[
  {
    "@type": "ThresholdRule",
    "ruleId": "rule-uptime-minimum",
    "leftOperand": "$.sla.slos[?(@.identifier=='slo-uptime-monthly')].targetValue",
    "operator": "GreaterThanOrEqual",
    "rightOperand": 99.95,
    "valueType": "decimal"
  },
  {
    "@type": "DateConstraintRule",
    "ruleId": "rule-expiry-in-future",
    "leftOperand": "$.validUntil",
    "operator": "GreaterThan",
    "rightOperand": "now",
    "valueType": "date"
  },
  {
    "@type": "SemanticRule",
    "ruleId": "rule-organization-country-de",
    "leftOperand": "$.parties[?(@.role=='customer')].country",
    "operator": "Equals",
    "rightOperand": "DE",
    "valueType": "string"
  },
  {
    "@type": "DateConstraintRule",
    "ruleId": "rule-access-before-contract-end",
    "leftOperand": "$.access.accessUntil",
    "operator": "LessThan",
    "rightOperand": "$.validUntil",
    "valueType": "date"
  }
]
```

Evaluation contract:

| Input | Meaning |
| --- | --- |
| `leftOperand` | JSONPath over the expanded contract profile or parameter value map. |
| `operator` | Closed enum. |
| `rightOperand` | Literal, `now`, JSONPath, or `{{conditionId.parameterName}}`. |
| `valueType` | Parser hint for dates/numbers/strings/booleans. |
| `severity` | `info`, `warning`, `error`, `blocking`. |

## 8. Beispiel-SLA-Template

See [sla-template.jsonld](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/examples/sla-template.jsonld). The example models:

| Template element | Representation |
| --- | --- |
| API service | `Service` |
| Availability objective | `SLO` with `SLI` and `MeasurementMetric` |
| Measurement window | `MeasurementRule` |
| Service credit | `Remedy` and `ServiceCredit` |
| Customer claim rules | `ClaimPolicy` |
| Force majeure / maintenance | `ExclusionEvent` |
| Placeholders | `TemplateVariable` and `PlaceholderBinding` |

## 9. Beispiel-Machine-readable-Contract

See [machine-readable-contract.jsonld](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/examples/machine-readable-contract.jsonld). The instance shows:

| Concern | Field |
| --- | --- |
| Current DB compatibility | `did`, `contractVersion`, `state`, `contractData` |
| Template traceability | `derivedFromTemplate`, `templateVersion` |
| Clause-level versioning | `contractVersions[].changedClauses[]`, `clauses[].version` |
| Semantic conditions | `contractData.semanticConditions` and `semanticConditionValues` |
| SLA monitoring | `sla.slos`, `measurementRules` |
| Policy deployment | `deployment.policyBundle` |
| Identity | `parties`, `signatories`, `credentialReferences` |
| Provenance/C2PA | `provenance`, `c2paManifest`, `statusCredential` |

## 10. SHACL-Strategie

SHACL is optional and externalized:

| Phase | Validation mechanism |
| --- | --- |
| Template authoring | Fast frontend checks, TypeScript types, and JSON Schema-like validation. |
| Template verification | Semantic Hub runs JSON-LD parsing plus SHACL shapes. |
| Contract creation | Backend validates required parameters, placeholder bindings, and rule syntax. |
| Review/approval | PACM can run SHACL and policy checks and return findings. |
| Pre-deployment | Blocking validation before deployment payload is sent. |
| Archive | Persist validation report hash and evidence. |

The starter shapes are in [facis-dcs-shapes.ttl](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/shapes/facis-dcs-shapes.ttl).

## 11. Repository-Struktur

Immediate PoC files are committed under `docs/semantic-ontology`. Recommended next code changes:

```text
backend/design/semantic.go
  Goa types for SemanticValidationRequest, SemanticValidationResponse, SemanticContextResponse

backend/internal/semantic/model
  Go structs for semantic profile

backend/internal/semantic/mapper
  conversion from current template_data/contract_data to semantic profile

frontend/ClientApp/src/models/semantic
  generated or copied TypeScript interfaces

deployment/node-red/src/engine
  semantic validation and deployment node descriptors
```

## 12. TypeScript Interfaces

See [facis-dcs-semantic.ts](/c:/Work/Projects/facis_dcs/facis-dcs/docs/semantic-ontology/types/facis-dcs-semantic.ts). These types are designed to extend the existing frontend interfaces:

```ts
type ContractData = ExistingContractData & SemanticContractDataExtension
type ContractTemplateData = ExistingTemplateData & SemanticTemplateDataExtension
```

## 13. REST/OpenAPI Integrationsstrategie

No breaking endpoint changes are required. Additive integration:

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/semantic/context/v1` | GET | Serve JSON-LD context. |
| `/semantic/ontology/v1` | GET | Serve Turtle ontology. |
| `/semantic/shapes/v1` | GET | Serve current SHACL shapes. |
| `/template/verify/{template_id}` | existing | Include `semanticFindings` and `profileVersion`. |
| `/contract-workflow-engine/create` | existing | Initialize `contract_data.semanticProfile`. |
| `/contract-workflow-engine/update` | existing | Validate `semanticConditionValues` and clause bindings. |
| `/contract-workflow-engine/review` | existing | Run missing values/rule syntax checks. |
| `/contract-workflow-engine/approve` | existing | Run blocking semantic validation. |
| `/deployment/contracts/{did}` | future | Export deployment policy bundle. |

OpenAPI schema strategy:

```yaml
SemanticValidationFinding:
  type: object
  required: [ruleId, severity, message]
  properties:
    ruleId: { type: string }
    severity: { type: string, enum: [info, warning, error, blocking] }
    path: { type: string }
    message: { type: string }
    source: { type: string, enum: [runtime, shacl, policy, credential] }
```

## 14. Node-RED Orchestrierungsstrategie

Node-RED/XFSC should consume and emit compact JSON-LD. Recommended nodes:

| Node | Input | Output |
| --- | --- | --- |
| `dcs-template-resolve` | `templateDid`, `version` | approved template payload |
| `dcs-contract-assemble` | template plus parameter values | draft contract JSON-LD |
| `dcs-semantic-validate` | contract JSON-LD | validation report |
| `dcs-credential-verify` | parties/signatories | credential status report |
| `dcs-contract-approve` | did, actor, decision | lifecycle event |
| `dcs-contract-sign` | did, signatory | signature task/status |
| `dcs-contract-deploy` | signed contract JSON-LD | target receipt |
| `dcs-contract-archive` | artifact references | archive receipt |

CloudEvent/NATS subject naming:

```text
dcs.contract.created
dcs.contract.semantic.validated
dcs.contract.approved
dcs.contract.signed
dcs.contract.deployment.requested
dcs.contract.deployment.acknowledged
dcs.contract.lifecycle.changed
dcs.contract.credential.revoked
```

Each event should include `correlationId`, `causationId`, `contractDid`, `contractVersion`, `semanticProfileVersion`, and `actor`.

## 15. Contract Lifecycle Mapping

The current database enum can stay as the storage state. The semantic lifecycle adds derived/interoperable states required by the SRS.

| Current DCS state | Semantic lifecycle state | Notes |
| --- | --- | --- |
| `DRAFT` | `Draft` / `Offered` | Contract is generated from template and editable. |
| `NEGOTIATION` | `InNegotiation` | Contract adjustments can create new versions. |
| `SUBMITTED` | `SubmittedForReview` | Review/approval routing active. |
| `REVIEWED` | `Reviewed` | Review completed, not final approval. |
| `APPROVED` | `Approved` / `ReadyForSignature` | Content locked for signing. |
| current signing subsystem | `Signed` / `Executed` | May be derived from signature records. |
| current deployment subsystem | `Deployed` / `Active` | May be derived from deployment receipt. |
| `TERMINATED` | `Terminated` | No active workflow except archive/compliance. |
| `EXPIRED` | `Expired` | Also derived by `contracts_effective`. |
| no current enum | `Suspended`, `Revoked`, `Archived`, `Replaced` | Add as semantic overlay first, DB enum later if needed. |

## 16. Contract Adjustment Mapping

Current `contract_negotiations.change_request` can carry this profile:

```json
{
  "@type": "ContractAdjustment",
  "adjustmentId": "urn:uuid:7fbcc641-bad7-47b8-8c6c-1da627f78e50",
  "contractDid": "did:web:dcs.example:contract:showtimes-2026",
  "baseVersion": 3,
  "operations": [
    {
      "operation": "replace",
      "targetType": "Clause",
      "targetId": "clause-access-window",
      "path": "$.documentBlocks[?(@.blockId=='clause-access-window')].text",
      "oldHash": "sha256:...",
      "newHash": "sha256:..."
    }
  ],
  "semanticImpact": {
    "conditionIds": ["sc-access-until-before-contract-end"],
    "requiresRevalidation": true
  }
}
```

Accepted adjustment behavior:

| Step | Result |
| --- | --- |
| Propose | Store adjustment in `contract_negotiations.change_request`. |
| Review | Validate target `blockId`, parameter refs, and rule syntax. |
| Accept | Increment `contract_version`; update only targeted clause/values. |
| Re-render | Human-readable view gets same version and content hash. |
| Audit | Emit `dcs.contract.adjustment.accepted`. |

## 17. Provenance Tracking Modell

Minimal provenance event:

```json
{
  "@type": "ProvenanceEvent",
  "eventId": "urn:uuid:...",
  "eventType": "contract.approved",
  "actor": "did:web:participant.example:alice",
  "actorRole": "ContractApprover",
  "credentialRef": "urn:uuid:credential-proof",
  "occurredAt": "2026-05-21T12:30:00Z",
  "entity": "did:web:dcs.example:contract:showtimes-2026",
  "entityVersion": 4,
  "contentHash": "sha256:...",
  "previousEventHash": "sha256:..."
}
```

Provenance is copied into:

| Target | Data |
| --- | --- |
| PACM audit log | full event |
| contract JSON-LD | compact current/provenance references |
| archive | validation report hash, artifact hashes |
| C2PA | lifecycle assertion, file hash, status credential link |
| VC | contract status credential |

## 18. Deployment-Modell

Deployment is a first-class semantic object:

```json
{
  "@type": "Deployment",
  "deploymentId": "urn:uuid:...",
  "targetSystem": "api-gateway-cultural-data",
  "targetEndpoint": "https://gateway.example/policies",
  "contractDid": "did:web:dcs.example:contract:showtimes-2026",
  "contractVersion": 4,
  "policyBundle": {
    "format": "odrl-jsonld",
    "rules": ["rule-org-country-de", "rule-access-before-contract-end"]
  },
  "status": "Acknowledged",
  "receipt": {
    "receiptId": "urn:uuid:...",
    "receivedAt": "2026-05-21T12:45:00Z",
    "targetHash": "sha256:..."
  }
}
```

For the PoC, deployment should export a stable bundle with:

| Field | Purpose |
| --- | --- |
| `contractDid` / `contractVersion` | Traceability. |
| `rules` | Target-enforceable constraints. |
| `parties` / `service` | Assignee/assigner/asset mapping. |
| `validFrom` / `validUntil` | Runtime access window. |
| `credentialRequirements` | Credential status and PoA enforcement. |
| `hashes` | Tamper evidence. |

## 19. Policy Enforcement Vorbereitung

The ontology prepares but does not implement a full policy engine:

| Policy concern | Source field |
| --- | --- |
| Access control for DCS users | role credentials, `CredentialReference`, RBAC claims |
| DCS-to-DCS data exchange | `PolicyBundle`, selected contract metadata endpoint |
| Target-system runtime enforcement | deployment bundle rules |
| Credential revocation | `credentialReferences.status`, `statusListRef` |
| SLA violation handling | `SLO`, `MeasurementRule`, `Remedy`, `ClaimPolicy` |

Rule export mapping to ODRL-style constraints:

| DCS operator | ODRL-like operator |
| --- | --- |
| `Equals` | `odrl:eq` |
| `NotEquals` | `odrl:neq` |
| `GreaterThan` | `odrl:gt` |
| `GreaterThanOrEqual` | `odrl:gteq` |
| `LessThan` | `odrl:lt` |
| `LessThanOrEqual` | `odrl:lteq` |
| `Between` | two constraints: `gteq` and `lteq` |
| `Contains` | target-specific function |
| `MatchesRegex` | target-specific function |

## 20. PostgreSQL JSONB Mapping

The current tables already support the PoC:

| Table/column | Mapping |
| --- | --- |
| `contract_templates.template_data` / `templateDataJSON` | template JSON-LD profile, semantic conditions, placeholders |
| `contracts.contract_data` | contract JSON-LD profile plus values, SLA, deployment, provenance |
| `contract_negotiations.change_request` | `ContractAdjustment` payload |
| audit tables/events | `ProvenanceEvent` and validation findings |
| outbox events | semantic lifecycle/deployment events |

Recommended JSONB indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_contracts_contract_data_gin
  ON contracts USING GIN (contract_data jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_contracts_semantic_profile
  ON contracts ((contract_data #>> '{semanticProfile,version}'));

CREATE INDEX IF NOT EXISTS idx_contracts_valid_until
  ON contracts ((contract_data #>> '{validUntil}'));

CREATE INDEX IF NOT EXISTS idx_contracts_party_country
  ON contracts USING GIN ((contract_data -> 'parties'));

CREATE INDEX IF NOT EXISTS idx_contracts_sla
  ON contracts USING GIN ((contract_data -> 'sla'));
```

Recommended query patterns:

```sql
-- contracts with a blocking semantic finding
SELECT did, contract_version
FROM contracts
WHERE contract_data @? '$.validationReports[*].findings[*] ? (@.severity == "blocking")';

-- contracts whose customer country is DE
SELECT did
FROM contracts
WHERE contract_data @? '$.parties[*] ? (@.role == "customer" && @.country == "DE")';

-- contracts with uptime SLO >= 99.95
SELECT did
FROM contracts
WHERE contract_data @? '$.sla.slos[*] ? (@.targetValue >= 99.95)';
```

## Naming Conventions

| Item | Convention | Example |
| --- | --- | --- |
| Ontology classes | PascalCase | `ContractAdjustment` |
| JSON properties | camelCase | `semanticConditionValues` |
| Existing DCS fields | Preserve current spelling | `conditionId`, `documentBlocks`, `templateDataVersion` |
| Rule IDs | kebab-case | `rule-access-before-contract-end` |
| Lifecycle values | PascalCase semantic values, existing DB enum in storage | `ReadyForSignature`, `APPROVED` |
| UUID references | URN UUID | `urn:uuid:...` |
| DID references | DID URI | `did:web:...` |
| Hashes | multialgorithm string | `sha256:...` |

## Implementation Path

1. Store the context, ontology, and shapes in Semantic Hub with version `v1`.
2. Add backend validation for parameter types, required flags, operators, placeholder references, and clause bindings.
3. Add `semanticProfile` to newly generated `contract_data`.
4. Extend template verification responses with semantic validation findings.
5. Emit semantic NATS/CloudEvents for validation, lifecycle, signing, deployment, and revocation.
6. Export deployment policy bundles from approved/signed contracts.
7. Add optional SHACL validation in PACM/CI once RDF expansion is operational.
