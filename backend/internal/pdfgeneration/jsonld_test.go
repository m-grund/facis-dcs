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
	name := "My Contract Template"
	titleIRI := testVocabIRI + "title"
	sectionsIRI := testVocabIRI + "sections"

	t.Run("expands compact terms to full IRIs and injects title", func(t *testing.T) {
		data := json.RawMessage(`{
			"@context": "https://w3id.org/facis/dcs/context/v1",
			"@type": "Document",
			"sections": []
		}`)
		out, err := MarshalJSONLD(data, &name)
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
		if _, hasCompact := doc["title"]; hasCompact {
			t.Error("compact key \"title\" must not appear; want full IRI")
		}
		if _, hasCompact := doc["sections"]; hasCompact {
			t.Error("compact key \"sections\" must not appear; want full IRI")
		}
		if doc[titleIRI] != name {
			t.Errorf("title IRI %q = %v, want %q", titleIRI, doc[titleIRI], name)
		}
		if _, hasSections := doc[sectionsIRI]; !hasSections {
			t.Errorf("sections IRI %q not found in expanded output", sectionsIRI)
		}
		// @type value must also be expanded.
		if doc["@type"] != testVocabIRI+"Document" {
			t.Errorf("@type = %v, want %q", doc["@type"], testVocabIRI+"Document")
		}
	})

	t.Run("omits title when name is nil", func(t *testing.T) {
		data := json.RawMessage(`{"@context":"https://w3id.org/facis/dcs/context/v1","sections":[]}`)
		out, err := MarshalJSONLD(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		json.Unmarshal(out, &doc)
		if _, ok := doc[titleIRI]; ok {
			t.Errorf("title IRI %q should be absent when name is nil", titleIRI)
		}
	})

}
