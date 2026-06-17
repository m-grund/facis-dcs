Feature: Structural markers
  Scenario: Generated PDFs expose the expected semantic markers
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "sosa": "http://www.w3.org/ns/sosa/",
          "ssn": "http://www.w3.org/ns/ssn/",
          "schema": "https://schema.org/",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:markers",
        "@type": ["dcs-pdf-core:Document", "sosa:ObservationCollection"],
        "title": "Semantic Marker Ledger",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Observations",
            "clauses": [
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "Each ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "sosa:Observation"},
                  " in this collection was produced by a calibrated ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "sosa:Sensor"},
                  "."
                ]
              },
              {
                "@type": "dcs-pdf-core:Clause",
                "content": [
                  "The ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "sosa:ObservableProperty"},
                  " is linked to the originating ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "sosa:Sensor"},
                  " via the ",
                  {"@type": "dcs-pdf-core:ContentNode", "@id": "ssn:implements"},
                  " relation."
                ]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Provenance",
            "clauses": [
              "A semantic glossary is appended after the body clauses by resolving the SOSA ontology at runtime."
            ]
          }
        ]
      }
      """
    When I compile the payload through /download
    Then the PDF contains these markers:
      | marker                    |
      | /Type /Catalog            |
      | /MarkInfo << /Marked true |
      | /StructTreeRoot           |
      | /OutputIntents            |
      | /AF [                     |
      | /AFRelationship /Source   |
      | /EmbeddedFile             |
      | /Subtype /application#2Fld+json |
      | /Annots [                 |
      | /XYZ 54.00                |
      | /Type /StructTreeRoot     |
      | /S /Document              |
      | /S /Sect                  |
      | /S /H1                    |
      | /Outlines                 |
    And the PDF exposes positive non-overlapping text coordinates
