package validation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// ADR-8/ADR-9: AuditContractContent's SHACL/profile enforcement reads
	// from the Semantic Hub only (no disk fallback) — tests install a
	// ShapeSource fixture backed by the real hub authoring files so the
	// real goRDFlib SHACL engine runs end to end without a live database
	// (see contractcontentaudit_test.go).
	SetShapeSource(fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl"),
		profileYAML: mustReadRepoFile("docs/semantic-ontology/validation/facis.sla.basic.v1.yaml"),
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
	})
	os.Exit(m.Run())
}

func validTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()
	data, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-1"}},
		},
		"documentBlocks": []any{
			map[string]any{"blockId": "clause-1", "type": "CLAUSE", "text": "Availability {{cond-1.percent}}", "conditionIds": []any{"cond-1"}},
		},
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "cond-1",
				"conditionName": "Availability",
				"schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "percent",
						"type":          "decimal",
						"schemaRef":     SchemaServiceV1,
						"semanticPath":  "service.sla.availability",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)
	return &data
}

func canonicalTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()
	data, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:title":        "Provider eligibility",
			"dcs:templateType": "dcs:Component",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "urn:uuid:block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Provider country: ",
						map[string]any{
							"@type":       "dcs:Placeholder",
							"dcs:token":   "{{provider.country}}",
							"dcs:bindsTo": map[string]any{"@id": "urn:uuid:field-provider-country"},
						},
					}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":          "urn:uuid:block-root",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "urn:uuid:block-clause-1"}}},
				},
			},
		},
		"dcs:contractData": []any{
			map[string]any{
				"@id":               "urn:uuid:requirement-provider",
				"@type":             "dcs:DataRequirement",
				"dcs:conditionId":   "provider",
				"dcs:name":          "Provider",
				"dcs:schemaVersion": "v1",
				"dcs:entityType":    "CompanyParty",
				"dcs:entityRole":    "provider",
				"dcs:fields": []any{
					map[string]any{
						"@id":               "urn:uuid:field-provider-country",
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": "country",
						"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
						"dcs:required":      true,
					},
				},
			},
		},
		"dcs:policies": map[string]any{
			"@id":          "urn:uuid:policy-set-1",
			"@type":        "odrl:Offer",
			"uid":          "urn:uuid:policy-set-1",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:obligation": []any{
				map[string]any{
					"@id":           "urn:uuid:policy-provider-country-0",
					"@type":         "odrl:Duty",
					"odrl:action":   map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:assigner": map[string]any{"@id": "urn:uuid:party-provider"},
					"odrl:assignee": map[string]any{"@id": "urn:uuid:party-customer"},
					"odrl:target":   map[string]any{"@id": "urn:uuid:policy-target"},
					"odrl:constraint": map[string]any{
						"@type":             "odrl:Constraint",
						"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-provider-country"},
						"odrl:operator":     map[string]any{"@id": "odrl:isAnyOf"},
						"odrl:rightOperand": []any{"DEU", "AUT", "CHE"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return &data
}

// firstPolicyDuty returns the first odrl:duty rule node from the canonical
// dcs:policies odrl:Set structure produced by canonicalTemplateData.
func firstPolicyDuty(data map[string]any) map[string]any {
	set := data["dcs:policies"].(map[string]any)
	duties := set["odrl:obligation"].([]any)
	return duties[0].(map[string]any)
}

func validSemanticContractData(t *testing.T) *datatype.JSON {
	t.Helper()
	conditions := []any{
		partyCondition("provider", "Provider"),
		partyCondition("customer", "Customer"),
		map[string]any{
			"conditionId":   "payment",
			"conditionName": "Payment",
			"schemaVersion": "v1",
			"parameters": []any{
				semanticParam("amount", "decimal", SchemaContractV1, "contract.payment.amount"),
				semanticParam("currency", "string", SchemaContractV1, "contract.payment.currency"),
				semanticParam("dueDate", "date", SchemaContractV1, "contract.payment.dueDate"),
			},
		},
		map[string]any{
			"conditionId":   "sla",
			"conditionName": "SLA Availability",
			"schemaVersion": "v1",
			"parameters": []any{
				semanticParam("availability", "decimal", SchemaServiceV1, "service.sla.availability"),
			},
		},
	}
	data, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-main"}},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId": "clause-main",
				"type":    "CLAUSE",
				"text": strings.Join([]string{
					"Provider {{provider.legalName}} from {{provider.country}}",
					"Customer {{customer.legalName}} from {{customer.country}}",
					"Payment {{payment.amount}} {{payment.currency}} due {{payment.dueDate}}",
					"Availability {{sla.availability}}",
				}, "\n"),
				"conditionIds": []any{"provider", "customer", "payment", "sla"},
			},
		},
		"semanticConditions": conditions,
		"semanticConditionValues": []any{
			semanticValue("clause-main", "provider", "legalName", "Musterfirma"),
			semanticValue("clause-main", "provider", "country", "POL"),
			semanticValue("clause-main", "customer", "legalName", "Example company"),
			semanticValue("clause-main", "customer", "country", "DEU"),
			semanticValue("clause-main", "payment", "amount", 10000.0),
			semanticValue("clause-main", "payment", "currency", "EUR"),
			semanticValue("clause-main", "payment", "dueDate", "2026-06-19"),
			semanticValue("clause-main", "sla", "availability", 99.9),
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)
	return &data
}

func partyCondition(id string, name string) map[string]any {
	return map[string]any{
		"conditionId":   id,
		"conditionName": name,
		"schemaVersion": "v1",
		"entityType":    "CompanyParty",
		"entityRole":    id,
		"parameters": []any{
			semanticParam("legalName", "string", SchemaPartyV1, "company.legalName"),
			semanticParam("country", "string", SchemaPartyV1, "company.location.country"),
		},
	}
}

func semanticParam(name string, paramType string, schemaRef string, semanticPath string) map[string]any {
	return map[string]any{
		"parameterName": name,
		"type":          paramType,
		"schemaRef":     schemaRef,
		"semanticPath":  semanticPath,
		"isRequired":    true,
		"operators":     []any{},
	}
}

func semanticValue(blockID string, conditionID string, parameterName string, value any) map[string]any {
	return map[string]any{
		"blockId":        blockID,
		"conditionId":    conditionID,
		"parameterName":  parameterName,
		"parameterValue": value,
	}
}

func TestNormalizeTemplateDataRejectsLegacyStructure(t *testing.T) {
	_, err := NormalizeTemplateData(validTemplateData(t))
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}

func TestNormalizeContractDataRejectsLegacyStructure(t *testing.T) {
	_, err := NormalizeContractData(validTemplateData(t), false)
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}

func TestNormalizeTemplateDataAcceptsCanonicalJSONLDEnvelope(t *testing.T) {
	normalized, err := NormalizeTemplateData(canonicalTemplateData(t))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	require.Contains(t, result, "dcs:documentStructure")
	require.NotContains(t, result, "documentOutline")
	// normalizeCanonicalContext anchors @context as [hub context URL,
	// submitted inline prefix map] (ADR-8).
	anchored := result["@context"].([]any)
	require.Equal(t, SchemaJSONLDContextV1, anchored[0])
	require.Equal(t, "https://w3id.org/facis/dcs/ontology/v1#", anchored[1].(map[string]any)["dcs"])
	// The shapes pin rides on sh:shapesGraph (the ADR-8 anchor).
	require.Equal(t, SchemaSHACLShapesV1, result["sh:shapesGraph"].(map[string]any)["@id"])
	require.NotContains(t, result, "dcs:schemaRefs")
}

func TestNormalizeTemplateDataForPersistenceAddsDocumentIdentity(t *testing.T) {
	normalized, err := NormalizeTemplateDataForPersistence(canonicalTemplateData(t), "did:web:facis.example:template:1")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, "did:web:facis.example:template:1", data["@id"])
	require.NotContains(t, data, "did")
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#block-clause-1", block["@id"])
	placeholder := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#field-provider-country", placeholder["dcs:bindsTo"].(map[string]any)["@id"])
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#field-provider-country", constraint["odrl:leftOperand"].(map[string]any)["@id"])
}

func TestNormalizeTemplateDataForPersistenceRebasesCopiedTemplateIDs(t *testing.T) {
	first, err := NormalizeTemplateDataForPersistence(canonicalTemplateData(t), "did:web:facis.example:template:source")
	require.NoError(t, err)
	copied, err := NormalizeTemplateDataForPersistence(first, "did:web:facis.example:template:copy")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*copied, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:copy#block-clause-1", block["@id"])
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:copy#field-provider-country", constraint["odrl:leftOperand"].(map[string]any)["@id"])
}

func TestNormalizeTemplateDataRejectsMissingPlaceholderField(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	placeholder := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	placeholder["dcs:bindsTo"] = map[string]any{"@id": "urn:uuid:field-missing"}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "placeholder binds to nonexistent contract data field")
}

func TestNormalizeTemplateDataRejectsMissingPolicyField(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:field-missing"}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "policy references nonexistent contract data field")
}

func TestNormalizeTemplateDataRejectsUnreferencedBlock(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	blocksWrapper := structure["dcs:blocks"].(map[string]any)
	blocksWrapper["@list"] = append(blocksWrapper["@list"].([]any), map[string]any{
		"@id":      "urn:uuid:block-unreferenced",
		"@type":    "dcs:TextBlock",
		"dcs:text": "unused",
	})
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "is not referenced by layout")
}

func TestNormalizeTemplateDataAcceptsUnreferencedClause(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	blocksWrapper := structure["dcs:blocks"].(map[string]any)
	blocksWrapper["@list"] = append(blocksWrapper["@list"].([]any), map[string]any{
		"@id":         "urn:uuid:block-clause-pool",
		"@type":       "dcs:Clause",
		"dcs:title":   "Reusable clause",
		"dcs:content": map[string]any{"@list": []any{"Reusable content"}},
	})
	contract, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&contract)
	require.NoError(t, err)
}

func TestValidateContractSemanticsAcceptsCanonicalContract(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	data["@type"] = "dcs:Contract"
	data["dcs:policies"].(map[string]any)["@type"] = "odrl:Agreement"
	data["semanticConditionValues"] = []any{}
	contract, err := datatype.NewJSON(data)
	require.NoError(t, err)

	require.NoError(t, ValidateContractSemantics(&contract))
}
