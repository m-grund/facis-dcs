package validation

import (
	"digital-contracting-service/internal/base/datatype"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			semanticValue("clause-main", "provider", "role", "provider"),
			semanticValue("clause-main", "customer", "legalName", "Example company"),
			semanticValue("clause-main", "customer", "country", "DEU"),
			semanticValue("clause-main", "customer", "role", "customer"),
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
		"parameters": []any{
			semanticParam("legalName", "string", SchemaPartyV1, "company.legalName"),
			semanticParam("country", "string", SchemaPartyV1, "company.location.country"),
			semanticParam("role", "string", SchemaPartyV1, "company.role"),
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

func mutateSemanticValue(t *testing.T, raw *datatype.JSON, conditionID string, parameterName string, value any) *datatype.JSON {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	values := decoded["semanticConditionValues"].([]any)
	for _, item := range values {
		semanticValue := item.(map[string]any)
		if semanticValue["conditionId"] == conditionID && semanticValue["parameterName"] == parameterName {
			semanticValue["parameterValue"] = value
			break
		}
	}
	result, err := datatype.NewJSON(decoded)
	require.NoError(t, err)
	return &result
}

func removeSemanticValue(t *testing.T, raw *datatype.JSON, conditionID string, parameterName string) *datatype.JSON {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	values := decoded["semanticConditionValues"].([]any)
	filtered := []any{}
	for _, item := range values {
		semanticValue := item.(map[string]any)
		if semanticValue["conditionId"] == conditionID && semanticValue["parameterName"] == parameterName {
			continue
		}
		filtered = append(filtered, item)
	}
	decoded["semanticConditionValues"] = filtered
	result, err := datatype.NewJSON(decoded)
	require.NoError(t, err)
	return &result
}

func ruleIDs(rules []any) []string {
	result := []string{}
	for _, item := range rules {
		rule := item.(map[string]any)
		ruleID, _ := rule["ruleId"].(string)
		result = append(result, ruleID)
	}
	return result
}

func TestNormalizeTemplateDataAddsSchemaAndPolicyRefs(t *testing.T) {
	normalized, err := NormalizeTemplateData(validTemplateData(t))
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, SchemaJSONLDContextV1, data["@context"])
	require.Equal(t, "ContractTemplate", data["@type"])
	require.Equal(t, SchemaTemplateDataV1, data["schemaRefs"].(map[string]any)["templateData"])
	require.Equal(t, SchemaJSONLDContextV1, data["schemaRefs"].(map[string]any)["jsonLdContext"])
	require.NotEmpty(t, data["policyRefs"])
	require.Equal(t, "FACIS_DCS_TEMPLATE_V1", data["validation"].(map[string]any)["profile"])
	require.Equal(t, SemanticProfileVersionV1, data["semanticProfile"].(map[string]any)["version"])
	require.IsType(t, []any{}, data["placeholderBindings"])
	require.IsType(t, []any{}, data["semanticRules"])
	condition := data["semanticConditions"].([]any)[0].(map[string]any)
	param := condition["parameters"].([]any)[0].(map[string]any)
	require.Equal(t, ontologyDCSTBase+"field-service-sla-availability", param["semanticPath"])
}

func TestNormalizeTemplateDataRejectsUnknownConditionReference(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticConditions"] = []any{}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "unknown semantic condition")
}

func TestNormalizeContractDataRequiresSemanticValuesWhenStrict(t *testing.T) {
	templateData := validTemplateData(t)
	contractData, err := NormalizeContractData(templateData, false)
	require.NoError(t, err)

	_, err = NormalizeContractData(contractData, true)
	require.ErrorContains(t, err, "required semantic value missing")
}

func TestNormalizeContractDataAddsJSONLDContractType(t *testing.T) {
	normalized, err := NormalizeContractData(validTemplateData(t), false)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, SchemaJSONLDContextV1, data["@context"])
	require.Equal(t, "Contract", data["@type"])
}

func TestNormalizeTemplateDataForPersistenceAddsDocumentIdentity(t *testing.T) {
	normalized, err := NormalizeTemplateDataForPersistence(validTemplateData(t), "did:web:facis.example:template:1")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, "did:web:facis.example:template:1", data["@id"])
	require.Equal(t, "did:web:facis.example:template:1", data["did"])
}

func TestNormalizeContractDataForPersistenceAddsDocumentIdentity(t *testing.T) {
	normalized, err := NormalizeContractDataForPersistence(validTemplateData(t), "did:web:facis.example:contract:1", false)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, "did:web:facis.example:contract:1", data["@id"])
	require.Equal(t, "did:web:facis.example:contract:1", data["did"])
}

func TestNormalizeContractDataAcceptsTypedSemanticValues(t *testing.T) {
	templateData := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*templateData, &decoded))
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "percent",
			"parameterValue": 99.9,
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.NoError(t, err)
}

func TestNormalizeContractDataBuildsContractStatementsAndRules(t *testing.T) {
	normalized, err := NormalizeContractData(validSemanticContractData(t), true)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	statementSet := data["contractStatements"].(map[string]any)
	require.Equal(t, contractStatementSetType, statementSet["@type"])
	statements := statementSet["statements"].([]any)
	require.NotEmpty(t, statements)
	require.Contains(t, statements, map[string]any{
		"@id":       "party-provider",
		"@type":     contractStatementPartyType,
		"role":      contractStatementProviderRole,
		"legalName": "Musterfirma",
		"country":   "POL",
	})
	require.Contains(t, statements, map[string]any{
		"@id":      "payment-main",
		"@type":    contractStatementPaymentType,
		"payer":    "party-customer",
		"payee":    "party-provider",
		"currency": "EUR",
		"amount":   10000.0,
		"dueDate":  "2026-06-19",
	})

	rules := data["semanticRules"].([]any)
	require.GreaterOrEqual(t, len(rules), 8)
	require.Contains(t, ruleIDs(rules), "rule-payment-amount-positive")
	require.Contains(t, ruleIDs(rules), "rule-exactly-one-provider")
}

func TestNormalizeContractDataRejectsInvalidStatementCountry(t *testing.T) {
	_, err := NormalizeContractData(mutateSemanticValue(t, validSemanticContractData(t), "provider", "country", "RUS"), true)
	require.ErrorContains(t, err, "violates constraint")
}

func TestNormalizeContractDataRejectsInvalidStatementCurrency(t *testing.T) {
	_, err := NormalizeContractData(mutateSemanticValue(t, validSemanticContractData(t), "payment", "currency", "XXX"), true)
	require.ErrorContains(t, err, "violates constraint")
}

func TestNormalizeContractDataRejectsSLAAvailabilityOver100(t *testing.T) {
	_, err := NormalizeContractData(mutateSemanticValue(t, validSemanticContractData(t), "sla", "availability", 100.1), true)
	require.ErrorContains(t, err, "violates constraint")
}

func TestNormalizeContractDataRejectsMissingRequiredStatementValue(t *testing.T) {
	_, err := NormalizeContractData(removeSemanticValue(t, validSemanticContractData(t), "payment", "amount"), true)
	require.ErrorContains(t, err, "required semantic value missing")
}

func TestValidateContractSemanticsRejectsPlaceholderWithoutBinding(t *testing.T) {
	raw := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	decoded["placeholderBindings"] = []any{}
	contractData, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	err = ValidateContractSemantics(&contractData)
	require.ErrorContains(t, err, "has no binding")
}

func TestValidateContractSemanticsRejectsBindingToUnknownParameter(t *testing.T) {
	raw := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	decoded["placeholderBindings"] = []any{
		map[string]any{
			"@type":            "PlaceholderBinding",
			"source":           "clause-placeholder",
			"blockId":          "clause-main",
			"placeholder":      "{{provider.legalName}}",
			"boundToCondition": "provider",
			"boundToParameter": "missing",
		},
	}
	contractData, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	err = ValidateContractSemantics(&contractData)
	require.ErrorContains(t, err, "unknown parameter")
}

func TestNormalizeContractDataRejectsPaymentAmountNotPositive(t *testing.T) {
	_, err := NormalizeContractData(mutateSemanticValue(t, validSemanticContractData(t), "payment", "amount", 0.0), true)
	require.ErrorContains(t, err, "rule-payment-amount-positive")
}

func TestNormalizeContractDataRejectsMissingProviderOrCustomer(t *testing.T) {
	_, err := NormalizeContractData(mutateSemanticValue(t, validSemanticContractData(t), "provider", "role", "customer"), true)
	require.ErrorContains(t, err, "exactly one provider")
}

func TestNormalizeContractDataAcceptsLegacyCustomerFieldAliases(t *testing.T) {
	data := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))

	blocks := decoded["documentBlocks"].([]any)
	clause := blocks[0].(map[string]any)
	clause["text"] = strings.ReplaceAll(clause["text"].(string), "{{customer.legalName}}", "{{customer.company_legalName}}")
	clause["text"] = strings.ReplaceAll(clause["text"].(string), "{{customer.country}}", "{{customer.company_location_country}}")

	conditions := decoded["semanticConditions"].([]any)
	for _, rawCondition := range conditions {
		condition := rawCondition.(map[string]any)
		if condition["conditionId"] != "customer" {
			continue
		}
		condition["parameters"] = []any{
			semanticParam("company_legalName", "string", SchemaPartyV1, "company_legalName"),
			semanticParam("company_location_country", "string", SchemaPartyV1, "company_location_country"),
			semanticParam("company_role", "string", SchemaPartyV1, "company_role"),
		}
	}

	values := decoded["semanticConditionValues"].([]any)
	for _, rawValue := range values {
		value := rawValue.(map[string]any)
		if value["conditionId"] != "customer" {
			continue
		}
		switch value["parameterName"] {
		case "legalName":
			value["parameterName"] = "company_legalName"
		case "country":
			value["parameterName"] = "company_location_country"
		case "role":
			value["parameterName"] = "company_role"
		}
	}

	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeContractData(&raw, true)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	statementSet := result["contractStatements"].(map[string]any)
	statements := statementSet["statements"].([]any)
	require.Contains(t, statements, map[string]any{
		"@id":       "party-customer",
		"@type":     contractStatementPartyType,
		"role":      contractStatementCustomerRole,
		"legalName": "Example company",
		"country":   "DEU",
	})
}

func TestNormalizeContractDataAcceptsConditionsFromEmbeddedTemplateSnapshot(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"approved-1"}},
			map[string]any{"blockId": "approved-1", "isRoot": false, "children": []any{"approved-1::clause-1"}},
			map[string]any{"blockId": "approved-1::clause-1", "isRoot": false, "children": []any{}},
		},
		"documentBlocks": []any{
			map[string]any{"blockId": "approved-1", "type": "APPROVED_TEMPLATE", "templateId": "template-1", "version": 1},
			map[string]any{"blockId": "approved-1::clause-1", "type": "CLAUSE", "text": "Company {{cond-1.company_legalName}}", "conditionIds": []any{"cond-1"}},
		},
		"semanticConditions": []any{},
		"subTemplateSnapshots": []any{
			map[string]any{
				"did": "template-1",
				"template_data": map[string]any{
					"semanticConditions": []any{
						map[string]any{
							"conditionId":   "cond-1",
							"conditionName": "Company",
							"schemaVersion": "v1",
							"parameters": []any{
								map[string]any{
									"parameterName": "company_legalName",
									"type":          "string",
									"schemaRef":     SchemaPartyV1,
									"semanticPath":  "company.legalName",
									"isRequired":    true,
									"operators":     []any{},
								},
							},
						},
					},
				},
			},
		},
		"semanticConditionValues": []any{
			map[string]any{
				"blockId":        "approved-1::clause-1",
				"conditionId":    "cond-1",
				"parameterName":  "company_legalName",
				"parameterValue": "Example GmbH",
			},
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.NoError(t, err)
}

func TestNormalizeTemplateDataAddsCanonicalValueConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	normalizedCondition := result["semanticConditions"].([]any)[0].(map[string]any)
	normalizedParam := normalizedCondition["parameters"].([]any)[0].(map[string]any)
	require.Equal(t, ontologyDCSTBase+"field-company-location-country", normalizedParam["semanticPath"])
	constraint := normalizedParam["valueConstraint"].(map[string]any)
	require.Equal(t, "iso-3166-1-alpha-3", constraint["format"])
	require.Contains(t, constraint["allowedValues"], "DEU")
}

func TestNormalizeTemplateDataAddsContractPartyRoleConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "role",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.role",
		"isRequired":    true,
		"operators":     []any{},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	normalizedCondition := result["semanticConditions"].([]any)[0].(map[string]any)
	normalizedParam := normalizedCondition["parameters"].([]any)[0].(map[string]any)
	constraint := normalizedParam["valueConstraint"].(map[string]any)
	require.Equal(t, "controlled-vocabulary", constraint["format"])
	require.Equal(t, []any{"supplier", "customer", "provider", "client"}, constraint["allowedValues"])
}

func TestNormalizeContractDataAcceptsCompanyParties(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "Company", "role": "supplier", "legalName": "Example Supplier GmbH"},
		map[string]any{"@type": "dcs:Company", "role": "customer", "legalName": "Example Customer AG"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.NoError(t, err)
}

func TestNormalizeContractDataRejectsPartyRoleOutsideVocabulary(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "Company", "role": "reseller", "legalName": "Example Reseller GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.role")
}

func TestNormalizeContractDataRejectsNonCompanyParty(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "Party", "role": "supplier", "legalName": "Example Supplier GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.@type")
}

func TestNormalizeContractDataRejectsSemanticValueOutsideConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	decoded["documentBlocks"].([]any)[0].(map[string]any)["text"] = "Country {{cond-1.country}}"
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "country",
			"parameterValue": "Germany",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, "violates constraint")
}

func TestNormalizeContractDataAcceptsSemanticValueInsideConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	decoded["documentBlocks"].([]any)[0].(map[string]any)["text"] = "Country {{cond-1.country}}"
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "country",
			"parameterValue": "DEU",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.NoError(t, err)
}

func TestNormalizeTemplateDataGeneratesSemanticRuleAndPlaceholderBinding(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	param := params[0].(map[string]any)
	param["operators"] = []any{
		map[string]any{
			"operate": "greaterThanOrEqual",
			"targets": []any{"99.95"},
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	bindings := result["placeholderBindings"].([]any)
	require.Len(t, bindings, 1)
	require.Equal(t, "{{cond-1.percent}}", bindings[0].(map[string]any)["placeholder"])
	rules := result["semanticRules"].([]any)
	require.Len(t, rules, 1)
	require.Equal(t, "GreaterThanOrEqual", rules[0].(map[string]any)["operator"])
	require.Equal(t, []any{"clause-1"}, rules[0].(map[string]any)["appliesToClause"])
	require.Equal(t, "semanticCondition", rules[0].(map[string]any)["source"])
}

func TestNormalizeTemplateDataCanonicalizesExistingSemanticRuleProperties(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticRules"] = []any{
		map[string]any{
			"@type":       "SemanticRule",
			"ruleId":      "existing-rule",
			"leftOperand": "$.country",
			"operate":     "equal",
			"targets":     []any{"DEU"},
			"blockIds":    []any{"clause-1"},
			"valueType":   "string",
			"severity":    "error",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	rule := result["semanticRules"].([]any)[0].(map[string]any)
	require.Equal(t, "Equals", rule["operator"])
	require.Equal(t, "DEU", rule["rightOperand"])
	require.Equal(t, []any{"clause-1"}, rule["appliesToClause"])
	require.NotContains(t, rule, "operate")
	require.NotContains(t, rule, "targets")
	require.NotContains(t, rule, "blockIds")
}

func TestNormalizeTemplateDataRejectsUnsupportedSemanticOperator(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	param := params[0].(map[string]any)
	param["operators"] = []any{
		map[string]any{
			"operate": "unsupported",
			"targets": []any{"99.95"},
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "unsupported operator")
}
