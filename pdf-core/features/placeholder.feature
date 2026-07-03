Feature: dcs:Placeholder rendering in human-readable PDF

  A dcs:Placeholder in a clause content list must be rendered as five underscores
  in the compiled PDF so that signatories can see where a fill-in value belongs.

  Scenario: Placeholder in clause content renders as five underscores
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:placeholder-test",
        "@type": "ContractTemplate",
        "documentTitle": "Placeholder Rendering Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Placeholder Rendering Test"
        },
        "contractData": [
          {
            "@id": "urn:doc:placeholder-test#req-party",
            "@type": "DataRequirement",
            "conditionId": "party",
            "name": "Party Details",
            "fields": [
              {
                "@id": "urn:doc:placeholder-test#field-party-name",
                "@type": "RequirementField",
                "parameterName": "party.name",
                "required": true
              }
            ]
          }
        ],
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:placeholder-test#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:placeholder-test#s1",
              "children": ["urn:doc:placeholder-test#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:placeholder-test#s1",
              "title": "1. Parties"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:placeholder-test#c1",
              "content": [
                "This agreement is entered into by ",
                {
                  "@type": "dcs:Placeholder",
                  "dcs:bindsTo": {"@id": "urn:doc:placeholder-test#field-party-name"}
                },
                "."
              ]
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the PDF contains these markers:
      | marker  |
      | _____   |
