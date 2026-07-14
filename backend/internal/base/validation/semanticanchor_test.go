package validation

import (
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"
)

func TestEnforceCanonicalOntologyIRIsRejectsConflict(t *testing.T) {
	SetCanonicalOntologyIRIs(map[string]string{
		"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
		"odrl": "http://www.w3.org/ns/odrl/2/",
	})
	defer SetCanonicalOntologyIRIs(nil)

	conflicting := datatype.JSON([]byte(`{
		"@context": {"dcs": "https://evil.example/other-ontology#"},
		"dcs:documentStructure": {"@type": "dcs:DocumentStructure", "dcs:blocks": {"@list": []}, "dcs:layout": [{"@id": "urn:uuid:block-root", "dcs:isRoot": true, "dcs:children": {"@list": []}}]}
	}`))
	if _, err := NormalizeTemplateData(&conflicting); err == nil ||
		!strings.Contains(err.Error(), "Semantic Hub") {
		t.Fatalf("expected a Semantic Hub ontology-conflict rejection, got: %v", err)
	}

	// The canonical IRI passes, and an unknown extra prefix stays allowed.
	ok := datatype.JSON([]byte(`{
		"@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "ex": "https://example.org/ns#"},
		"dcs:documentStructure": {"@type": "dcs:DocumentStructure", "dcs:blocks": {"@list": []}, "dcs:layout": [{"@id": "urn:uuid:block-root", "dcs:isRoot": true, "dcs:children": {"@list": []}}]}
	}`))
	if _, err := NormalizeTemplateData(&ok); err != nil {
		t.Fatalf("expected the canonical context to pass, got: %v", err)
	}
}

func TestSchemaAnchorRefsArePointedAtTheHub(t *testing.T) {
	SetSchemaAnchorRefs(
		"http://dcs.local/semantic/context/facis-dcs?version=1",
		"https://w3id.org/facis/dcs/ontology/v1#",
		"http://dcs.local/semantic/shapes/facis-dcs?version=1",
	)
	defer SetSchemaAnchorRefs(SchemaJSONLDContextV1, SchemaOntologyV1, SchemaSHACLShapesV1)

	if schemaRefJSONLDContext != "http://dcs.local/semantic/context/facis-dcs?version=1" {
		t.Fatalf("expected the context schemaRef to be re-pointed, got %q", schemaRefJSONLDContext)
	}
	if schemaRefSHACLShapes != "http://dcs.local/semantic/shapes/facis-dcs?version=1" {
		t.Fatalf("expected the shapes schemaRef to be re-pointed, got %q", schemaRefSHACLShapes)
	}
}
