Feature: Amendment
  Scenario: Amended PDFs are returned as incremental updates preserving original content
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "prov": "http://www.w3.org/ns/prov#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:amendment-base",
        "@type": ["dcs-pdf-core:Document", "prov:Bundle"],
        "title": "Amendment Test Document",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Original Terms",
            "clauses": [
              "Original clause established at document creation."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    And an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "prov": "http://www.w3.org/ns/prov#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:amendment-base",
        "@type": ["dcs-pdf-core:Document", "prov:Bundle"],
        "title": "Amendment Test Document",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Original Terms",
            "clauses": [
              "Original clause established at document creation."
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Amendment",
            "clauses": [
              "Amendment clause added to reflect updated terms."
            ]
          }
        ]
      }
      """
    When I update the compiled PDF with the amended payload through /update
    Then the response content type is "application/pdf"
    And the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And the amended PDF embeds the new JSON-LD payload
    And the response body contains "Amendment clause added to reflect updated terms."

  Scenario: Updating with an unchanged payload returns a conflict error
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "prov": "http://www.w3.org/ns/prov#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:amendment-nochange",
        "@type": ["dcs-pdf-core:Document", "prov:Bundle"],
        "title": "No-Change Test",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Terms",
            "clauses": [
              "This clause will not change."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    When I update the compiled PDF with the same payload through /update
    Then the response status is 409
