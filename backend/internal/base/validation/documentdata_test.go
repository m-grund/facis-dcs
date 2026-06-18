package validation

import (
	"encoding/json"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"

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
	require.Equal(t, expandOntologyResource("dcst:field-service-sla-availability"), param["semanticPath"])
}

func TestNormalizeTemplateDataAcceptsCanonicalJSONLDEnvelope(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@id":   "did:web:facis.example:template:canonical",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@id":   "did:web:facis.example:template:canonical#metadata",
			"@type": "dcs:TemplateMetadata",
		},
		"dcs:documentStructure": map[string]any{
			"@id":          "did:web:facis.example:template:canonical#document-structure",
			"@type":        "dcs:DocumentStructure",
			"dcs:sections": []any{},
			"dcs:clauses":  []any{},
		},
		"dcs:contractData": []any{},
		"dcs:policies":     []any{},
	})
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	require.Contains(t, result, "dcs:documentStructure")
	require.NotContains(t, result, "documentOutline")
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

func TestNormalizeContractDataDoesNotInferRoleFromCustomerEntityType(t *testing.T) {
	data := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))

	conditions := decoded["semanticConditions"].([]any)
	for _, rawCondition := range conditions {
		condition := rawCondition.(map[string]any)
		if condition["conditionId"] != "customer" {
			continue
		}
		condition["entityType"] = "Customer"
		delete(condition, "entityRole")
		params := condition["parameters"].([]any)
		condition["parameters"] = []any{params[0], params[1]}
	}
	values := decoded["semanticConditionValues"].([]any)
	filtered := []any{}
	for _, rawValue := range values {
		value := rawValue.(map[string]any)
		if value["conditionId"] == "customer" && value["parameterName"] == "role" {
			continue
		}
		filtered = append(filtered, rawValue)
	}
	decoded["semanticConditionValues"] = filtered

	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, "unsupported entityType")
}

func TestNormalizeContractDataRejectsNonCanonicalSemanticPathAliases(t *testing.T) {
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
		}
	}

	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, `unknown domain semanticPath "company_legalName"`)
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
	require.Equal(t, expandOntologyResource("dcst:field-company-location-country"), normalizedParam["semanticPath"])
	constraint := normalizedParam["valueConstraint"].(map[string]any)
	require.Equal(t, "iso-3166-1-alpha-3", constraint["format"])
	require.Contains(t, constraint["allowedValues"], "DEU")
	require.Contains(t, constraint["allowedValues"], "DNK")
	options := constraint["valueOptions"].([]any)
	require.Equal(t, "Germany", valueOptionByCode(options, "DEU")["label"])
	require.Equal(t, "Denmark", valueOptionByCode(options, "DNK")["label"])
}

func TestNormalizeTemplateDataAddsValueOptionSymbols(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "currency",
		"type":          "string",
		"schemaRef":     SchemaContractV1,
		"semanticPath":  "contract.payment.currency",
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
	require.Contains(t, constraint["allowedValues"], "EUR")
	require.Equal(t, "€", valueOptionByCode(constraint["valueOptions"].([]any), "EUR")["symbol"])
}

func valueOptionByCode(options []any, code string) map[string]any {
	for _, raw := range options {
		option, ok := raw.(map[string]any)
		if ok && option["value"] == code {
			return option
		}
	}
	return nil
}

func TestNormalizeTemplateDataRejectsCompanyRoleDomainField(t *testing.T) {
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
	require.Nil(t, normalized)
	require.ErrorContains(t, err, `unknown domain semanticPath "company.role"`)
}

func TestNormalizeContractDataAcceptsCompanyParties(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "CompanyParty", "role": "supplier", "legalName": "Example Supplier GmbH"},
		map[string]any{"@type": "dcs:CompanyParty", "role": "customer", "legalName": "Example Customer AG"},
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
		map[string]any{"@type": "CompanyParty", "role": "reseller", "legalName": "Example Reseller GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.role")
}

func TestNormalizeContractDataRejectsNonCompanyPartyType(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "Company", "role": "supplier", "legalName": "Example Supplier GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.@type")
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

func TestValueConstraintResolvesAllowedValuesRef(t *testing.T) {
	err := valueMatchesConstraint("DEU", &valueConstraint{AllowedValuesRef: "ISO 3166-1 alpha-3"})
	require.NoError(t, err)

	err = valueMatchesConstraint("ZZZ", &valueConstraint{AllowedValuesRef: "ISO 3166-1 alpha-3"})
	require.ErrorContains(t, err, "expected one of")
}

func TestValueConstraintValidatesFormatWithoutAllowedValues(t *testing.T) {
	err := valueMatchesConstraint("DEU", &valueConstraint{Format: "iso-3166-1-alpha-3"})
	require.NoError(t, err)

	err = valueMatchesConstraint("DE", &valueConstraint{Format: "iso-3166-1-alpha-3"})
	require.ErrorContains(t, err, "expected value matching format")
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
			"operate": "GreaterThanOrEqual",
			"targets": []any{99.95},
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
	require.Equal(t, "odrl:gteq", rules[0].(map[string]any)["operator"])
	require.Equal(t, 99.95, rules[0].(map[string]any)["rightOperand"])
	require.Equal(t, []any{"clause-1"}, rules[0].(map[string]any)["appliesToClause"])
	require.Equal(t, "semanticCondition", rules[0].(map[string]any)["source"])
	policyBundle := result["policyBundle"].(map[string]any)
	require.Equal(t, "PolicyBundle", policyBundle["@type"])
	require.Equal(t, "odrl-jsonld", policyBundle["format"])
	duties := policyBundle["rules"].([]any)
	require.Len(t, duties, 1)
	constraints := duties[0].(map[string]any)["odrl:constraint"].([]any)
	require.Equal(t, "odrl:gteq", constraints[0].(map[string]any)["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, 99.95, constraints[0].(map[string]any)["odrl:rightOperand"])
}

func TestNormalizeTemplateDataGeneratesSetOperatorPolicy(t *testing.T) {
	raw := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	condition := decoded["semanticConditions"].([]any)[0].(map[string]any)
	condition["conditionName"] = "Jurisdiction"
	params := condition["parameters"].([]any)
	param := params[0].(map[string]any)
	param["parameterName"] = "jurisdiction"
	param["type"] = "string"
	param["schemaRef"] = SchemaContractV1
	param["semanticPath"] = "contract.jurisdiction"
	param["operators"] = []any{
		map[string]any{
			"operate": "In",
			"targets": []any{"DEU", "AUT"},
		},
	}
	decoded["documentBlocks"].([]any)[0].(map[string]any)["text"] = "Jurisdiction {{cond-1.jurisdiction}}"
	templateData, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&templateData)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	policyBundle := result["policyBundle"].(map[string]any)
	duties := policyBundle["rules"].([]any)
	constraints := duties[0].(map[string]any)["odrl:constraint"].([]any)
	require.Equal(t, "odrl:isAnyOf", constraints[0].(map[string]any)["odrl:operator"].(map[string]any)["@id"])
}

func TestNormalizeTemplateDataAcceptsCanonicalSemanticRuleOperator(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticRules"] = []any{
		map[string]any{
			"@type":           "SemanticRule",
			"ruleId":          "existing-rule",
			"leftOperand":     "$.country",
			"operator":        "Equals",
			"rightOperand":    "DEU",
			"appliesToClause": []any{"clause-1"},
			"valueType":       "string",
			"severity":        "error",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	rule := result["semanticRules"].([]any)[0].(map[string]any)
	require.Equal(t, "odrl:eq", rule["operator"])
	require.Equal(t, "DEU", rule["rightOperand"])
	require.Equal(t, []any{"clause-1"}, rule["appliesToClause"])
}

func TestNormalizeTemplateDataRejectsNonCanonicalSemanticRuleOperator(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticRules"] = []any{
		map[string]any{
			"@type":           "SemanticRule",
			"ruleId":          "existing-rule",
			"leftOperand":     "$.country",
			"operator":        "equal",
			"rightOperand":    "DEU",
			"appliesToClause": []any{"clause-1"},
			"valueType":       "string",
			"severity":        "error",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "unsupported semantic rule operator")
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
