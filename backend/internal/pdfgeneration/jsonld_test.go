package pdfgeneration

import (
	"encoding/json"
	"testing"
)

func TestInjectTitle(t *testing.T) {
	name := "My Contract Template"

	t.Run("injects title into document missing it", func(t *testing.T) {
		input := []byte(`{"@context":"http://localhost:8080/ontology/dcs-pdf-core","sections":[]}`)
		out, err := InjectTitle(input, &name)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		if doc["title"] != name {
			t.Errorf("title = %v, want %q", doc["title"], name)
		}
	})

	t.Run("nil name leaves document unchanged", func(t *testing.T) {
		input := []byte(`{"@context":"http://localhost:8080/ontology/dcs-pdf-core"}`)
		out, err := InjectTitle(input, nil)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		json.Unmarshal(out, &doc)
		if _, ok := doc["title"]; ok {
			t.Error("title should not be injected when name is nil")
		}
	})
}
