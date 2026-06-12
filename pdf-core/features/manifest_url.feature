Feature: Remote manifest URL embedding (DCS-OR-C2PA-008)
  Scenario: Update with manifest_url embeds a remote_manifests entry in the C2PA claim
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:manifest-url-base",
        "@type": "dcs-pdf-core:Document",
        "title": "Remote Manifest Base",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Provenance", "clauses": ["Base document."]}
        ]
      }
      """
    And I compile the payload through /download
    When I amend the PDF with a new payload and manifest URL "https://api.example.com/contracts/did:example:abc/c2pa-manifest":
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:manifest-url-base",
        "@type": "dcs-pdf-core:Document",
        "title": "Remote Manifest Base",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Provenance", "clauses": ["Base document.", "Amendment one."]}
        ]
      }
      """
    Then the response status is 200
    And the updated PDF C2PA manifest contains the remote manifest URL "https://api.example.com/contracts/did:example:abc/c2pa-manifest"

  Scenario: Update without manifest_url produces no remote_manifests entry
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:manifest-url-absent",
        "@type": "dcs-pdf-core:Document",
        "title": "No Remote Manifest",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Provenance", "clauses": ["Base document."]}
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
        "@id": "urn:doc:manifest-url-absent",
        "@type": "dcs-pdf-core:Document",
        "title": "No Remote Manifest",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Provenance", "clauses": ["Base document.", "Amendment one."]}
        ]
      }
      """
    Then the response status is 200
    And the updated PDF C2PA manifest contains no remote manifest URL

  Scenario: POST /manifest/extract returns the embedded JUMBF manifest store
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:manifest-extract",
        "@type": "dcs-pdf-core:Document",
        "title": "Manifest Extract Test",
        "sections": [
          {"@type": "dcs-pdf-core:Section", "heading": "1. Content", "clauses": ["Extract the manifest."]}
        ]
      }
      """
    And I compile the payload through /download
    When I extract the C2PA manifest store from the compiled PDF
    Then the response status is 200
    And the response content type is "application/octet-stream"
    And the manifest store response contains the JUMBF marker

  Scenario: POST /manifest/extract rejects non-PDF content type
    Given the compiler service is running
    When I POST plain text to "/manifest/extract"
    Then the response status is 415
