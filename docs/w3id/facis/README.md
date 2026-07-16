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
