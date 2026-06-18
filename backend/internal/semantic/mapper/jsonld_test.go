package mapper

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/stretchr/testify/require"
)

func newJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &raw
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
}

func templateFixture(t *testing.T) templatedb.ContractTemplate {
	t.Helper()
	name := "Provider Eligibility"
	documentNumber := "TPL-001"
	return templatedb.ContractTemplate{
		DID:            "did:web:example:template:provider-eligibility",
		DocumentNumber: &documentNumber,
		Version:        3,
		State:          "APPROVED",
		TemplateType:   "SUB_CONTRACT",
		Name:           &name,
		CreatedBy:      "user-1",
		CreatedAt:      fixedTime(),
		UpdatedAt:      fixedTime(),
		TemplateData: newJSON(t, map[string]any{
			"document": map[string]any{
				"@type": "Document",
				"outline": []any{
					map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-provider"}},
					map[string]any{"blockId": "clause-provider", "children": []any{}},
				},
				"blocks": []any{
					map[string]any{
						"blockId":      "clause-provider",
						"type":         "CLAUSE",
						"title":        "Provider eligibility",
						"text":         "Provider country: {{provider.country}}",
						"conditionIds": []any{"provider"},
					},
					map[string]any{
						"blockId": "unused-empty-clause",
						"type":    "CLAUSE",
						"text":    "",
					},
				},
			},
			"requirements": []any{
				map[string]any{
					"conditionId":   "provider",
					"conditionName": "Provider",
					"entityType":    "CompanyParty",
					"entityRole":    "provider",
					"parameters": []any{
						map[string]any{
							"parameterName": "country",
							"type":          "string",
							"semanticPath":  "company.country",
							"isRequired":    true,
							"operators": []any{
								map[string]any{
									"operate": "In",
									"targets": []any{"DEU", "AUT", "CHE"},
								},
							},
						},
					},
				},
			},
			"subTemplateSnapshots": []any{
				map[string]any{
					"did":     "did:web:example:template:legal-terms",
					"version": 1,
					"template_data": map[string]any{
						"document": map[string]any{"outline": []any{}, "blocks": []any{}},
						"requirements": []any{
							map[string]any{
								"conditionId": "jurisdiction",
								"parameters": []any{
									map[string]any{
										"parameterName": "country",
										"semanticPath":  "contract.jurisdiction",
										"operators": []any{
											map[string]any{"operate": "Equals", "targets": []any{"DEU"}},
										},
									},
								},
							},
						},
						"policyBundle": map[string]any{"rules": []any{}},
					},
				},
			},
		}),
	}
}

func TestBuildTemplateJSONLDProducesSeparatedTopLevelSections(t *testing.T) {
	env, err := BuildTemplateJSONLD(templateFixture(t), DefaultProfile())
	require.NoError(t, err)
	require.NoError(t, jsonRoundTrip(env))

	context := env["@context"].(map[string]any)
	require.Equal(t, dcsContextV1, context["dcs"])
	require.Equal(t, odrlContextV2, context["odrl"])
	require.Equal(t, xsdContext, context["xsd"])
	require.Equal(t, "dcs:ContractTemplate", env["@type"])
	require.Contains(t, env, "dcs:metadata")
	require.Contains(t, env, "dcs:documentStructure")
	require.Contains(t, env, "dcs:contractData")
	require.Contains(t, env, "dcs:policies")
	require.NotContains(t, env, "template_data")
	require.NotContains(t, env, "semanticRules")
}

func TestBuildTemplateJSONLDSeparatesDocumentStructureFromPolicies(t *testing.T) {
	env, err := BuildTemplateJSONLD(templateFixture(t), DefaultProfile())
	require.NoError(t, err)

	documentStructure := env["dcs:documentStructure"].(map[string]any)
	clauses := documentStructure["dcs:clauses"].([]any)
	require.Len(t, clauses, 1)
	clause := clauses[0].(map[string]any)
	require.Equal(t, "Provider country: {{provider.country}}", clause["dcs:text"])
	require.NotContains(t, clause, "odrl:constraint")
	require.NotContains(t, clause, "odrl:leftOperand")

	placeholders := clause["dcs:placeholders"].([]any)
	require.Len(t, placeholders, 1)
	placeholder := placeholders[0].(map[string]any)
	require.Equal(t, "dcs:Placeholder", placeholder["@type"])
	bindsTo := placeholder["dcs:bindsTo"].(map[string]any)
	require.Equal(t, "did:web:example:template:provider-eligibility#providerCountry", bindsTo["@id"])
}

func TestBuildTemplateJSONLDProviderCountryInIsODRLDuty(t *testing.T) {
	env, err := BuildTemplateJSONLD(templateFixture(t), DefaultProfile())
	require.NoError(t, err)

	policies := env["dcs:policies"].([]any)
	duty := findByID(t, policies, "did:web:example:template:provider-eligibility#policy-provider-country-in")
	require.Equal(t, "odrl:Duty", duty["@type"])

	constraint := duty["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:Constraint", constraint["@type"])
	require.Equal(t, "did:web:example:template:provider-eligibility#providerCountry", constraint["odrl:leftOperand"].(map[string]any)["@id"])
	require.Equal(t, "odrl:isAnyOf", constraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, []any{
		map[string]any{"@value": "DEU", "@type": "xsd:string"},
		map[string]any{"@value": "AUT", "@type": "xsd:string"},
		map[string]any{"@value": "CHE", "@type": "xsd:string"},
	}, constraint["odrl:rightOperand"])
	require.NotContains(t, duty, "dcs:severity")
	require.NotContains(t, duty, "dcs:enforcementPhase")
	require.NotContains(t, duty, "dcs:ruleKind")
	require.NotContains(t, duty, "odrl:assignee")
}

func TestBuildTemplateJSONLDKeepsODRLRightOperand(t *testing.T) {
	template := templateFixture(t)
	template.TemplateData = newJSON(t, map[string]any{
		"document": map[string]any{
			"outline": []any{},
			"blocks":  []any{},
		},
		"requirements": []any{
			map[string]any{
				"conditionId": "payment",
				"parameters": []any{
					map[string]any{
						"parameterName": "amount",
						"type":          "decimal",
						"semanticPath":  "contract.payment.amount",
						"operators": []any{
							map[string]any{
								"odrl:operator":     map[string]any{"@id": "odrl:gt"},
								"odrl:rightOperand": 500.0,
							},
						},
					},
				},
			},
		},
	})

	env, err := BuildTemplateJSONLD(template, DefaultProfile())
	require.NoError(t, err)

	policies := env["dcs:policies"].([]any)
	require.Len(t, policies, 1)
	constraint := policies[0].(map[string]any)["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:gt", constraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, map[string]any{"@value": "500", "@type": "xsd:decimal"}, constraint["odrl:rightOperand"])
}

func TestTypedRightOperandUsesParameterDatatype(t *testing.T) {
	tests := []struct {
		name          string
		value         any
		parameterType string
		expected      map[string]any
	}{
		{name: "decimal", value: 99.95, parameterType: "decimal", expected: map[string]any{"@value": "99.95", "@type": "xsd:decimal"}},
		{name: "integer", value: 12, parameterType: "integer", expected: map[string]any{"@value": "12", "@type": "xsd:integer"}},
		{name: "boolean", value: true, parameterType: "boolean", expected: map[string]any{"@value": "true", "@type": "xsd:boolean"}},
		{name: "date", value: "2026-06-18", parameterType: "date", expected: map[string]any{"@value": "2026-06-18", "@type": "xsd:date"}},
		{name: "string", value: "DEU", parameterType: "string", expected: map[string]any{"@value": "DEU", "@type": "xsd:string"}},
		{name: "enum", value: "GOLD", parameterType: "enum", expected: map[string]any{"@value": "GOLD", "@type": "xsd:string"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, typedRightOperand(test.value, test.parameterType))
		})
	}
}

func TestParseOperatorUnwrapsTypedRightOperands(t *testing.T) {
	operate, targets := parseOperator(map[string]any{
		"operate": "odrl:isAnyOf",
		"odrl:rightOperand": []any{
			map[string]any{"@value": "99.95", "@type": "xsd:decimal"},
			map[string]any{"@value": "100.5", "@type": "xsd:decimal"},
		},
	})

	require.Equal(t, "odrl:isAnyOf", operate)
	require.Equal(t, []any{99.95, 100.5}, targets)
}

func TestBuildTemplateJSONLDContractDataContainsStableFieldIDs(t *testing.T) {
	env, err := BuildTemplateJSONLD(templateFixture(t), DefaultProfile())
	require.NoError(t, err)

	contractData := env["dcs:contractData"].([]any)
	provider := findByID(t, contractData, "did:web:example:template:provider-eligibility#provider")
	require.Equal(t, "dcs:ContractPartyRole", provider["@type"])
	require.Equal(t, "dcs:Provider", provider["dcs:role"].(map[string]any)["@id"])
	require.Equal(t, "did:web:example:template:provider-eligibility#providerParty", provider["dcs:partyRef"].(map[string]any)["@id"])
	require.Equal(t, "did:web:example:template:provider-eligibility#providerCountry", provider["dcs:country"].(map[string]any)["@id"])
}

func TestBuildTemplateJSONLDIncludesReusableSubTemplateDataWithStableIDs(t *testing.T) {
	env, err := BuildTemplateJSONLD(templateFixture(t), DefaultProfile())
	require.NoError(t, err)

	documentStructure := env["dcs:documentStructure"].(map[string]any)
	subTemplates := documentStructure["dcs:subTemplates"].([]any)
	require.Len(t, subTemplates, 1)
	require.Equal(t, "did:web:example:template:legal-terms", subTemplates[0].(map[string]any)["@id"])

	contractData := env["dcs:contractData"].([]any)
	require.Len(t, contractData, 2)
	require.Equal(t, "did:web:example:template:legal-terms#jurisdiction", contractData[1].(map[string]any)["@id"])

	policies := env["dcs:policies"].([]any)
	require.Len(t, policies, 2)
	subPolicy := policies[1].(map[string]any)
	constraint := subPolicy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:example:template:legal-terms#jurisdictionCountry", constraint["odrl:leftOperand"].(map[string]any)["@id"])
}

func TestBuildTemplateJSONLDSmallTemplateUsesLayeredSemanticObjectsAndODRL(t *testing.T) {
	template := templateFixture(t)
	template.TemplateData = newJSON(t, map[string]any{
		"document": map[string]any{
			"outline": []any{
				map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-main"}},
				map[string]any{"blockId": "clause-main", "children": []any{}},
			},
			"blocks": []any{
				map[string]any{
					"blockId": "clause-main",
					"type":    "CLAUSE",
					"title":   "Commercial terms",
					"text":    "Provider country: {{provider.country}}. Customer country: {{customer.country}}. Payment amount: {{payment.amount}}.",
					"conditionIds": []any{
						"provider",
						"customer",
						"payment",
					},
				},
			},
		},
		"requirements": []any{
			map[string]any{
				"conditionId":   "provider",
				"conditionName": "Provider",
				"entityType":    "CompanyParty",
				"entityRole":    "provider",
				"parameters": []any{
					map[string]any{
						"parameterName": "country",
						"type":          "string",
						"semanticPath":  "company.country",
						"operators": []any{
							map[string]any{
								"operate": "In",
								"targets": []any{"DEU", "AUT", "CHE"},
							},
						},
					},
				},
			},
			map[string]any{
				"conditionId":   "customer",
				"conditionName": "Customer",
				"entityType":    "CompanyParty",
				"entityRole":    "customer",
				"parameters": []any{
					map[string]any{
						"parameterName": "country",
						"type":          "string",
						"semanticPath":  "company.country",
					},
				},
			},
			map[string]any{
				"conditionId":   "payment",
				"conditionName": "Payment",
				"entityType":    "PaymentTerm",
				"parameters": []any{
					map[string]any{
						"parameterName": "amount",
						"type":          "decimal",
						"semanticPath":  "contract.payment.amount",
						"operators": []any{
							map[string]any{
								"operate": "GreaterThan",
								"targets": []any{500.0},
							},
						},
					},
				},
			},
		},
	})

	env, err := BuildTemplateJSONLD(template, DefaultProfile())
	require.NoError(t, err)
	require.NoError(t, jsonRoundTrip(env))

	require.Contains(t, env, "dcs:metadata")
	require.Contains(t, env, "dcs:documentStructure")
	require.Contains(t, env, "dcs:contractData")
	require.Contains(t, env, "dcs:policies")

	documentStructure := env["dcs:documentStructure"].(map[string]any)
	clauses := documentStructure["dcs:clauses"].([]any)
	require.Len(t, clauses, 1)
	require.NotContains(t, clauses[0].(map[string]any), "odrl:constraint")
	placeholders := clauses[0].(map[string]any)["dcs:placeholders"].([]any)
	providerCountryPlaceholder := findPlaceholderByBinding(t, placeholders, "did:web:example:template:provider-eligibility#providerCountry")
	require.Equal(t, "{{provider.country}}", providerCountryPlaceholder["dcs:token"])

	contractData := env["dcs:contractData"].([]any)
	provider := findByID(t, contractData, "did:web:example:template:provider-eligibility#provider")
	require.Equal(t, "dcs:ContractPartyRole", provider["@type"])
	require.Equal(t, "dcs:Provider", provider["dcs:role"].(map[string]any)["@id"])
	require.Equal(t, "did:web:example:template:provider-eligibility#providerCountry", provider["dcs:country"].(map[string]any)["@id"])
	customer := findByID(t, contractData, "did:web:example:template:provider-eligibility#customer")
	require.Equal(t, "dcs:ContractPartyRole", customer["@type"])
	require.Equal(t, "dcs:Customer", customer["dcs:role"].(map[string]any)["@id"])
	payment := findByID(t, contractData, "did:web:example:template:provider-eligibility#payment")
	require.Equal(t, "dcs:PaymentTerm", payment["@type"])
	require.Equal(t, "did:web:example:template:provider-eligibility#paymentAmount", payment["dcs:amount"].(map[string]any)["@id"])

	policies := env["dcs:policies"].([]any)
	providerCountryPolicy := findByID(t, policies, "did:web:example:template:provider-eligibility#policy-provider-country-in")
	constraint := providerCountryPolicy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:example:template:provider-eligibility#providerCountry", constraint["odrl:leftOperand"].(map[string]any)["@id"])
	require.Equal(t, "odrl:isAnyOf", constraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, []any{
		map[string]any{"@value": "DEU", "@type": "xsd:string"},
		map[string]any{"@value": "AUT", "@type": "xsd:string"},
		map[string]any{"@value": "CHE", "@type": "xsd:string"},
	}, constraint["odrl:rightOperand"])
	require.NotContains(t, providerCountryPolicy, "odrl:assignee")
	require.NotContains(t, providerCountryPolicy, "dcs:severity")
}

func TestBuildContractJSONLDUsesSameSeparatedSections(t *testing.T) {
	sourceTemplate := templateFixture(t)
	contract := contractdb.Contract{
		DID:             "did:web:example:contract:1",
		ContractVersion: 1,
		State:           "DRAFT",
		CreatedBy:       "user-1",
		CreatedAt:       fixedTime(),
		UpdatedAt:       fixedTime(),
		ContractData:    sourceTemplate.TemplateData,
	}

	env, err := BuildContractJSONLD(contract, sourceTemplate, DefaultProfile())
	require.NoError(t, err)
	require.Equal(t, "dcs:Contract", env["@type"])
	require.Contains(t, env, "dcs:metadata")
	require.Contains(t, env, "dcs:documentStructure")
	require.Contains(t, env, "dcs:contractData")
	require.Contains(t, env, "dcs:policies")
}

func jsonRoundTrip(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var decoded any
	return json.Unmarshal(raw, &decoded)
}

func findByID(t *testing.T, values []any, id string) map[string]any {
	t.Helper()
	for _, value := range values {
		item, ok := value.(map[string]any)
		if ok && item["@id"] == id {
			return item
		}
	}
	require.Failf(t, "item not found", "missing @id %s", id)
	return nil
}

func findPlaceholderByBinding(t *testing.T, values []any, id string) map[string]any {
	t.Helper()
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		bindsTo, ok := item["dcs:bindsTo"].(map[string]any)
		if ok && bindsTo["@id"] == id {
			return item
		}
	}
	require.Failf(t, "placeholder not found", "missing dcs:bindsTo %s", id)
	return nil
}
