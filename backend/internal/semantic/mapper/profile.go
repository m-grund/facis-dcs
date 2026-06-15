package mapper

// OntologyProfile controls the JSON-LD envelope produced by BuildTemplateJSONLD
// and BuildContractJSONLD. It captures everything that varies between ontologies:
// which @context URI to reference, what semantic-profile metadata to emit, and
// which JSONB fields are promoted to the outer envelope level vs. kept nested.
type OntologyProfile struct {
	Name    string
	Version string

	// ContextURL is written into @context in every produced envelope.
	ContextURL  string
	OntologyURL string
	ShapesURL   string

	// TemplatePromotedFields lists JSONB keys that are extracted from the
	// inner template_data object and placed at the outer envelope level.
	TemplatePromotedFields map[string]bool

	// ContractPromotedFields lists JSONB keys that are extracted from the
	// inner contractData object and placed at the outer envelope level.
	ContractPromotedFields map[string]bool
}

// DefaultProfile returns the FACIS DCS v1 profile (SLA + semantic rules ontology).
// All fields match the constants in jsonld.go and the canonical example documents
// under docs/semantic-ontology/examples/.
func DefaultProfile() OntologyProfile {
	return OntologyProfile{
		Name:        semanticProfileName,
		Version:     semanticProfileV1,
		ContextURL:  jsonLDContextV1,
		OntologyURL: ontologyV1,
		ShapesURL:   shaclShapesV1,
		TemplatePromotedFields: map[string]bool{
			"sla":           true,
			"semanticRules": true,
			"policyBundle":  true,
			"provenance":    true,
			"contentHash":   true, // DCS-FR-CWE-04: content-hash sync between machine- and human-readable
		},
		ContractPromotedFields: map[string]bool{
			"parties":           true,
			"signatories":       true,
			"sla":               true,
			"semanticRules":     true,
			"policyBundle":      true,
			"validationReports": true,
			"clauses":           true,
			"contractVersions":  true,
			"adjustments":       true,
			"deployment":        true,
			"provenance":        true,
			"c2paManifest":      true,
			"statusCredential":  true,
			"contentHash":       true,
			"jurisdiction":      true, // DCS-FR-CSA-10: metadata indexing field
		},
	}
}
