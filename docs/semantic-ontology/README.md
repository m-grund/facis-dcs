# FACIS DCS Semantic Architecture

Every DCS document — template or contract — is one canonical JSON-LD
envelope, and everything semantic about it is served, versioned, by the
Semantic Hub (`/semantic/...`). The hub's embedded seed documents at
`backend/internal/semantichub/assets/` are the single authoring source:

| Hub entry (name / kind) | Content |
| --- | --- |
| `facis-dcs` / context | The JSON-LD context documents anchor as their `@context` |
| `facis-dcs` / shapes | SHACL shapes for the canonical envelope (goRDFlib enforces them at submission and signing) |
| `clause-catalog` / shapes | Typed clause NodeShapes — the builder palette renders forms from this Turtle (shacl-form) and validation enforces the same graph |
| `facis-dcs` / ontology | The dcs: envelope vocabulary (dereference target of the term IRIs) |
| `facis-sla` / ontology | The SLA domain-field catalog: `dcs:DomainField` properties with `rdfs:domain`/`rdfs:range`, value constraints, taxonomy value options |
| `dcs-odrl-profile` / ontology | The ODRL profile: supported constraint operators (with per-type applicability), actions, default action |
| `facis.sla.basic` / profile | Statement-level business rules (`dcterms:conformsTo` target) |

Versions are immutable and monotonic; registering + activating a new
version changes what newly produced documents are validated against, while
already-produced documents stay pinned via their `@context` URL,
`sh:shapesGraph`, and `dcterms:conformsTo` anchors.

## The canonical envelope

A document is a `dcs:ContractTemplate` or `dcs:Contract` carrying:

- `dcs:metadata` — title, description, template type.
- `dcs:documentStructure` — ordered `dcs:blocks` (`dcs:Section`,
  `dcs:TextBlock`, `dcs:Clause` — a clause's `dcs:content` list mixes prose
  strings and `dcs:Placeholder` segments binding to requirement fields; a
  clause may carry a `dcs:typedClause` instance shaped by the hub's clause
  catalog) and a `dcs:layout` tree.
- `dcs:contractData` — `dcs:DataRequirement` groups of
  `dcs:RequirementField`s; a field may reference a catalog property via
  `dcs:domainField` (the field IRI is its identity).
- `dcs:policies` — ONE enclosing ODRL policy: `odrl:Offer` until the first
  signature seals it into an `odrl:Agreement`. Every rule carries
  `odrl:action`, `odrl:assigner`, `odrl:assignee`, `odrl:target`, and
  `dcs:prose` (the human-readable clause it operationalizes).
- `semanticConditionValues` — submitted runtime values:
  `{ forField: <RequirementField IRI>, parameterValue, blockId }`.
- Contracts add `derivedFromTemplate` (provenance) and `dcs:parties`
  (`dcs:CompanyParty` nodes with `dcs:role`).

Real, current examples: [contract.jsonld](examples/contract.jsonld)
(placeholder fill + ODRL Offer) and
[typed-clause-contract.jsonld](examples/typed-clause-contract.jsonld)
(hub typed clause carried through template derivation).

## Enforcement

- `validateAgainstHubShapes` (goRDFlib, ADR-9) expands a document against
  its pinned hub context and validates against the pinned shapes graph
  (canonical shapes + clause catalog). Error findings BLOCK contract
  submission and signature application.
- The validation profile evaluates statement-level business rules against
  the expanded typed nodes (`backend/internal/base/validation/
  contractstatementvalidation.go`).
- ODRL constraints over requirement fields are evaluated server-side at
  approval and against reported KPI metrics after deployment
  (`odrlexpanded.go`; the profile is `dcs-odrl-profile`).
- Signing embeds SHACL evidence (shapes version + findings hash) into the
  signing-summary credential so external verifiers can re-run validation
  against the exact pinned shapes (ADR-8).

## LinkML (pdf-core)

`linkml/linkml.yaml` is the machine-readable schema of the canonical
envelope. Its generated context/OWL/SHACL outputs are embedded by pdf-core
for deterministic rendering and payload canonicalization — regenerate with
`make -C docs/semantic-ontology/linkml`.

## Runtime rule

JSON-LD is the source of record. RDF is derived for validation and
interoperability; no OWL inference is required at runtime.
