package mapper

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Tests use the future w3id IRI as a stable fixture value.
	SetOntologyContextIRI("https://w3id.org/facis/dcs/context/v1")
	os.Exit(m.Run())
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newJSON(t *testing.T, v any) *datatype.JSON {
	t.Helper()
	j, err := datatype.NewJSON(v)
	require.NoError(t, err)
	return &j
}

func now() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

// minimalTemplateInner returns a pre-built inner JSONB that matches what
// NormalizeTemplateDataForPersistence would produce for the simplest valid template.
// It contains the outer envelope fields that normalization injects (these must be
// stripped by BuildTemplateJSONLD into the right layer).
func minimalTemplateInner(t *testing.T) *datatype.JSON {
	t.Helper()
	return newJSON(t, map[string]any{
		// Envelope fields added by normalization – mapper must strip/re-place these.
		"@context":        jsonLDContextV1,
		"@type":           "ContractTemplate",
		"@id":             "did:web:example:template:test",
		"did":             "did:web:example:template:test",
		"semanticProfile": standardSemanticProfile(),
		// Core document fields that stay in template_data.
		"templateDataVersion": 1,
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-1"}},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId":      "clause-1",
				"type":         "CLAUSE",
				"title":        "Service Scope",
				"text":         "Access to {{sc-svc.serviceName}}.",
				"conditionIds": []any{"sc-svc"},
			},
		},
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "sc-svc",
				"conditionName": "Service",
				"schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "serviceName",
						"type":          "string",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"customMetaData":       []any{map[string]any{"name": "contractType", "value": "SLA"}},
		"subTemplateSnapshots": []any{},
		"templateVariables":    []any{},
		"placeholderBindings": []any{
			map[string]any{
				"@type":            "PlaceholderBinding",
				"placeholder":      "{{sc-svc.serviceName}}",
				"boundToCondition": "sc-svc",
				"boundToParameter": "serviceName",
				"blockId":          "clause-1",
			},
		},
		"schemaRefs": map[string]any{"documentStructure": "facis.dcs.document-structure.v1"},
		"policyRefs": []any{},
		"validation": map[string]any{"schemaVersion": "v1"},
		// Promoted fields that should appear at the outer envelope level.
		"sla": map[string]any{
			"@type": "SLAAgreement",
			"services": []any{
				map[string]any{
					"@type":     "Service",
					"serviceId": "api",
					"slos": []any{
						map[string]any{
							"@type":    "SLO",
							"sloType":  "availability",
							"operator": "GreaterThanOrEqual",
						},
					},
				},
			},
		},
		"semanticRules": []any{
			map[string]any{
				"@type":        "ThresholdRule",
				"ruleId":       "rule-uptime",
				"leftOperand":  "{{sc-svc.serviceName}}",
				"operator":     "GreaterThanOrEqual",
				"rightOperand": 99.9,
				"valueType":    "decimal",
				"severity":     "blocking",
			},
		},
		"provenance": []any{
			map[string]any{
				"@type":      "ProvenanceEvent",
				"eventId":    "urn:uuid:aaa",
				"eventType":  "template.created",
				"actor":      "did:web:actor",
				"actorRole":  "TemplateManager",
				"occurredAt": "2026-05-21T10:00:00Z",
			},
		},
	})
}

// minimalContractInner returns a pre-built inner JSONB for a contract,
// including promoted fields (parties, sla, etc.) that were stored inline.
func minimalContractInner(t *testing.T) *datatype.JSON {
	t.Helper()
	return newJSON(t, map[string]any{
		// Envelope fields added by normalization.
		"@context":        jsonLDContextV1,
		"@type":           "Contract",
		"@id":             "did:web:example:contract:test",
		"did":             "did:web:example:contract:test",
		"semanticProfile": standardSemanticProfile(),
		// Core contract document fields.
		"templateDataVersion": 1,
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-1"}},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId":      "clause-1",
				"type":         "CLAUSE",
				"title":        "Service Scope",
				"text":         "Access to Best API.",
				"conditionIds": []any{"sc-svc"},
				"version":      1,
				"contentHash":  "sha256:abc",
			},
		},
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "sc-svc",
				"conditionName": "Service",
				"schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "serviceName",
						"type":          "string",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"semanticConditionValues": []any{
			map[string]any{
				"blockId":        "clause-1",
				"conditionId":    "sc-svc",
				"parameterName":  "serviceName",
				"parameterValue": "Best API",
			},
		},
		"subTemplateSnapshots": []any{},
		"placeholderBindings":  []any{},
		"schemaRefs":           map[string]any{"documentStructure": "facis.dcs.document-structure.v1"},
		"policyRefs":           []any{},
		"validation":           map[string]any{"schemaVersion": "v1"},
		"sourceTemplate": map[string]any{
			"did":     "did:web:example:template:test",
			"version": 1,
		},
		// Promoted fields.
		"parties": []any{
			map[string]any{
				"@type":      "CompanyParty",
				"identifier": "provider",
				"role":       "supplier",
				"name":       "Provider GmbH",
				"legalName":  "Provider GmbH",
			},
		},
		"signatories": []any{
			map[string]any{
				"@type":      "Signatory",
				"identifier": "signer-1",
				"name":       "Alice",
			},
		},
		"sla": map[string]any{
			"@type":    "SLAAgreement",
			"services": []any{},
		},
		"semanticRules": []any{
			map[string]any{
				"@type":    "ThresholdRule",
				"ruleId":   "rule-test",
				"operator": "GreaterThanOrEqual",
			},
		},
		"validationReports": []any{
			map[string]any{
				"@type":      "ValidationReport",
				"identifier": "report-1",
				"findings":   []any{},
				"source":     "runtime",
				"createdAt":  "2026-05-21T10:00:00Z",
			},
		},
		"clauses": []any{
			map[string]any{"@type": "Clause", "blockId": "clause-1", "clauseVersion": 1},
		},
		"contractVersions": []any{
			map[string]any{"@type": "ContractVersion", "contractVersion": 1, "contentHash": "sha256:v1"},
		},
		"adjustments":      []any{},
		"deployment":       map[string]any{"@type": "Deployment", "identifier": "dep-1"},
		"provenance":       []any{map[string]any{"@type": "ProvenanceEvent", "eventId": "urn:uuid:bbb"}},
		"c2paManifest":     map[string]any{"manifestUrl": "https://archive.example/manifest"},
		"statusCredential": map[string]any{"id": "urn:uuid:ccc"},
		"contentHash":      "sha256:contract-v1",
	})
}

func minimalTemplate(t *testing.T) templatedb.ContractTemplate {
	t.Helper()
	docNum := "TMPL-001"
	name := "Test Template"
	desc := "A test template"
	return templatedb.ContractTemplate{
		DID:            "did:web:dcs.example:template:test-template",
		DocumentNumber: &docNum,
		Version:        1,
		State:          "APPROVED",
		TemplateType:   "FRAME_CONTRACT",
		Name:           &name,
		Description:    &desc,
		CreatedBy:      "user-1",
		CreatedAt:      now(),
		UpdatedAt:      now(),
		TemplateData:   minimalTemplateInner(t),
	}
}

func minimalContract(t *testing.T) contractdb.Contract {
	t.Helper()
	name := "Test Contract"
	desc := "A test contract"
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	exp := time.Date(2027, 5, 31, 23, 59, 59, 0, time.UTC)
	return contractdb.Contract{
		DID:             "did:web:dcs.example:contract:test-contract",
		ContractVersion: 1,
		State:           "DRAFT",
		CreatedBy:       "user-1",
		CreatedAt:       now(),
		UpdatedAt:       now(),
		StartDate:       &start,
		ExpDate:         &exp,
		Name:            &name,
		Description:     &desc,
		ContractData:    minimalContractInner(t),
	}
}

// ── BuildTemplateJSONLD ───────────────────────────────────────────────────────

func TestBuildTemplateJSONLD_EnvelopeFields(t *testing.T) {
	tmpl := minimalTemplate(t)
	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	assert.Equal(t, jsonLDContextV1, env["@context"])
	assert.Equal(t, tmpl.DID, env["@id"])
	assert.Equal(t, "ContractTemplate", env["@type"])
	assert.Equal(t, tmpl.DID, env["did"])
	assert.Equal(t, tmpl.Version, env["templateVersion"])
	assert.Equal(t, "v1", env["schemaVersion"])
	assert.Equal(t, "TMPL-001", env["documentNumber"])
	assert.Equal(t, "Test Template", env["name"])
	assert.Equal(t, "A test template", env["description"])
	assert.Equal(t, "2026-05-21T10:00:00Z", env["createdAt"])
	assert.Equal(t, "2026-05-21T10:00:00Z", env["updatedAt"])

	profile, ok := env["semanticProfile"].(map[string]any)
	require.True(t, ok, "semanticProfile must be a map")
	assert.Equal(t, semanticProfileName, profile["name"])
	assert.Equal(t, semanticProfileV1, profile["version"])
	assert.Equal(t, jsonLDContextV1, profile["context"])
}

func TestBuildTemplateJSONLD_OptionalFieldsOmitted(t *testing.T) {
	tmpl := minimalTemplate(t)
	tmpl.DocumentNumber = nil
	tmpl.Name = nil
	tmpl.Description = nil
	tmpl.DID = "did:web:dcs.example:template:no-uuid-here"

	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	_, hasDocNum := env["documentNumber"]
	assert.False(t, hasDocNum, "documentNumber should be absent when nil")
	_, hasName := env["name"]
	assert.False(t, hasName, "name should be absent when nil")
	_, hasDesc := env["description"]
	assert.False(t, hasDesc, "description should be absent when nil")
	_, hasUUID := env["uuid"]
	assert.False(t, hasUUID, "uuid should be absent when DID has no UUID segment")
}

func TestBuildTemplateJSONLD_UUIDFromDID(t *testing.T) {
	tmpl := minimalTemplate(t)
	tmpl.DID = "did:web:dcs.example:template:5c6152f8-f54d-4474-a58c-1fd3f651fe0d"

	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	assert.Equal(t, "urn:uuid:5c6152f8-f54d-4474-a58c-1fd3f651fe0d", env["uuid"])
}

func TestBuildTemplateJSONLD_PromotedFieldsAtTopLevel(t *testing.T) {
	tmpl := minimalTemplate(t)
	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	// sla, semanticRules, provenance must be promoted to envelope level.
	_, hasSLA := env["sla"]
	assert.True(t, hasSLA, "sla must appear at top level")
	_, hasRules := env["semanticRules"]
	assert.True(t, hasRules, "semanticRules must appear at top level")
	_, hasProv := env["provenance"]
	assert.True(t, hasProv, "provenance must appear at top level")

	// Verify values were not lost.
	sla, _ := env["sla"].(map[string]any)
	assert.Equal(t, "SLAAgreement", sla["@type"])
}

func TestBuildTemplateJSONLD_CoreFieldsInTemplateData(t *testing.T) {
	tmpl := minimalTemplate(t)
	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	td, ok := env["template_data"].(map[string]any)
	require.True(t, ok, "template_data must be a map")

	// All DCS-required fields for full template reconstruction.
	assert.Contains(t, td, "templateDataVersion", "templateDataVersion must be in template_data")
	assert.Contains(t, td, "documentOutline", "documentOutline must be in template_data")
	assert.Contains(t, td, "documentBlocks", "documentBlocks must be in template_data")
	assert.Contains(t, td, "semanticConditions", "semanticConditions must be in template_data")
	assert.Contains(t, td, "customMetaData", "customMetaData must be in template_data")
	assert.Contains(t, td, "subTemplateSnapshots", "subTemplateSnapshots must be in template_data")
	assert.Contains(t, td, "placeholderBindings", "placeholderBindings must be in template_data")
	assert.Contains(t, td, "schemaRefs", "schemaRefs must be in template_data")
	assert.Contains(t, td, "policyRefs", "policyRefs must be in template_data")
	assert.Contains(t, td, "validation", "validation must be in template_data")
}

func TestBuildTemplateJSONLD_StaleEnvelopeFieldsNotLeaked(t *testing.T) {
	// The inner JSONB already contains @context, @type, @id, did, semanticProfile
	// added by normalization. These must NOT appear inside template_data.
	tmpl := minimalTemplate(t)
	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	td := env["template_data"].(map[string]any)
	assert.NotContains(t, td, "@context", "@context must not be in template_data")
	assert.NotContains(t, td, "@id", "@id must not be in template_data")
	assert.NotContains(t, td, "@type", "@type must not be in template_data")
	assert.NotContains(t, td, "did", "did must not be in template_data")
	assert.NotContains(t, td, "semanticProfile", "semanticProfile must not be in template_data")
	// Promoted fields must not appear in template_data either.
	assert.NotContains(t, td, "sla", "sla must not be in template_data")
	assert.NotContains(t, td, "semanticRules", "semanticRules must not be in template_data")
	assert.NotContains(t, td, "provenance", "provenance must not be in template_data")
}

func TestBuildTemplateJSONLD_NilTemplateData(t *testing.T) {
	tmpl := minimalTemplate(t)
	tmpl.TemplateData = nil

	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	td, ok := env["template_data"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, td, "template_data should be empty when TemplateData is nil")
}

// TestBuildTemplateJSONLD_CustomProfile verifies that a non-default profile
// changes @context, semanticProfile, and the set of promoted fields.
func TestBuildTemplateJSONLD_CustomProfile(t *testing.T) {
	supplyChainProfile := OntologyProfile{
		Name:        "FACIS DCS Supply Chain Profile",
		Version:     "v2",
		ContextURL:  "https://w3id.org/facis/dcs/context/supply-chain/v2",
		OntologyURL: "https://w3id.org/facis/dcs/ontology/supply-chain/v2",
		ShapesURL:   "https://w3id.org/facis/dcs/shapes/supply-chain/v2",
		TemplatePromotedFields: map[string]bool{
			"deliveryTerms":    true,
			"qualityStandards": true,
		},
		ContractPromotedFields: map[string]bool{},
	}

	inner := newJSON(t, map[string]any{
		"documentOutline":    []any{},
		"documentBlocks":     []any{},
		"semanticConditions": []any{},
		"deliveryTerms":      map[string]any{"incoterms": "CIF", "destination": "Hamburg"},
		"qualityStandards":   []any{"ISO9001"},
		"sla":                map[string]any{"@type": "SLAAgreement"}, // not in this profile
	})

	tmpl := minimalTemplate(t)
	tmpl.TemplateData = inner

	env, err := BuildTemplateJSONLD(tmpl, supplyChainProfile)
	require.NoError(t, err)

	// @context and semanticProfile must reflect the custom profile.
	assert.Equal(t, "https://w3id.org/facis/dcs/context/supply-chain/v2", env["@context"])
	sp, ok := env["semanticProfile"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "FACIS DCS Supply Chain Profile", sp["name"])
	assert.Equal(t, "v2", sp["version"])
	assert.Equal(t, "https://w3id.org/facis/dcs/ontology/supply-chain/v2", sp["ontology"])

	// Custom promoted fields are at top level.
	assert.Contains(t, env, "deliveryTerms")
	assert.Contains(t, env, "qualityStandards")

	// sla is not in this profile's promoted set → must stay in template_data.
	_, hasSLA := env["sla"]
	assert.False(t, hasSLA, "sla must not be promoted for supply chain profile")
	td, _ := env["template_data"].(map[string]any)
	assert.Contains(t, td, "sla", "sla must remain in template_data for supply chain profile")
}

// ── BuildContractJSONLD ───────────────────────────────────────────────────────

func TestBuildContractJSONLD_EnvelopeFields(t *testing.T) {
	contract := minimalContract(t)
	tmpl := minimalTemplate(t)
	env, err := BuildContractJSONLD(contract, tmpl, DefaultProfile())
	require.NoError(t, err)

	assert.Equal(t, jsonLDContextV1, env["@context"])
	assert.Equal(t, contract.DID, env["@id"])
	assert.Equal(t, "Contract", env["@type"])
	assert.Equal(t, contract.DID, env["did"])
	assert.Equal(t, contract.ContractVersion, env["contractVersion"])
	assert.Equal(t, "DRAFT", env["state"])
	assert.Equal(t, "Draft", env["lifecycleState"])
	assert.Equal(t, "2026-05-21T10:00:00Z", env["createdAt"])
	assert.Equal(t, "2026-05-21T10:00:00Z", env["updatedAt"])
	assert.Equal(t, "2026-06-01T00:00:00Z", env["validFrom"])
	assert.Equal(t, "2027-05-31T23:59:59Z", env["validUntil"])
	assert.Equal(t, tmpl.DID, env["derivedFromTemplate"])
	assert.Equal(t, tmpl.Version, env["templateVersion"])
	assert.Equal(t, "Test Contract", env["name"])
	assert.Equal(t, "A test contract", env["description"])
}

func TestBuildContractJSONLD_ValidFrom_ValidUntilAbsentWhenNil(t *testing.T) {
	contract := minimalContract(t)
	contract.StartDate = nil
	contract.ExpDate = nil

	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	_, hasVF := env["validFrom"]
	assert.False(t, hasVF, "validFrom must be absent when StartDate is nil")
	_, hasVU := env["validUntil"]
	assert.False(t, hasVU, "validUntil must be absent when ExpDate is nil")
}

func TestBuildContractJSONLD_LifecycleStateMapping(t *testing.T) {
	cases := []struct {
		dbState  string
		expected string
	}{
		{"DRAFT", "Draft"},
		{"NEGOTIATION", "InNegotiation"},
		{"SUBMITTED", "SubmittedForReview"},
		{"REVIEWED", "Reviewed"},
		{"APPROVED", "Approved"},
		{"TERMINATED", "Terminated"},
		{"EXPIRED", "Expired"},
		{"UNKNOWN_STATE", "UNKNOWN_STATE"}, // passthrough for unknown values
	}

	for _, tc := range cases {
		t.Run(tc.dbState, func(t *testing.T) {
			assert.Equal(t, tc.expected, semanticLifecycleState(tc.dbState))
		})
	}
}

func TestBuildContractJSONLD_PromotedFieldsAtTopLevel(t *testing.T) {
	contract := minimalContract(t)
	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	promoted := []string{
		"parties", "signatories", "sla", "semanticRules", "validationReports",
		"clauses", "contractVersions", "adjustments", "deployment",
		"provenance", "c2paManifest", "statusCredential", "contentHash",
		// DCS-FR-CSA-10: jurisdiction promoted for metadata indexing
		// (only asserted when present in JSONB; minimalContractInner doesn't include it)
	}
	for _, field := range promoted {
		assert.Contains(t, env, field, "promoted field %q must be at top level", field)
	}

	parties, _ := env["parties"].([]any)
	require.Len(t, parties, 1)
	party := parties[0].(map[string]any)
	assert.Equal(t, "Provider GmbH", party["name"])
}

func TestBuildContractJSONLD_CoreFieldsInContractData(t *testing.T) {
	contract := minimalContract(t)
	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	cd, ok := env["contractData"].(map[string]any)
	require.True(t, ok, "contractData must be a map")

	// Fields required for DCS contract reconstruction.
	assert.Contains(t, cd, "templateDataVersion")
	assert.Contains(t, cd, "documentOutline")
	assert.Contains(t, cd, "documentBlocks")
	assert.Contains(t, cd, "semanticConditions")
	assert.Contains(t, cd, "semanticConditionValues")
	assert.Contains(t, cd, "subTemplateSnapshots")
	assert.Contains(t, cd, "placeholderBindings")
	assert.Contains(t, cd, "schemaRefs")
	assert.Contains(t, cd, "policyRefs")
	assert.Contains(t, cd, "validation")
	assert.Contains(t, cd, "sourceTemplate")
}

func TestBuildContractJSONLD_PromotedFieldsAbsentFromContractData(t *testing.T) {
	contract := minimalContract(t)
	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	cd := env["contractData"].(map[string]any)
	promoted := []string{
		"parties", "signatories", "sla", "semanticRules", "validationReports",
		"clauses", "contractVersions", "adjustments", "deployment",
		"provenance", "c2paManifest", "statusCredential", "contentHash",
		"@context", "@id", "@type", "did", "semanticProfile",
	}
	for _, field := range promoted {
		assert.NotContains(t, cd, field, "field %q must not appear in contractData", field)
	}
}

func TestBuildContractJSONLD_JurisdictionPromoted(t *testing.T) {
	inner := newJSON(t, map[string]any{
		"documentOutline":         []any{},
		"documentBlocks":          []any{},
		"semanticConditions":      []any{},
		"semanticConditionValues": []any{},
		"jurisdiction":            "DEU",
	})
	contract := minimalContract(t)
	contract.ContractData = inner

	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	assert.Equal(t, "DEU", env["jurisdiction"], "jurisdiction must be promoted to top level (DCS-FR-CSA-10)")
	cd, _ := env["contractData"].(map[string]any)
	assert.NotContains(t, cd, "jurisdiction", "jurisdiction must not remain in contractData")
}

func TestBuildTemplateJSONLD_ContentHashPromoted(t *testing.T) {
	inner := newJSON(t, map[string]any{
		"documentOutline":    []any{},
		"documentBlocks":     []any{},
		"semanticConditions": []any{},
		"contentHash":        "sha256:tmpl-v1",
	})
	tmpl := minimalTemplate(t)
	tmpl.TemplateData = inner

	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	assert.Equal(t, "sha256:tmpl-v1", env["contentHash"], "contentHash must be promoted to top level (DCS-FR-CWE-04)")
	td, _ := env["template_data"].(map[string]any)
	assert.NotContains(t, td, "contentHash", "contentHash must not remain in template_data")
}

func TestBuildContractJSONLD_EmptyContractData(t *testing.T) {
	contract := minimalContract(t)
	contract.ContractData = nil

	env, err := BuildContractJSONLD(contract, minimalTemplate(t), DefaultProfile())
	require.NoError(t, err)

	cd, ok := env["contractData"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, cd, "contractData should be empty when ContractData is nil")
}

// TestBuildContractJSONLD_CustomProfile verifies that a non-default profile
// changes @context, semanticProfile, and the set of promoted fields.
func TestBuildContractJSONLD_CustomProfile(t *testing.T) {
	paymentProfile := OntologyProfile{
		Name:        "FACIS DCS Payment Profile",
		Version:     "v1",
		ContextURL:  "https://w3id.org/facis/dcs/context/payment/v1",
		OntologyURL: "https://w3id.org/facis/dcs/ontology/payment/v1",
		ShapesURL:   "https://w3id.org/facis/dcs/shapes/payment/v1",
		TemplatePromotedFields: map[string]bool{
			"paymentTerms": true,
		},
		ContractPromotedFields: map[string]bool{
			"parties":      true,
			"paymentTerms": true,
			"invoices":     true,
		},
	}

	inner := newJSON(t, map[string]any{
		"documentOutline":         []any{},
		"documentBlocks":          []any{},
		"semanticConditions":      []any{},
		"semanticConditionValues": []any{},
		"parties":                 []any{map[string]any{"@type": "CompanyParty", "role": "supplier"}},
		"paymentTerms":            map[string]any{"currency": "EUR", "dueDays": 30},
		"invoices":                []any{map[string]any{"invoiceId": "INV-001"}},
		"sla":                     map[string]any{"@type": "SLAAgreement"}, // not in payment profile
	})

	contract := minimalContract(t)
	contract.ContractData = inner

	env, err := BuildContractJSONLD(contract, minimalTemplate(t), paymentProfile)
	require.NoError(t, err)

	// @context and semanticProfile must reflect the custom profile.
	assert.Equal(t, "https://w3id.org/facis/dcs/context/payment/v1", env["@context"])
	sp, ok := env["semanticProfile"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "FACIS DCS Payment Profile", sp["name"])
	assert.Equal(t, "https://w3id.org/facis/dcs/context/payment/v1", sp["context"])
	assert.Equal(t, "https://w3id.org/facis/dcs/ontology/payment/v1", sp["ontology"])

	// Custom promoted fields appear at top level.
	assert.Contains(t, env, "paymentTerms")
	assert.Contains(t, env, "invoices")
	assert.Contains(t, env, "parties") // explicitly in payment profile's promoted set

	// sla is not in this profile's promoted set → must stay in contractData.
	_, hasSLA := env["sla"]
	assert.False(t, hasSLA, "sla must not be promoted for payment profile")
	cd, _ := env["contractData"].(map[string]any)
	assert.Contains(t, cd, "sla", "sla must remain in contractData for payment profile")

	// Standard SLA promoted fields must NOT appear (not in this profile).
	_, hasSemanticRules := env["semanticRules"]
	assert.False(t, hasSemanticRules, "semanticRules must not be promoted for payment profile")
}

// ── Roundtrip: template → contract, verify reconstructibility ────────────────

// TestRoundtrip_DocumentStructurePreservation verifies that the JSON-LD envelope
// produced by BuildTemplateJSONLD and BuildContractJSONLD contains all fields
// required to fully reconstruct the human-readable DCS contract in the frontend.
func TestRoundtrip_DocumentStructurePreservation(t *testing.T) {
	tmpl := minimalTemplate(t)
	templateEnv, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	// --- Template side ---
	// Requirement: template_data must carry all structural fields.
	td := templateEnv["template_data"].(map[string]any)

	outline, _ := td["documentOutline"].([]any)
	require.Len(t, outline, 1)
	root := outline[0].(map[string]any)
	assert.Equal(t, "root", root["blockId"])
	assert.Equal(t, true, root["isRoot"])

	blocks, _ := td["documentBlocks"].([]any)
	require.Len(t, blocks, 1)
	block := blocks[0].(map[string]any)
	assert.Equal(t, "clause-1", block["blockId"])
	assert.Equal(t, "CLAUSE", block["type"])

	conditions, _ := td["semanticConditions"].([]any)
	require.Len(t, conditions, 1)
	cond := conditions[0].(map[string]any)
	assert.Equal(t, "sc-svc", cond["conditionId"])

	// --- Contract side ---
	contract := minimalContract(t)
	contractEnv, err := BuildContractJSONLD(contract, tmpl, DefaultProfile())
	require.NoError(t, err)

	cd := contractEnv["contractData"].(map[string]any)

	// Contract version of the document structure must be intact.
	contractOutline, _ := cd["documentOutline"].([]any)
	require.Len(t, contractOutline, 1)

	contractBlocks, _ := cd["documentBlocks"].([]any)
	require.Len(t, contractBlocks, 1)
	contractBlock := contractBlocks[0].(map[string]any)
	assert.Equal(t, "clause-1", contractBlock["blockId"])

	contractConditions, _ := cd["semanticConditions"].([]any)
	require.Len(t, contractConditions, 1)

	scvs, _ := cd["semanticConditionValues"].([]any)
	require.Len(t, scvs, 1)
	scv := scvs[0].(map[string]any)
	assert.Equal(t, "clause-1", scv["blockId"])
	assert.Equal(t, "sc-svc", scv["conditionId"])
	assert.Equal(t, "serviceName", scv["parameterName"])
	assert.Equal(t, "Best API", scv["parameterValue"])

	// Template traceability.
	assert.Equal(t, tmpl.DID, contractEnv["derivedFromTemplate"])
	assert.Equal(t, tmpl.Version, contractEnv["templateVersion"])
}

// ── SLA example structure ─────────────────────────────────────────────────────

// TestSLAExampleStructure verifies that a template modeled after the canonical
// sla-template.jsonld example (docs/semantic-ontology/examples/sla-template.jsonld)
// produces a correctly structured envelope.
func TestSLAExampleStructure(t *testing.T) {
	slaInner := newJSON(t, map[string]any{
		"templateDataVersion": 1,
		"documentOutline": []any{
			map[string]any{
				"blockId": "section-root",
				"isRoot":  true,
				"children": []any{
					"clause-availability", "clause-access-policy", "clause-remedy",
				},
			},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId":      "clause-availability",
				"type":         "CLAUSE",
				"title":        "Availability Objective",
				"text":         "Monthly uptime must be at least {{sc-uptime.uptimePercentage}} percent.",
				"conditionIds": []any{"sc-uptime"},
			},
			map[string]any{
				"blockId":      "clause-access-policy",
				"type":         "CLAUSE",
				"title":        "Access Policy",
				"text":         "Access limited to {{sc-country.organizationCountry}} until {{sc-access-window.accessUntil}}.",
				"conditionIds": []any{"sc-country", "sc-access-window"},
			},
			map[string]any{
				"blockId":      "clause-remedy",
				"type":         "CLAUSE",
				"title":        "Remedy",
				"text":         "Service credit of {{sc-remedy.creditPercentage}} percent.",
				"conditionIds": []any{"sc-remedy"},
			},
		},
		"semanticConditions": []any{
			map[string]any{
				"@type": "SemanticCondition", "conditionId": "sc-uptime",
				"conditionName": "Minimum monthly uptime", "schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "uptimePercentage",
						"type":          "decimal", "isRequired": true, "defaultValue": 99.95,
						"operators": []any{
							map[string]any{"operate": "greaterThanOrEqual", "targets": []any{"99.95"}},
						},
					},
				},
			},
			map[string]any{
				"@type": "SemanticCondition", "conditionId": "sc-country",
				"conditionName": "Customer country", "schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "organizationCountry",
						"type":          "string", "isRequired": true, "defaultValue": "DE",
						"operators": []any{
							map[string]any{"operate": "equal", "targets": []any{"DE"}},
						},
					},
				},
			},
			map[string]any{
				"@type": "SemanticCondition", "conditionId": "sc-access-window",
				"conditionName": "Access window", "schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "accessUntil",
						"type":          "date", "isRequired": true,
						"operators": []any{
							map[string]any{"operate": "lessThanOrEqual", "targets": []any{"{{contractEndDate}}"}},
						},
					},
				},
			},
			map[string]any{
				"@type": "SemanticCondition", "conditionId": "sc-remedy",
				"conditionName": "Service credit", "schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "creditPercentage",
						"type":          "decimal", "isRequired": true, "defaultValue": 10,
						"operators": []any{
							map[string]any{"operate": "between", "targets": []any{"0", "100"}},
						},
					},
				},
			},
		},
		"customMetaData": []any{
			map[string]any{"name": "contractType", "value": "SLA"},
		},
		"subTemplateSnapshots": []any{},
		"templateVariables": []any{
			map[string]any{
				"@type": "TemplateVariable", "parameterName": "contractEndDate",
				"parameterType": "date", "required": true,
			},
		},
		"placeholderBindings": []any{
			map[string]any{
				"@type":            "PlaceholderBinding",
				"placeholder":      "{{sc-uptime.uptimePercentage}}",
				"boundToCondition": "sc-uptime", "boundToParameter": "uptimePercentage",
				"blockId": "clause-availability",
			},
		},
		// Promoted fields.
		"sla": map[string]any{
			"@type": "SLAAgreement",
			"services": []any{
				map[string]any{
					"@type": "Service", "serviceId": "showtimes-api", "name": "Showtimes API",
					"slos": []any{
						map[string]any{
							"@type": "SLO", "sloType": "availability",
							"targetValue": 99.95, "unit": "percent",
							"operator": "GreaterThanOrEqual", "measurementWindow": "P1M",
						},
					},
				},
			},
		},
		"semanticRules": []any{
			map[string]any{
				"@type": "ThresholdRule", "ruleId": "rule-uptime-minimum",
				"leftOperand": "{{sc-uptime.uptimePercentage}}",
				"operator":    "GreaterThanOrEqual", "rightOperand": 99.95,
				"valueType": "decimal", "severity": "blocking",
			},
		},
	})

	docNum := "FACIS-SLA-API-ACCESS"
	name := "API Access SLA Template"
	tmpl := templatedb.ContractTemplate{
		DID:            "did:web:dcs.example:template:sla-api-access-v1",
		DocumentNumber: &docNum,
		Version:        1,
		State:          "APPROVED",
		TemplateType:   "FRAME_CONTRACT",
		Name:           &name,
		CreatedBy:      "user",
		CreatedAt:      now(),
		UpdatedAt:      now(),
		TemplateData:   slaInner,
	}

	env, err := BuildTemplateJSONLD(tmpl, DefaultProfile())
	require.NoError(t, err)

	// Top-level fields.
	assert.Equal(t, "ContractTemplate", env["@type"])
	assert.Equal(t, "did:web:dcs.example:template:sla-api-access-v1", env["did"])
	assert.Equal(t, "FACIS-SLA-API-ACCESS", env["documentNumber"])
	assert.Equal(t, "API Access SLA Template", env["name"])

	// SLA at top level.
	sla, ok := env["sla"].(map[string]any)
	require.True(t, ok)
	services, _ := sla["services"].([]any)
	require.Len(t, services, 1)
	service := services[0].(map[string]any)
	assert.Equal(t, "showtimes-api", service["serviceId"])

	// template_data contains all clause and condition structure.
	td := env["template_data"].(map[string]any)
	blocks, _ := td["documentBlocks"].([]any)
	assert.Len(t, blocks, 3, "all three clause blocks must be in template_data")

	conditions, _ := td["semanticConditions"].([]any)
	assert.Len(t, conditions, 4, "all four semantic conditions must be in template_data")

	tvs, _ := td["templateVariables"].([]any)
	assert.Len(t, tvs, 1, "templateVariables must be in template_data")

	// Promoted fields must NOT be duplicated inside template_data.
	assert.NotContains(t, td, "sla")
	assert.NotContains(t, td, "semanticRules")
}

// ── canonicalizeOperator ──────────────────────────────────────────────────────

func TestCanonicalizeLegacyOperator_AllMappings(t *testing.T) {
	cases := []struct {
		legacy    string
		canonical string
	}{
		{"equal", "Equals"},
		{"notEqual", "NotEquals"},
		{"greaterThan", "GreaterThan"},
		{"greaterThanOrEqual", "GreaterThanOrEqual"},
		{"lessThan", "LessThan"},
		{"lessThanOrEqual", "LessThanOrEqual"},
		{"between", "Between"},
		{"contains", "Contains"},
		{"matchesRegex", "MatchesRegex"},
	}
	for _, tc := range cases {
		t.Run(tc.legacy, func(t *testing.T) {
			assert.Equal(t, tc.canonical, canonicalizeOperator(tc.legacy))
		})
	}
}

func TestCanonicalizeLegacyOperator_AlreadyCanonical(t *testing.T) {
	canonical := []string{"Equals", "NotEquals", "GreaterThan", "GreaterThanOrEqual",
		"LessThan", "LessThanOrEqual", "Between", "Contains", "MatchesRegex"}
	for _, op := range canonical {
		assert.Equal(t, op, canonicalizeOperator(op), "canonical operator must be unchanged: %s", op)
	}
}

func TestCanonicalizeLegacyOperator_Unknown(t *testing.T) {
	assert.Equal(t, "", canonicalizeOperator("unknown"))
	assert.Equal(t, "", canonicalizeOperator(""))
	assert.Equal(t, "", canonicalizeOperator("EQUALS"))
}

// ── normalizeSLAOperators ─────────────────────────────────────────────────────

func TestNormalizeSLAOperators_LegacyOperatorsNormalized(t *testing.T) {
	data := map[string]any{
		"sla": map[string]any{
			"@type": "SLAAgreement",
			"services": []any{
				map[string]any{
					"@type": "Service", "serviceId": "api",
					"slos": []any{
						map[string]any{
							"@type":    "SLO",
							"operator": "greaterThanOrEqual", // legacy
							"measurementRules": []any{
								map[string]any{
									"@type":    "MeasurementRule",
									"operator": "lessThan", // legacy
								},
							},
						},
						map[string]any{
							"@type":    "SLO",
							"operator": "equal", // legacy
						},
					},
				},
			},
		},
	}

	normalizeSLAOperators(data)

	sla := data["sla"].(map[string]any)
	services := sla["services"].([]any)
	service := services[0].(map[string]any)
	slos := service["slos"].([]any)

	slo0 := slos[0].(map[string]any)
	assert.Equal(t, "GreaterThanOrEqual", slo0["operator"], "SLO operator must be canonical")

	rules := slo0["measurementRules"].([]any)
	rule := rules[0].(map[string]any)
	assert.Equal(t, "LessThan", rule["operator"], "MeasurementRule operator must be canonical")

	slo1 := slos[1].(map[string]any)
	assert.Equal(t, "Equals", slo1["operator"])
}

func TestNormalizeSLAOperators_AlreadyCanonicalUnchanged(t *testing.T) {
	data := map[string]any{
		"sla": map[string]any{
			"services": []any{
				map[string]any{
					"slos": []any{
						map[string]any{"operator": "GreaterThanOrEqual"},
					},
				},
			},
		},
	}

	normalizeSLAOperators(data)

	slo := data["sla"].(map[string]any)["services"].([]any)[0].(map[string]any)["slos"].([]any)[0].(map[string]any)
	assert.Equal(t, "GreaterThanOrEqual", slo["operator"])
}

func TestNormalizeSLAOperators_NoSLAKey(t *testing.T) {
	// Must not panic when sla is absent.
	data := map[string]any{"other": "value"}
	assert.NotPanics(t, func() { normalizeSLAOperators(data) })
}

// ── NormalizeSemanticTemplateData integration test ────────────────────────────

// TestNormalizeSemanticTemplateData_SLALegacyOperators verifies that calling
// NormalizeSemanticTemplateData on a valid template payload normalizes legacy
// SLA operators without failing the base structural validation.
func TestNormalizeSemanticTemplateData_SLALegacyOperators(t *testing.T) {
	// Minimal valid template data that passes the base normalization:
	// one TEXT block (no conditions required), empty semanticConditions.
	raw := newJSON(t, map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"intro"}},
		},
		"documentBlocks": []any{
			map[string]any{"blockId": "intro", "type": "TEXT", "text": "Introduction."},
		},
		"semanticConditions": []any{},
		"customMetaData":     []any{},
		// SLA block with legacy operators — should be normalized.
		"sla": map[string]any{
			"@type": "SLAAgreement",
			"services": []any{
				map[string]any{
					"@type": "Service", "serviceId": "api",
					"slos": []any{
						map[string]any{
							"@type":    "SLO",
							"sloType":  "availability",
							"operator": "greaterThanOrEqual", // legacy
							"measurementRules": []any{
								map[string]any{
									"@type":    "MeasurementRule",
									"operator": "greaterThanOrEqual", // legacy
								},
							},
						},
					},
				},
			},
		},
	})

	normalized, err := NormalizeSemanticTemplateData(raw, "did:web:example:template:sla-test")
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &decoded))

	// Base normalization must have run: @context and @type injected.
	assert.Equal(t, jsonLDContextV1, decoded["@context"])
	assert.Equal(t, "ContractTemplate", decoded["@type"])

	// SLA operators must be canonical.
	sla := decoded["sla"].(map[string]any)
	services := sla["services"].([]any)
	service := services[0].(map[string]any)
	slos := service["slos"].([]any)
	slo := slos[0].(map[string]any)
	assert.Equal(t, "GreaterThanOrEqual", slo["operator"], "SLO operator must be canonicalized")

	rules := slo["measurementRules"].([]any)
	rule := rules[0].(map[string]any)
	assert.Equal(t, "GreaterThanOrEqual", rule["operator"], "MeasurementRule operator must be canonicalized")
}

// ── uuidURNFromDID ────────────────────────────────────────────────────────────

func TestUUIDURNFromDID(t *testing.T) {
	cases := []struct {
		did      string
		expected string
	}{
		{
			"did:web:dcs.example:contract:2d1bf6cf-cc6f-47d1-a91b-e03c3da1e7d1",
			"urn:uuid:2d1bf6cf-cc6f-47d1-a91b-e03c3da1e7d1",
		},
		{
			"did:web:dcs.example:template:sla-api-access-v1",
			"", // no UUID in DID
		},
		{
			"did:web:dcs.example:template:5C6152F8-F54D-4474-A58C-1FD3F651FE0D", // uppercase
			"urn:uuid:5c6152f8-f54d-4474-a58c-1fd3f651fe0d",
		},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.did, func(t *testing.T) {
			assert.Equal(t, tc.expected, uuidURNFromDID(tc.did))
		})
	}
}
