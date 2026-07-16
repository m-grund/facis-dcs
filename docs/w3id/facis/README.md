# FACIS DCS vocabulary namespace — identity and resolution

The vocabulary namespace (`dcs:`/`dcst:` term IRIs) is the *identity* of
the FACIS vocabulary, shared by every document in the ecosystem. Two
instances minting terms under different namespaces produce RDF that no
longer unifies — `dcs:title` from one is a different predicate than
`dcs:title` from the other, shapes stop targeting each other's data, and
the ecosystem decays into dialect islands. The namespace is therefore a
**global constant, not a per-instance setting**.

Two ways to make it dereferenceable:

1. **w3id.org registration** (preferred, permanent): see below. Until the
   PR merges the IRIs are stable identifiers that simply do not resolve
   yet — the served copies remain fetchable from any instance's public
   `/semantic/ontology/facis-dcs` and `/semantic/context/facis-dcs`
   routes.
2. **`DCS_ONTOLOGY_BASE_IRI`** (helm: `route.ontologyBaseIRI`): mints the
   namespaces under an organization-controlled base instead of w3id at
   genesis-seed time. This is an ecosystem-wide decision: every
   participating instance MUST be configured with the identical value,
   set before its first boot (hub versions are immutable — changing it
   later orphans previously produced documents' vocabulary). Choose a
   base whose domain is as permanent as the documents produced under it.

# w3id.org registration for the FACIS DCS namespaces

The `https://w3id.org/facis/...` IRIs used throughout FACIS DCS documents
(ontology terms, taxonomy values, the historical context/shapes
identifiers) resolve through [w3id.org](https://w3id.org), a community-run
permanent-identifier redirect service. Registration is a pull request to
<https://github.com/perma-id/w3id.org> adding the `facis/` directory with
the `.htaccess` in this folder.

Before filing the PR, replace `FACIS_PUBLIC_HOST` in `.htaccess` with the
canonical public DCS deployment origin (scheme + host + API base path,
e.g. `https://dcs.example.org/digital-contracting-service/api`). The
redirect targets are the Semantic Hub's public resolution routes:

| w3id IRI                                   | resolves to                          |
|--------------------------------------------|--------------------------------------|
| `/facis/dcs/ontology/v1` (and `#term`)     | `/semantic/ontology/facis-dcs`       |
| `/facis/dcs/context/v1`                    | `/semantic/context/facis-dcs`        |
| `/facis/dcs/shapes/v1`                     | `/semantic/shapes/facis-dcs`         |
| `/facis/dcs/taxonomy/v1` (and `#term`)     | `/semantic/ontology/facis-dcs`       |

All four routes are public and unauthenticated (`backend/design/
semantic_hub.go`), so the redirect targets dereference without a DCS
login. Fragment identifiers (`#ContractMetadata`, `#role-provider`, ...)
are preserved by the redirect per RFC 7231 and land on the served
ontology document.
