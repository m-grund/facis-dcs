# DCS ODRL Profile (v1)

Status: production-near PoC profile
Base vocabulary: [ODRL 2.2 Information Model / Vocabulary](https://www.w3.org/TR/odrl-model/), [ODRL JSON-LD context](https://www.w3.org/ns/odrl.jsonld)
Profile IRI: `https://w3id.org/facis/dcs/ontology/v1/odrl-profile`
Requirement: Workstream F ("Machine-readable contract soundness: real ODRL + server-side enforcement", `docs/anforderung.md`), Requirement-Slug `odrl-soundness`, ACs 1-3/8.

## 1. Why a profile

FACIS DCS embeds ODRL 2.2 rules directly inside `dcs:Contract`/`dcs:ContractTemplate` documents (`dcs:policies`) to make field-level obligations, permissions, and prohibitions machine-checkable both client-side (`useSemanticValueVerification.ts`) and server-side (`backend/internal/base/validation/contractcontentaudit.go`, `evaluateODRLConstraint`). Plain ODRL does not define what it means for a rule to be "about" a contract-data field value — that is what this profile adds, without introducing any new constraint-evaluation semantics beyond standard ODRL.

## 2. Structural requirements (normative for this profile)

A conformant `dcs:policies` value is exactly one of:

1. Absent, or an empty array (`[]`) — no policies declared yet.
2. A single JSON-LD object with `@type` `odrl:Set`:
   - `uid` — **MUST** equal the enclosing document's DID (the contract or template DID this policy set governs).
   - `odrl:profile` — **MUST** reference this profile's IRI, `https://w3id.org/facis/dcs/ontology/v1/odrl-profile`.
   - Zero or more rules held in the standard ODRL rule-bucket properties `odrl:permission`, `odrl:prohibition`, `odrl:duty` (and, for pass-through/interop, `odrl:obligation`), each an array or a single object.

The bare, un-enclosed array of `odrl:Duty`/`odrl:Permission`/`odrl:Prohibition` nodes that earlier FACIS DCS versions accepted (no enclosing `odrl:Set`, no `odrl:action`, no parties/target) is a **legacy shape and is explicitly rejected** by structural validation (`ValidateContractSemantics` / `NormalizeContractData` → `validateCanonicalEnvelope` → `validateODRLPoliciesShape`, `backend/internal/base/validation/documentdata.go`). This is a deliberate greenfield break (per `docs/anforderung.md` §0.1.4): there is no adapter/compat layer for the old shape.

## 3. Rule requirements (normative for this profile)

Every rule inside the enclosing `odrl:Set` **MUST** declare:

| Property | Cardinality | Meaning |
| --- | --- | --- |
| `odrl:action` | exactly 1 | see §4 below for the allowed action IRIs |
| `odrl:assigner` | exactly 1 (`@id` reference) | the party the rule is imposed *by* — for a template (open offer) this is a role-derived open reference; for a bound contract instance (agreement) it resolves against that contract's own party DIDs |
| `odrl:assignee` | exactly 1 (`@id` reference) | the party the rule is imposed *on* |
| `odrl:target` | exactly 1 (`@id` reference) | the contract / data-asset this rule governs — normally the enclosing document's own DID |
| `odrl:constraint` | 0 or 1 | unchanged from the pre-profile shape: `odrl:leftOperand` (an `@id` reference to a `dcs:RequirementField`), `odrl:operator` (an `@id` reference to one of the 8 supported operators, §5), `odrl:rightOperand` (a literal or array of literals) |

This is a standard ODRL Offer/Agreement distinction, not a DCS-specific extension: templates are open offers (parties not yet bound), contract instances are agreements (parties bound to real DIDs).

## 4. Actions

| Action IRI | When to use it |
| --- | --- |
| `dcs:provideCompliantValue` | The rule expresses "the bound contract-data field value must satisfy the declared constraint" — the common case for rules generated from semantic conditions/parameters in the template/contract editor. |
| `odrl:use`, `odrl:spatial` (as an operand context), `odrl:dateTime` (as an operand context) | Prefer the matching standard ODRL vocabulary term where a clause expresses a standard usage/spatial/temporal permission directly rather than a field-value constraint. |

The canonical single source for these IRIs in the frontend is `frontend/ClientApp/src/modules/template-repository/utils/sla-ontology-catalog.ts` (no hardcoded action-IRI lists in components, per `CLAUDE.md`).

## 5. Constraint operators (unchanged, already normative)

`odrl:eq`, `odrl:neq`, `odrl:gt`, `odrl:gteq`, `odrl:lt`, `odrl:lteq`, `odrl:isAnyOf`, `odrl:isNoneOf` — evaluated identically client-side (`useSemanticValueVerification.ts`) and server-side (`evaluateODRLConstraint`, `backend/internal/base/validation/contractcontentaudit.go:1069`).

## 6. Example

```json
{
  "@context": {
    "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
    "odrl": "http://www.w3.org/ns/odrl/2/"
  },
  "dcs:policies": {
    "@id": "urn:uuid:policy-set-1",
    "@type": "odrl:Set",
    "uid": "did:web:example.org:contract:acme-2026-001",
    "odrl:profile": { "@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile" },
    "odrl:duty": [
      {
        "@id": "urn:uuid:policy-provider-country-0",
        "@type": "odrl:Duty",
        "odrl:action": { "@id": "dcs:provideCompliantValue" },
        "odrl:assigner": { "@id": "did:web:example.org:contract:acme-2026-001#provider" },
        "odrl:assignee": { "@id": "did:web:example.org:contract:acme-2026-001#customer" },
        "odrl:target": { "@id": "did:web:example.org:contract:acme-2026-001" },
        "odrl:constraint": {
          "@type": "odrl:Constraint",
          "odrl:leftOperand": { "@id": "urn:uuid:field-provider-country" },
          "odrl:operator": { "@id": "odrl:isAnyOf" },
          "odrl:rightOperand": ["DEU", "AUT", "CHE"]
        }
      }
    ]
  }
}
```

## 7. Open follow-up (not in this pack's scope)

- SHACL shapes for this profile's structural rules (`docs/semantic-ontology/linkml/`, `shapes/`) are not yet added — the current structural enforcement lives in Go validation code (`documentdata.go`), not in the SHACL/LinkML pipeline. This is flagged for the verifier/next iteration rather than blocking this pack.
- Vendoring the official `https://www.w3.org/ns/odrl.jsonld` context for a full expand/compact roundtrip check has not been done in this pass; FACIS DCS currently declares its own minimal `odrl` context prefix (`http://www.w3.org/ns/odrl/2/`) inline. Logistics point for a follow-up, not a blocker for AC1-3/8.
