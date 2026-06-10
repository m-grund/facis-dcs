package compiler

import (
	"bytes"
	"strings"
	"testing"
)

// TestInitOntologyIRISetsBaseURL verifies that initOntologyIRI correctly builds
// dcsCoreIRI and all derived model IRI slices from the given base URL.
func TestInitOntologyIRISetsBaseURL(t *testing.T) {
	initOntologyIRI("https://example.com")
	defer initOntologyIRI("") // restore default

	if dcsCoreIRI != "https://example.com/ontology/dcs-pdf-core#" {
		t.Errorf("dcsCoreIRI = %q, want %q", dcsCoreIRI, "https://example.com/ontology/dcs-pdf-core#")
	}
	want := "https://example.com/ontology/dcs-pdf-core#title"
	found := false
	for _, iri := range modelTitleIRIs {
		if iri == want {
			found = true
		}
	}
	if !found {
		t.Errorf("modelTitleIRIs does not contain %q: got %v", want, modelTitleIRIs)
	}
}

// TestInitOntologyIRIDefaultsTo127 verifies that an empty base URL defaults to
// http://127.0.0.1:8080, preserving the out-of-the-box developer experience.
func TestInitOntologyIRIDefaultsTo127(t *testing.T) {
	initOntologyIRI("")
	if !strings.HasPrefix(dcsCoreIRI, "http://127.0.0.1:8080/") {
		t.Errorf("empty base URL should default to 127.0.0.1:8080, got %q", dcsCoreIRI)
	}
}

// TestInitOntologyIRITrimsTrailingSlash verifies that a trailing slash in the
// base URL does not produce a double-slash in the derived IRI.
func TestInitOntologyIRITrimsTrailingSlash(t *testing.T) {
	initOntologyIRI("https://example.com/")
	defer initOntologyIRI("")

	if strings.Contains(dcsCoreIRI, "//ontology") {
		t.Errorf("dcsCoreIRI has double slash: %q", dcsCoreIRI)
	}
}

// TestTitleExtractedFromDcsCoreIRI verifies that the title property expressed
// under the dcs_pdf_core namespace (via @vocab) is extracted as the document
// title. This is the canonical form; dcterms:title is no longer accepted.
func TestTitleExtractedFromDcsCoreIRI(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "http://127.0.0.1:8080/ontology/dcs-pdf-core#",
			"dcs-pdf-core": "http://127.0.0.1:8080/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:title-iri-test",
		"@type": "dcs-pdf-core:Document",
		"title": "My Canonical Title",
		"sections": [
			{"@type": "dcs-pdf-core:Section", "heading": "S1", "clauses": ["Text."]}
		]
	}`)
	doc := mustExtractFromPayload(t, payload)
	if doc.Title != "My Canonical Title" {
		t.Errorf("title = %q, want %q", doc.Title, "My Canonical Title")
	}
	pdf := renderPDF(doc)
	if !bytes.Contains(pdf, []byte("My Canonical Title")) {
		t.Error("compiled PDF does not contain the expected title text")
	}
}

// TestPayloadWithConfiguredIRICompiles verifies that a payload whose @vocab
// matches a custom base URL (configured via initOntologyIRI) compiles
// successfully and the section is extracted.
func TestPayloadWithConfiguredIRICompiles(t *testing.T) {
	initOntologyIRI("https://custom.example.com")
	defer initOntologyIRI("")

	payload := []byte(`{
		"@context": {
			"@vocab": "https://custom.example.com/ontology/dcs-pdf-core#"
		},
		"@id": "urn:doc:custom-iri",
		"@type": "https://custom.example.com/ontology/dcs-pdf-core#Document",
		"title": "Custom IRI Document",
		"sections": [
			{
				"@type": "https://custom.example.com/ontology/dcs-pdf-core#Section",
				"heading": "Custom Heading",
				"clauses": ["Custom clause."]
			}
		]
	}`)
	doc := mustExtractFromPayload(t, payload)
	if len(doc.Sections) == 0 {
		t.Error("no sections extracted with custom IRI — initOntologyIRI did not take effect")
	}
	if doc.Title != "Custom IRI Document" {
		t.Errorf("title = %q, want %q", doc.Title, "Custom IRI Document")
	}
}
