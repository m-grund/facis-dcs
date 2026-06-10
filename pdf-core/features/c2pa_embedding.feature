Feature: C2PA embedding
  Scenario: Generated PDFs include core C2PA manifest-store structures
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "prov": "http://www.w3.org/ns/prov#",
          "schema": "https://schema.org/",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:c2pa-coverage",
        "@type": "dcs-pdf-core:Document",
        "title": "C2PA Coverage Ledger",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Provenance",
            "clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "This document records provenance through ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Entity"},
                  " relationships."
                ]
              }
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the embedded C2PA attachment starts with a JUMBF superbox
    And the embedded C2PA attachment contains these markers:
      | marker          |
      | c2pa            |
      | c2pa.assertions |
      | c2pa.claim.v2   |
      | c2pa.signature  |
    And the embedded c2pa.hash.data assertion hash matches the compiled PDF bytes
    And the embedded c2pa.hash.data assertion payload contains the "pad" field
    And the embedded signing leaf certificate is x509 v3
    And the embedded signing leaf certificate includes emailProtection EKU
    And the embedded C2PA signature includes x5chain protected header key
    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI
