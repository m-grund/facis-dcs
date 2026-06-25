Feature: Verification witness
  Scenario: Verified PDFs are returned as append-only incremental updates
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
        "@id": "urn:doc:verify",
        "@type": ["dcs-pdf-core:Document", "prov:Bundle"],
        "title": "Verification Witness",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Verification",
            "clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "Verification re-renders the ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Entity"},
                  " extracted from the embedded JSON-LD payload."
                ]
              },
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "The ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Activity"},
                  " sealing this witness was performed by a ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:SoftwareAgent"},
                  " acting as the compiler runtime."
                ]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Integrity",
            "clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "Witness bytes are appended without disturbing the original prefix, preserving the ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Bundle"},
                  " integrity."
                ]
              }
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    When I verify the compiled PDF through /verify
    Then the response content type is "application/json"
    And the verified PDF is longer than the original
    And the verified PDF preserves the original bytes as a prefix
    And the verified PDF C2PA attachment contains two manifest boxes
    And the verified PDF preserves the original manifest bytes as the parent chain node
    And the verified PDF C2PA attachment contains the c2pa.ingredient assertion marker
    And the verified PDF ingredient references the parent manifest with matching hash
    And the verified PDF C2PA attachment includes an opened action with ingredient references
    And the active update manifest claim references c2pa.hash.data
    And the active update manifest c2pa.hash.data assertion hash matches the verified PDF bytes
    And the verified PDF contains these markers:
      | marker                  |
      | /Prev                   |
      | verification-witness    |
      | /Type /XRef             |
