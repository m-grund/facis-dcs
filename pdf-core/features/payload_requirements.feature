Feature: Payload ontology requirements
  Scenario: Missing document title is rejected
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "schema": "https://schema.org/",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-missing-title",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Intro",
            "clauses": [
              {"@type": "dcs-pdf-core:Clause", "content": ["Clause text"]}
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "payload failed SHACL validation"
    And the response body contains "dcs-pdf-core#title"

  Scenario: Multiple document titles are rejected
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "schema": "https://schema.org/",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-multi-title",
        "title": ["Version A", "Version B"],
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Intro",
            "clauses": [
              {"@type": "dcs-pdf-core:Clause", "content": ["Clause text"]}
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "dcs-pdf-core#title"
    And the response body contains "MaxCountConstraintComponent"

  Scenario: Section heading is required
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-no-heading",
        "title": "Missing Heading",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "clauses": [
              {"@type": "dcs-pdf-core:Clause", "content": ["Clause text"]}
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "dcs-pdf-core#heading"

  Scenario: Section clauses are required
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-no-clauses",
        "title": "Missing Clauses",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Intro"
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "dcs-pdf-core#clauses"

  Scenario: Clause object content is required
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-no-content",
        "title": "Missing Clause Content",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Intro",
            "clauses": [
              {"@type": "dcs-pdf-core:Clause"}
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the response status is 400
    And the response body contains "dcs-pdf-core#content"

  Scenario: Equivalent JSON-LD flavors compile to identical output
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "schema": "https://schema.org/",
          "prov": "http://www.w3.org/ns/prov#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:req-flavors",
        "@type": "dcs-pdf-core:Document",
        "title": "Flavor Equivalence",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Terms",
            "clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "Reference ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Entity"},
                  " remains equivalent across JSON-LD flavors."
                ]
              }
            ]
          }
        ]
      }
      """
    And an equivalent semantic payload flavor:
      """
      {
        "@context": {
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "schema": "https://schema.org/",
          "prov": "http://www.w3.org/ns/prov#"
        },
        "@id": "urn:doc:req-flavors",
        "@type": "dcs-pdf-core:Document",
        "dcs-pdf-core:title": "Flavor Equivalence",
        "dcs-pdf-core:sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "dcs-pdf-core:heading": "1. Terms",
            "dcs-pdf-core:clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "dcs-pdf-core:content": [
                  "Reference ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "prov:Entity"},
                  " remains equivalent across JSON-LD flavors."
                ]
              }
            ]
          }
        ]
      }
      """
    When I compile both payload flavors through /download
    Then both compiled payload flavors are byte-for-byte identical
