package pdfgeneration

import (
	"encoding/json"
	"os"
	"testing"
)

const testVocabIRI = "http://localhost:8080/ontology/dcs-pdf-core#"

func TestMain(m *testing.M) {
	SetVocabIRI(testVocabIRI)
	os.Exit(m.Run())
}

func TestMarshalJSONLD(t *testing.T) {
	sectionsIRI := testVocabIRI + "sections"
	nameIRI := testVocabIRI + "name"

	t.Run("expands compact terms to full IRIs", func(t *testing.T) {
		data := json.RawMessage(`{
			"@context": "https://w3id.org/facis/dcs/context/v1",
			"@type": "Document",
			"name": "My Contract Template",
			"sections": []
		}`)
		out, err := MarshalJSONLD(data)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}

		if _, hasCtx := doc["@context"]; hasCtx {
			t.Error("@context must be dropped in expanded output")
		}
		if _, hasCompact := doc["sections"]; hasCompact {
			t.Error("compact key \"sections\" must not appear; want full IRI")
		}
		if _, hasSections := doc[sectionsIRI]; !hasSections {
			t.Errorf("sections IRI %q not found in expanded output", sectionsIRI)
		}
		if doc[nameIRI] != "My Contract Template" {
			t.Errorf("name IRI %q = %v, want %q", nameIRI, doc[nameIRI], "My Contract Template")
		}
		// @type value must also be expanded.
		if doc["@type"] != testVocabIRI+"Document" {
			t.Errorf("@type = %v, want %q", doc["@type"], testVocabIRI+"Document")
		}
	})

	t.Run("name already in jsonb is expanded to full IRI", func(t *testing.T) {
		data := json.RawMessage(`{"@context":"https://w3id.org/facis/dcs/context/v1","name":"From JSONB","sections":[]}`)
		out, err := MarshalJSONLD(data)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		json.Unmarshal(out, &doc)
		if doc[nameIRI] != "From JSONB" {
			t.Errorf("name IRI %q = %v, want %q", nameIRI, doc[nameIRI], "From JSONB")
		}
	})
}
