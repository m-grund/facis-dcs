Feature: Subsection rendering, accessibility anchoring, and amendment lifecycle

  Sections nest to arbitrary depth via the dcs-pdf-core:subsections property.
  Each subsection level must:
    - appear in the PDF bookmark outline, nested under its parent
    - be indented further right than its parent
    - carry an /H2, /H3, or /H4 struct tag matching its depth
    - have its /Sect struct element properly nested inside the parent /Sect
      (not as a direct child of /Document) — this is the PDF/A-3a accessibility
      anchoring requirement
  The re-rendering guarantee applies to subsection content equally: amending a
  subsection clause and recompiling from the embedded JSON-LD must produce
  identical page content to a fresh compile of the amended payload.

  Background:
    Given the compiler service is running

  # ── Scenario 1 ──────────────────────────────────────────────────────────────
  # Three-level nesting: headings and struct tags must reflect depth.
  Scenario: Three-level subsection document compiles with correct PDF/A-3a structure
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:subsection-structure",
        "@type": "dcs-pdf-core:Document",
        "title": "Nested Obligations Document",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. General Obligations",
            "clauses": ["The parties shall comply with all applicable regulations."],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Data Protection",
                "clauses": ["Each party shall act as an independent data controller."],
                "subsections": [
                  {
                    "@type": "dcs-pdf-core:Section",
                    "heading": "1.1.1 Breach Notification",
                    "clauses": [
                      "A party suffering a data breach shall notify the other within 72 hours.",
                      "Notification shall include the nature of the breach and remediation steps taken."
                    ]
                  }
                ]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.2 Health and Safety",
                "clauses": ["Each party shall maintain a safe working environment at all times."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Dispute Resolution",
            "clauses": [
              "Disputes shall first be referred to a senior representative of each party.",
              "Unresolved disputes shall be submitted to binding arbitration."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "2.1 Arbitration Rules",
                "clauses": ["Arbitration shall be conducted under the ICC Rules of Arbitration."]
              }
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    Then the response status is 200
    And the response content type is "application/pdf"
    And the PDF contains these markers:
      | marker                          |
      | /H2                             |
      | /H3                             |
      | /S /H2                          |
      | /S /H3                          |
      | /Title (1.1 Data Protection)    |
      | /Title (1.1.1 Breach Notification) |
      | /Title (1.2 Health and Safety)  |
      | /Title (2.1 Arbitration Rules)  |
    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI

  # ── Scenario 2 ──────────────────────────────────────────────────────────────
  # Re-rendering from extracted JSON-LD must include all subsection content.
  Scenario: Re-rendering from extracted JSON-LD preserves all subsection headings
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:subsection-rerender",
        "@type": "dcs-pdf-core:Document",
        "title": "Environmental Compliance Policy",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Scope",
            "clauses": ["This policy applies to all operations of the Organisation."],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Definitions",
                "clauses": ["\"Operations\" includes manufacturing, logistics, and office activities."]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.2 Exclusions",
                "clauses": ["Research and development activities are governed by a separate policy."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Reporting",
            "clauses": ["Annual environmental reports shall be submitted to the Board."],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "2.1 Metrics",
                "clauses": [
                  "Reports shall include carbon footprint, water usage, and waste volumes.",
                  "Metrics shall be independently verified by a registered environmental auditor."
                ]
              }
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

  # ── Scenario 3 ──────────────────────────────────────────────────────────────
  # Amend a subsection clause; re-rendering from every lifecycle stage must
  # match a fresh compile of that version's payload.
  Scenario: Amending a subsection clause preserves re-rendering across sign and re-verify
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:subsection-amendment",
        "@type": "dcs-pdf-core:Document",
        "title": "Procurement Policy v1",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Supplier Selection",
            "clauses": ["Suppliers shall be assessed against cost, quality, and delivery criteria."],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Tendering Threshold",
                "clauses": [
                  "Contracts exceeding ten thousand (10,000) GBP require a formal tender process.",
                  "The Procurement Committee shall evaluate all tenders."
                ]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.2 Preferred Suppliers",
                "clauses": ["The preferred supplier list shall be reviewed annually."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Contract Management",
            "clauses": [
              "All contracts shall be managed by a designated Contract Owner.",
              "Contract renewals require approval from the Finance Director."
            ]
          }
        ],
        "signatureFields": [
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-cpo", "label": "Chief Procurement Officer"}
        ]
      }
      """
    And I compile the payload through /download

    # Baseline: re-render from compiled document.
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # CPO signs the policy.
    When I apply a PAdES signature to the compiled PDF at field "sig-cpo"
    Then the signed PDF contains a valid PAdES signature
    And the signed PDF has no extra AcroForm signature fields
    When I extract and recompile the embedded JSON-LD from the "signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Board updates the tendering threshold and adds a new emergency procurement subsection.
    When an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:subsection-amendment",
        "@type": "dcs-pdf-core:Document",
        "title": "Procurement Policy v2 (Amended)",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Supplier Selection",
            "clauses": ["Suppliers shall be assessed against cost, quality, and delivery criteria."],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Tendering Threshold",
                "clauses": [
                  "Contracts exceeding twenty-five thousand (25,000) GBP require a formal tender process.",
                  "The Procurement Committee shall evaluate all tenders.",
                  "Sole-source justification must be documented for single-supplier awards."
                ]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.2 Preferred Suppliers",
                "clauses": ["The preferred supplier list shall be reviewed annually."]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.3 Emergency Procurement",
                "clauses": [
                  "In circumstances of operational emergency, the CEO may waive tender requirements.",
                  "Emergency procurement shall be ratified by the Board within thirty (30) days."
                ]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Contract Management",
            "clauses": [
              "All contracts shall be managed by a designated Contract Owner.",
              "Contract renewals require approval from the Finance Director."
            ]
          }
        ],
        "signatureFields": [
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-cpo", "label": "Chief Procurement Officer"}
        ]
      }
      """
    And I update the signed PDF with the amended payload through /update
    Then the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And a fresh compile of the amended payload has different page content from the compiled PDF

    # Re-render from the amended document — must match the amended payload, not v1.
    When I extract and recompile the embedded JSON-LD from the "amended" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    # CPO signs again and compliance verifies the full lifecycle.
    When I apply a PAdES signature to the amended PDF at field "sig-cpo"
    Then the re-signed PDF contains a valid PAdES signature
    When I extract and recompile the embedded JSON-LD from the "re-signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload
    When I verify the "re-signed" PDF through /verify
    And I extract and recompile the embedded JSON-LD from the "verified" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI
