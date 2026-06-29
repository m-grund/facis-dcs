Feature: Deterministic re-rendering guarantee across the full document lifecycle

  The human-readable page content (BT/ET text operator blocks) produced by
  CompilePDF is invariant: extracting the embedded JSON-LD from any version of
  a PDF — original, verified, signed, amended, re-signed — and recompiling it
  must produce page content byte-for-byte identical to a fresh compile of that
  payload.  This guarantee is the entire basis of the system's tamper-evidence.

  Background:
    Given the compiler service is running

  # ── Scenario 1 ──────────────────────────────────────────────────────────────
  # Identical payloads must always produce identical PDFs.
  Scenario: Double compilation of the same payload yields identical PDFs
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:determinism-idempotent",
        "@type": "dcs-pdf-core:Document",
        "title": "Determinism Idempotency Check",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Definitions",
            "clauses": [
              "\"Agreement\" means this document.",
              "\"Party\" means any signatory hereto."
            ]
          }
        ]
      }
      """
    When I compile the payload twice through /download
    Then both PDF responses are byte-for-byte identical

  # ── Scenario 2 ──────────────────────────────────────────────────────────────
  # Re-rendering from the embedded JSON-LD must survive C2PA verification and
  # two rounds of PAdES signatures.  Real-life analogue: legal team compiles a
  # draft, compliance verifies it, counsel signs, then counterparty signs.
  Scenario: Re-rendering survives C2PA verification and dual PAdES signatures
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:determinism-sign-verify",
        "@type": "dcs-pdf-core:Document",
        "title": "Master Services Agreement",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Definitions",
            "clauses": [
              "\"Services\" means the professional services described in Schedule A.",
              "\"Fees\" means the amounts payable under clause 3."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Interpretation",
                "clauses": [
                  "Words in the singular include the plural and vice versa.",
                  "References to statutes include all amendments thereto."
                ]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Obligations",
            "clauses": [
              "The Provider shall deliver the Services by the agreed milestone dates.",
              "The Client shall pay invoices within thirty (30) calendar days of receipt."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "2.1 Standard of Care",
                "clauses": ["Services shall be performed with reasonable professional skill and care."]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "2.2 Acceptance",
                "clauses": [
                  "The Client shall notify the Provider of defects within ten (10) business days.",
                  "Failure to notify within this period constitutes acceptance."
                ]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "3. Confidentiality",
            "clauses": [
              "Each party shall keep the other party's Confidential Information strictly confidential.",
              "This obligation survives termination for five (5) years."
            ]
          }
        ],
        "signatureFields": [
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-provider", "label": "Service Provider"},
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-client",   "label": "Client"}
        ]
      }
      """
    And I compile the payload through /download

    # Re-render from the compiled PDF itself — baseline check.
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Compliance verifies the compiled PDF (C2PA witness appended).
    When I verify the compiled PDF through /verify
    And I extract and recompile the embedded JSON-LD from the "verified" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Counsel (Party A) applies a PAdES signature.
    When I apply a PAdES signature to the compiled PDF at field "sig-provider"
    And I extract and recompile the embedded JSON-LD from the "signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload
    And the signed PDF contains a valid PAdES signature
    And the signed PDF has no extra AcroForm signature fields

    # Counterparty (Party B) countersigns.
    When I apply a PAdES signature to the amended PDF at field "sig-client"
    And I extract and recompile the embedded JSON-LD from the "re-signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload
    And the re-signed PDF contains a valid PAdES signature
    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI

  # ── Scenario 3 ──────────────────────────────────────────────────────────────
  # Full convoluted real-life lifecycle:
  #   compile v1 → verify → sign (Party A) → amend → re-sign (Party B) →
  #   verify again → extract from every stage and confirm re-rendering.
  #
  # Analogue: MSA is compiled; compliance witnesses; lead counsel signs; a
  # material clause is corrected in amendment; counterparty's counsel signs
  # the amendment; the final document is verified for archival.
  Scenario: Re-rendering guarantee holds across amend, re-sign, and re-verify lifecycle
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:determinism-full-lifecycle",
        "@type": "dcs-pdf-core:Document",
        "title": "Software Licence Agreement v1",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Grant of Licence",
            "clauses": [
              "The Licensor grants the Licensee a non-exclusive, non-transferable licence.",
              "The licence covers use on up to ten (10) devices."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Permitted Use",
                "clauses": ["The Licensee may use the Software solely for internal business purposes."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Restrictions",
            "clauses": [
              "The Licensee shall not sublicense, sell, or transfer the Software.",
              "Reverse engineering is prohibited except where required by applicable law."
            ]
          }
        ],
        "signatureFields": [
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-licensor", "label": "Licensor"},
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-licensee", "label": "Licensee"}
        ]
      }
      """
    And I compile the payload through /download

    # Baseline: re-render from the compiled document.
    When I extract and recompile the embedded JSON-LD from the "compiled" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Compliance team verifies the compiled PDF.
    When I verify the compiled PDF through /verify
    And I extract and recompile the embedded JSON-LD from the "verified" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Lead counsel (Licensor) signs the compiled document.
    When I apply a PAdES signature to the compiled PDF at field "sig-licensor"
    Then the signed PDF is longer than the compiled PDF
    And the signed PDF preserves the compiled PDF bytes as a prefix
    And the signed PDF contains a valid PAdES signature
    And the signed PDF has no extra AcroForm signature fields
    When I extract and recompile the embedded JSON-LD from the "signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the original payload

    # Legal review finds that the device limit clause needs correction.
    # Amendment: ten → fifty devices, and a new Audit Rights section is added.
    When an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:determinism-full-lifecycle",
        "@type": "dcs-pdf-core:Document",
        "title": "Software Licence Agreement v2 (Amended)",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Grant of Licence",
            "clauses": [
              "The Licensor grants the Licensee a non-exclusive, non-transferable licence.",
              "The licence covers use on up to fifty (50) devices."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Permitted Use",
                "clauses": ["The Licensee may use the Software solely for internal business purposes."]
              },
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.2 Additional Sites",
                "clauses": ["Use at additional sites requires prior written consent of the Licensor."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Restrictions",
            "clauses": [
              "The Licensee shall not sublicense, sell, or transfer the Software.",
              "Reverse engineering is prohibited except where required by applicable law."
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "3. Audit Rights",
            "clauses": [
              "The Licensor may audit the Licensee's use of the Software on thirty (30) days notice.",
              "Audits shall be conducted during normal business hours at the Licensee's premises."
            ]
          }
        ],
        "signatureFields": [
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-licensor", "label": "Licensor"},
          {"@type": "dcs-pdf-core:SignatureField", "name": "sig-licensee", "label": "Licensee"}
        ]
      }
      """
    And I update the signed PDF with the amended payload through /update
    Then the response content type is "application/pdf"
    And the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And the amended PDF embeds the new JSON-LD payload
    And the amended PDF C2PA attachment contains 2 manifest boxes

    # Sanity check: v2 renders differently from v1.
    And a fresh compile of the amended payload has different page content from the compiled PDF

    # Re-render from the amended document.
    When I extract and recompile the embedded JSON-LD from the "amended" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    # Counterparty counsel (Licensee) signs the amendment.
    When I apply a PAdES signature to the amended PDF at field "sig-licensee"
    Then the re-signed PDF is longer than the amended PDF
    And the re-signed PDF preserves the amended PDF bytes as a prefix
    And the re-signed PDF contains a valid PAdES signature
    And the re-signed PDF contains 2 PAdES signatures

    # Re-render from the re-signed document (amendment + signature appended).
    When I extract and recompile the embedded JSON-LD from the "re-signed" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    # Final archive verification of the fully signed amended document.
    When I verify the "re-signed" PDF through /verify
    And I extract and recompile the embedded JSON-LD from the "verified" PDF
    Then the recompiled PDF page content matches a fresh compile of the amended payload

    And all saved PDF artifacts are validated by c2patool and dockerized veraPDF CLI

  # ── Scenario 4 ──────────────────────────────────────────────────────────────
  # A single flipped bit in the human-readable page content must be caught by
  # /verify.  The re-render comparison detects any divergence between the
  # submitted bytes and what the embedded JSON-LD would produce.
  Scenario: Verify rejects a compiled PDF whose page content has been tampered with
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:tamper-compiled",
        "@type": "dcs-pdf-core:Document",
        "title": "Non-Disclosure Agreement",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Confidential Information",
            "clauses": [
              "Each party agrees to hold the other's Confidential Information in strict confidence.",
              "Confidential Information shall not be disclosed to any third party without prior written consent."
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Permitted Disclosures",
            "clauses": [
              "Disclosure is permitted where required by law or court order.",
              "The disclosing party shall give prompt notice of any compelled disclosure."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download

    # Flip one byte inside a BT/ET text block — the visible text now diverges from
    # what the embedded JSON-LD describes.
    Given I tamper with the page content of the "compiled" PDF
    When I verify the tampered PDF through /verify
    # The re-render comparison catches the divergence: embedded payload reproduces
    # a different byte sequence than the tampered PDF.
    Then the response status is 409

  # ── Scenario 5 ──────────────────────────────────────────────────────────────
  # Tampering with an amended PDF is equally detectable: the incremental
  # provenance chain check must fail when page content bytes are modified.
  Scenario: Verify rejects an amended PDF whose page content has been tampered with
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:tamper-amended",
        "@type": "dcs-pdf-core:Document",
        "title": "Service Level Agreement v1",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Availability",
            "clauses": [
              "The Service shall be available ninety-nine point nine percent (99.9%) of the time.",
              "Downtime is measured over each calendar month."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download
    When an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:tamper-amended",
        "@type": "dcs-pdf-core:Document",
        "title": "Service Level Agreement v2",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Availability",
            "clauses": [
              "The Service shall be available ninety-nine point nine percent (99.9%) of the time.",
              "Downtime is measured over each calendar month."
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Remedies",
            "clauses": [
              "A service credit of five percent (5%) of monthly fees shall apply per hour of excess downtime.",
              "Credits shall be applied to the next invoice automatically."
            ]
          }
        ]
      }
      """
    And I update the compiled PDF with the amended payload through /update
    Then the response status is 200

    # Tamper with the amended PDF — flip a byte in the incremental update's
    # page content stream.  The provenance chain check must detect the divergence.
    Given I tamper with the page content of the "amended" PDF
    When I verify the tampered PDF through /verify
    Then the response status is 409

  # ── Scenario 6 ──────────────────────────────────────────────────────────────
  # After a legitimate /verify the C2PA witness records a hash of the exact PDF
  # bytes at that point.  Tampering with the verified PDF's page content breaks
  # the c2pa.hash.data binding — the hash in the manifest no longer matches the
  # actual bytes.  This provides an independent, cryptographic tamper-evidence
  # layer complementary to the re-render check.
  Scenario: Tampering with a verified PDF invalidates its C2PA hash binding
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:tamper-c2pa",
        "@type": "dcs-pdf-core:Document",
        "title": "Escrow Agreement",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Deposit",
            "clauses": [
              "The Depositor shall place the Source Code into escrow within thirty (30) days of execution.",
              "The Escrow Agent shall acknowledge receipt in writing."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Verification of Deposit",
                "clauses": ["The Beneficiary may request a technical verification of the deposited materials once per year."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Release Conditions",
            "clauses": [
              "Escrow materials shall be released upon insolvency of the Depositor.",
              "A written request from the Beneficiary is required to trigger release."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download

    # Legitimate C2PA witness is appended — the manifest records the exact hash
    # of the PDF bytes at this point.
    When I verify the compiled PDF through /verify

    # Flip one byte in the verified PDF's human-readable page content.
    # The C2PA manifest still holds the hash of the pre-tamper bytes.
    Given I tamper with the page content of the "verified" PDF

    # The c2pa.hash.data exclusions cover only the JUMBF stream; the flipped byte
    # is in a page content stream and therefore inside the hash boundary.
    # The stored hash must no longer match the tampered bytes.
    Then the tampered PDF C2PA hash does not match its content

  # ── Scenario 7 ──────────────────────────────────────────────────────────────
  # An adversary downloads a system-compiled PDF and appends content using an
  # ordinary PDF editor (Acrobat, LibreOffice, etc.) rather than the /update
  # endpoint.  The offline editor preserves the original compiled bytes as a
  # byte-for-byte prefix and adds a structurally valid incremental update section
  # — the kind of change that a naive "is the original prefix intact?" check
  # would miss entirely.
  #
  # /verify must still reject this because the re-render comparison operates on
  # the FULL submitted bytes: CompilePDF(embeddedPayload) never emits the
  # editor's annotation object, so the two byte sequences can never be equal.
  #
  # The absence of the dcs-pdf-core incremental marker routes this through the
  # plain bytes.Equal path, not the provenance chain path.  Either way the
  # offline amendment is detected.
  Scenario: Verify rejects a PDF amended offline even though the original prefix is intact
    Given a semantic payload:
      """
      {
        "@context": {
          "@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
          "dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
        },
        "@id": "urn:doc:offline-amendment",
        "@type": "dcs-pdf-core:Document",
        "title": "Distribution Agreement",
        "sections": [
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "1. Grant of Rights",
            "clauses": [
              "The Supplier grants the Distributor exclusive rights to market the Products in the Territory.",
              "The Territory is defined as the United Kingdom and the Republic of Ireland."
            ],
            "subsections": [
              {
                "@type": "dcs-pdf-core:Section",
                "heading": "1.1 Sub-Distribution",
                "clauses": ["The Distributor shall not appoint sub-distributors without prior written consent."]
              }
            ]
          },
          {
            "@type": "dcs-pdf-core:Section",
            "heading": "2. Minimum Purchase Obligations",
            "clauses": [
              "The Distributor shall purchase no less than one thousand (1,000) units per quarter.",
              "Failure to meet the minimum triggers a right of termination in favour of the Supplier."
            ]
          }
        ]
      }
      """
    And I compile the payload through /download

    # A PDF editor appends a structurally valid incremental update — review
    # annotation and updated xref — without using /update.  The original
    # compiled bytes are preserved byte-for-byte as a prefix.
    Given I apply an offline amendment to the compiled PDF

    # Demonstrate that the naive prefix-integrity check would be fooled:
    # the original bytes ARE still there unchanged.
    Then the offline-amended PDF preserves the compiled PDF bytes as a prefix

    # But /verify compares the FULL submitted bytes against a fresh compile.
    # CompilePDF(embeddedPayload) never emits the offline editor's annotation
    # object, so the comparison fails and the amendment is detected.
    When I verify the offline-amended PDF through /verify
    Then the response status is 409
