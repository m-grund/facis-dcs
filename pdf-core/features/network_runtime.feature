Feature: Network runtime
  Scenario: The service exposes the documented machine and human interfaces
    Given the compiler service is running
    When I fetch "/ui/"
    Then the response status is 200
    And the response content type starts with "text/html"
    And the response body contains "SwaggerUIBundle"
    When I fetch "/swagger.json"
    Then the response status is 200
    And the response content type starts with "application/json"
    And the response body contains "/download"
    And the response body contains "/verify"

  Scenario: The service self-hosts ontology assets
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:network-runtime",
        "@type": "dcs-pdf-core:Document",
        "title": "Hosted Ontology",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Runtime",
            "clauses": [
              "The runtime serves its own structural ontology and patch metadata."
            ]
          }
        ]
      }
      """
    When I fetch "/index.html"
    Then the response status is 200
    And the response content type starts with "text/html"
    And the response body contains "SwaggerUIBundle"
    When I fetch "/ontology/dcs-pdf-core"
    Then the response status is 200
    And the response content type starts with "application/ld+json"
    And the response body contains "@context"
    When I fetch "/ontology/dcs-pdf-core.owl"
    Then the response status is 200
    And the response content type starts with "application/ld+json"
    And the response body contains "@id"

  Scenario: The service rejects invalid content types
    Given the compiler service is running
    When I POST plain text to "/verify"
    Then the response status is 415
