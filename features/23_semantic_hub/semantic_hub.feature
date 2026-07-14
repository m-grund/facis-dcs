# Semantic Hub (DCS-FR-TR-03, UC-02-08, backend/design/semantic_hub.go):
# versioned storage for the JSON-LD contexts, SHACL shapes, and validation
# profiles every DCS document is produced against. Seeded at startup with the
# FACIS DCS v1 profile (backend/internal/semantichub/assets, authored under
# docs/semantic-ontology). Reads are public — produced artifacts carry
# hub-served schemaRefs external verifiers resolve without a DCS login —
# writes are Template Manager-scoped. The normalization layer anchors every
# produced canonical document to the hub (dcs:schemaRefs) and rejects
# documents that redefine a hub-declared ontology prefix.

@DCS-FR-TR-03 @UC-02-08
Feature: Semantic Hub — versioned schema storage, anchoring, and enforcement

  Scenario: The genesis FACIS DCS context is seeded, active, and publicly resolvable
    When the active "context" schema "facis-dcs" is retrieved from the Semantic Hub
    Then get http 200:Success code
    And the retrieved schema is version 1, active, of kind "context"
    And the retrieved schema content declares the "dcs" ontology IRI "https://w3id.org/facis/dcs/ontology/v1#"
    When the JSON-LD context "facis-dcs" is resolved from the Semantic Hub without authentication
    Then get http 200:Success code
    And the resolved document carries a JSON-LD "@context" object

  Scenario: The genesis SHACL shapes are seeded and retrievable
    When the active "shapes" schema "facis-dcs" is retrieved from the Semantic Hub
    Then get http 200:Success code
    And the retrieved schema is version 1, active, of kind "shapes"

  @UC-02-08
  Scenario: A Template Manager registers a new context version and rolls back to the genesis version
    When the Template Manager registers a new active version of the "context" schema "facis-dcs" extending the genesis context
    Then get http 200:Success code
    And the schema registration reports version 2 as active
    And the Semantic Hub lists 2 versions of the "context" schema "facis-dcs" with version 2 active
    When the Template Manager rolls the "context" schema "facis-dcs" back to version 1
    Then get http 200:Success code
    And the Semantic Hub lists 2 versions of the "context" schema "facis-dcs" with version 1 active

  Scenario: A role outside the schema-management scope cannot register a schema version
    Given I am authenticated with roles: "Template Creator"
    When I attempt to register a Semantic Hub schema version with my current role
    Then the request is denied with a client error

  # The anchoring half of DCS-FR-TR-03: every produced canonical document
  # records the hub-served, versioned schema URLs it was produced against
  # (dcs:schemaRefs, injected by the normalization layer), and that anchor
  # RESOLVES against this instance's Semantic Hub.
  @DCS-FR-TR-03
  Scenario: A produced contract document is anchored to the Semantic Hub and the anchor resolves
    Given contract "Hub Anchored Contract" is in "Draft" status
    Then the contract "Hub Anchored Contract" carries a Semantic Hub schema anchor
    And the contract "Hub Anchored Contract"'s JSON-LD context anchor resolves to the hub's registered context

  # The enforcement half ("templating should use it"): the hub's active
  # context is authoritative for what the ontology prefixes mean — a template
  # redefining a hub-declared prefix to a different IRI is rejected at
  # creation.
  @DCS-FR-TR-03
  Scenario: A template redefining a hub-declared ontology prefix is rejected
    When a template is created whose "@context" redefines the "dcs" prefix to "https://evil.example/other-ontology#"
    Then the request is denied with a client error
    And the rejection names the Semantic Hub's active context
