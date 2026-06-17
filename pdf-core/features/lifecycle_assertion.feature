Feature: DCS lifecycle assertion in C2PA manifest (DCS-OR-C2PA-003)

  Scenario: Compiled PDF dcs.lifecycle assertion contains all required fields
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:lifecycle-assertion-test",
        "@type": "dcs-pdf-core:Document",
        "title": "Lifecycle Assertion Test",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Terms", "clauses": ["Testing lifecycle assertion embedding."]}
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the embedded C2PA manifest contains a dcs.lifecycle assertion
    And the dcs.lifecycle assertion has contract_id "urn:doc:lifecycle-assertion-test"
    And the dcs.lifecycle assertion has a non-empty file_hash
    And the dcs.lifecycle assertion has status "draft"
    And the dcs.lifecycle assertion has a valid effective_at timestamp
    And the dcs.lifecycle assertion has an empty prev_manifest_hash
    And the dcs.lifecycle assertion has field "reason"
    And the dcs.lifecycle assertion has field "authority"
    And the dcs.lifecycle assertion has field "vc_id"

  Scenario: Updated PDF dcs.lifecycle assertion has status amended and prev_manifest_hash set
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:lifecycle-update-test",
        "@type": "dcs-pdf-core:Document",
        "title": "Lifecycle Update Test",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Original", "clauses": ["Original clause."]}
        ]
      }
      """
    And I compile the payload through /download
    When I amend the PDF with a new payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:lifecycle-update-test",
        "@type": "dcs-pdf-core:Document",
        "title": "Lifecycle Update Test",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Original", "clauses": ["Original clause.", "Amended clause."]}
        ]
      }
      """
    Then the response status is 200
    And the active C2PA manifest in the updated PDF contains a dcs.lifecycle assertion
    And the dcs.lifecycle assertion in the updated manifest has contract_id "urn:doc:lifecycle-update-test"
    And the dcs.lifecycle file_hash differs from the compiled PDF file_hash
    And the dcs.lifecycle assertion in the updated manifest has status "amended"
    And the dcs.lifecycle assertion in the updated manifest has a valid effective_at timestamp
    And the dcs.lifecycle assertion in the updated manifest has a non-empty prev_manifest_hash
