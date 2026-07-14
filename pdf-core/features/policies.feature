Feature: dcs:policies (odrl:Set) rendering in human-readable PDF

  A dcs:ContractTemplate can carry a dcs:policies node: a single enclosing
  odrl:Set whose odrl:duty, odrl:permission, and odrl:prohibition buckets hold
  the contract's obligations, permissions, and prohibitions. Each rule declares
  an odrl:action, the odrl:assigner/odrl:assignee parties, an odrl:target, and
  an optional odrl:constraint. The compiler renders the set as a "Policies"
  section so the rules are visible, human-readable text in the output PDF.

  Background:
    Given the compiler service is running

  Scenario: An odrl:Set of duty, permission, and prohibition renders as readable text
    Given a semantic payload:
      """
      {
        "@context": {
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "odrl": "http://www.w3.org/ns/odrl/2/",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "did:web:localhost:template:policies-demo",
        "@type": "dcs:ContractTemplate",
        "dcs:metadata": {
          "@type": "dcs:TemplateMetadata",
          "dcs:title": "Policy Rendering Demo"
        },
        "dcs:documentStructure": {
          "@type": "dcs:DocumentStructure",
          "dcs:layout": [
            {
              "@type": "dcs:LayoutNode",
              "dcs:isRoot": true,
              "dcs:children": ["did:web:localhost:template:policies-demo#s1"]
            },
            {
              "@type": "dcs:LayoutNode",
              "@id": "did:web:localhost:template:policies-demo#s1",
              "dcs:children": ["did:web:localhost:template:policies-demo#c1"]
            }
          ],
          "dcs:blocks": [
            {
              "@type": "dcs:Section",
              "@id": "did:web:localhost:template:policies-demo#s1",
              "dcs:title": "1. Terms"
            },
            {
              "@type": "dcs:Clause",
              "@id": "did:web:localhost:template:policies-demo#c1",
              "dcs:content": ["An ordinary clause."]
            }
          ]
        },
        "dcs:policies": {
          "@id": "did:web:localhost:template:policies-demo#policy-set",
          "@type": "odrl:Set",
          "uid": "did:web:localhost:template:policies-demo",
          "odrl:profile": {"@id": "https://w3id.org/facis/dcs/odrl-profile/v1"},
          "odrl:duty": [
            {
              "@id": "did:web:localhost:template:policies-demo#policy-duty-0",
              "@type": "odrl:Duty",
              "odrl:action": {"@id": "https://w3id.org/facis/dcs/action/provideData"},
              "odrl:assigner": {"@id": "did:web:localhost:template:policies-demo#party-provider"},
              "odrl:assignee": {"@id": "did:web:localhost:template:policies-demo#party-customer"},
              "odrl:target": {"@id": "did:web:localhost:template:policies-demo"},
              "odrl:constraint": {
                "@type": "odrl:Constraint",
                "odrl:leftOperand": {"@id": "did:web:localhost:template:policies-demo#field-country"},
                "odrl:operator": {"@id": "odrl:isAnyOf"},
                "odrl:rightOperand": [{"@value": "DEU"}, {"@value": "AUT"}, {"@value": "CHE"}]
              }
            }
          ],
          "odrl:permission": [
            {
              "@id": "did:web:localhost:template:policies-demo#policy-perm-0",
              "@type": "odrl:Permission",
              "odrl:action": {"@id": "https://w3id.org/facis/dcs/action/inspect"},
              "odrl:assigner": {"@id": "did:web:localhost:template:policies-demo#party-provider"},
              "odrl:assignee": {"@id": "did:web:localhost:template:policies-demo#party-customer"},
              "odrl:target": {"@id": "did:web:localhost:template:policies-demo"}
            }
          ],
          "odrl:prohibition": [
            {
              "@id": "did:web:localhost:template:policies-demo#policy-proh-0",
              "@type": "odrl:Prohibition",
              "odrl:action": {"@id": "https://w3id.org/facis/dcs/action/redistribute"},
              "odrl:assigner": {"@id": "did:web:localhost:template:policies-demo#party-provider"},
              "odrl:assignee": {"@id": "did:web:localhost:template:policies-demo#party-customer"},
              "odrl:target": {"@id": "did:web:localhost:template:policies-demo"}
            }
          ]
        }
      }
      """
    When I compile the payload through /download
    Then the response status is 200
    And the response content type is "application/pdf"
    And the PDF contains these markers:
      | marker                                                       |
      | (Policies) Tj                                                |
      | (Obligation: provideData) Tj                                 |
      | (Condition: field-country must be any of DEU, AUT, CHE) Tj   |
      | (Permission: inspect) Tj                                     |
      | (Prohibition: redistribute) Tj                               |
