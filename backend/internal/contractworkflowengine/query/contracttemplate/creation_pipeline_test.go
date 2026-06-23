package contracttemplate

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/stretchr/testify/require"
)

const (
	creationTemplateDID = "did:web:example:template:dach-service"
	creationContractDID = "did:web:example:contract:dach-service"
)

func TestCreateTemplateThenFinalContractWithPartiesPaymentAndAvailability(t *testing.T) {
	templateData := newCreationPipelineJSON(t, creationPipelineTemplate())
	persistedTemplate, err := validation.NormalizeTemplateDataForPersistence(templateData, creationTemplateDID)
	require.NoError(t, err)

	contractDraft, err := convertTemplateDataToContractData(persistedTemplate, creationTemplateDID)
	require.NoError(t, err)

	var contractData map[string]any
	require.NoError(t, json.Unmarshal(*contractDraft, &contractData))
	contractData["semanticConditionValues"] = creationPipelineValues()
	contractWithValues := newCreationPipelineJSON(t, contractData)
	persistedContract, err := validation.NormalizeContractDataForPersistence(
		contractWithValues,
		creationContractDID,
		true,
	)
	require.NoError(t, err)

	assertCreationPipelineLayout(t, persistedContract)
	assertCreationPipelinePolicies(t, persistedContract)

	now := time.Date(2026, time.June, 19, 12, 0, 0, 0, time.UTC)
	templateName := "DACH Service Agreement"
	template := templatedb.ContractTemplate{
		DID:          creationTemplateDID,
		Version:      1,
		State:        "APPROVED",
		TemplateType: "SUB_CONTRACT",
		Name:         &templateName,
		CreatedBy:    "test-participant",
		CreatedAt:    now,
		UpdatedAt:    now,
		TemplateData: persistedTemplate,
	}
	contract := contractdb.Contract{
		DID:             creationContractDID,
		ContractVersion: 1,
		State:           "APPROVED",
		CreatedBy:       "test-participant",
		CreatedAt:       now,
		UpdatedAt:       now,
		ContractData:    persistedContract,
	}

	published, err := semanticmapper.BuildContractJSONLD(contract, template, semanticmapper.DefaultProfile())
	require.NoError(t, err)
	content, err := json.MarshalIndent(published, "", "  ")
	require.NoError(t, err)
	t.Logf("Published JSON-LD:\n%s", content)
	require.NotContains(t, published, "semanticConditionValues")
	require.Contains(t, string(content), "dcs:Placeholder")
	require.Contains(t, string(content), "dcs:bindsTo")
	require.NotContains(t, string(content), "dcs:DataRequirement")
	require.NotContains(t, string(content), "dcs:RequirementField")

	require.ElementsMatch(t, []string{
		"@context",
		"@id",
		"@type",
		"dcs:metadata",
		"dcs:contractData",
		"dcs:contractFields",
		"dcs:documentStructure",
		"dcs:policies",
	}, creationPipelineMapKeys(published))

	metadata := published["dcs:metadata"].(map[string]any)
	require.Equal(t, "APPROVED", metadata["dcs:state"])
	require.Equal(t, "Approved", metadata["dcs:lifecycleState"])
	require.Equal(t,
		map[string]any{"@id": creationTemplateDID},
		metadata["dcs:derivedFromTemplate"],
	)
	require.NotContains(t, metadata, "dcs:templateType")
	require.NotContains(t, published, "derivedFromTemplate")
	require.NotContains(t, published, "sourceTemplate")

	structure := published["dcs:documentStructure"].(map[string]any)
	blocks := structure["dcs:blocks"].([]any)
	require.Equal(t,
		[]any{
			"Customer ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-customer-legalName"),
			" from ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-customer-country"),
		},
		creationPipelineClauseContentByID(t, blocks, creationContractDID+"#block-clause-customer"),
	)
	require.Equal(t,
		[]any{
			"Provider ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-provider-legalName"),
			" from ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-provider-country"),
		},
		creationPipelineClauseContentByID(t, blocks, creationContractDID+"#block-clause-provider"),
	)
	require.Equal(t,
		[]any{
			"Payment ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-payment-amount"),
			" ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-payment-currency"),
		},
		creationPipelineClauseContentByID(t, blocks, creationContractDID+"#block-clause-payment"),
	)
	require.Equal(t,
		[]any{
			"Availability must be at least ",
			creationPipelinePublishedPlaceholder(creationContractDID + "#field-availability-availability"),
			" percent.",
		},
		creationPipelineClauseContentByID(t, blocks, creationContractDID+"#block-clause-availability"),
	)

	objects := published["dcs:contractData"].([]any)
	require.Equal(t, map[string]any{
		"@id":           creationContractDID + "#customer",
		"@type":         "dcs:CompanyParty",
		"dcs:legalName": map[string]any{"@type": "xsd:string", "@value": "Customer GmbH"},
		"dcs:country": map[string]any{
			"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-DEU",
		},
		"dcs:role": map[string]any{"@id": "dcs:Customer"},
	}, creationPipelineObjectByID(t, objects, creationContractDID+"#customer"))
	require.Equal(t, map[string]any{
		"@id":           creationContractDID + "#provider",
		"@type":         "dcs:CompanyParty",
		"dcs:legalName": map[string]any{"@type": "xsd:string", "@value": "Provider AG"},
		"dcs:country": map[string]any{
			"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-AUT",
		},
		"dcs:role": map[string]any{"@id": "dcs:Provider"},
	}, creationPipelineObjectByID(t, objects, creationContractDID+"#provider"))
	require.Equal(t, map[string]any{
		"@id":        creationContractDID + "#payment",
		"@type":      "dcs:PaymentTerm",
		"dcs:amount": map[string]any{"@type": "xsd:decimal", "@value": "1250"},
		"dcs:currency": map[string]any{
			"@id": "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR",
		},
	}, creationPipelineObjectByID(t, objects, creationContractDID+"#payment"))
	require.Equal(t, map[string]any{
		"@id":              creationContractDID + "#availability",
		"@type":            "dcs:SLO",
		"dcs:availability": map[string]any{"@type": "xsd:decimal", "@value": "99.9"},
	}, creationPipelineObjectByID(t, objects, creationContractDID+"#availability"))

	fields := published["dcs:contractFields"].([]any)
	assertCreationPipelineContractField(
		t,
		fields,
		creationContractDID+"#field-provider-country",
		"AUT",
		creationContractDID+"#provider",
		"dcs:country",
	)
	assertCreationPipelineContractField(
		t,
		fields,
		creationContractDID+"#field-payment-currency",
		"EUR",
		creationContractDID+"#payment",
		"dcs:currency",
	)
	assertCreationPipelineContractField(
		t,
		fields,
		creationContractDID+"#field-availability-availability",
		99.9,
		creationContractDID+"#availability",
		"dcs:availability",
	)
	assertCreationPipelinePolicyOperandsExist(t, published["dcs:policies"].([]any), fields)

	providerPolicy := creationPipelinePolicyBySuffix(
		t,
		published["dcs:policies"].([]any),
		"policy-provider-country-dach",
	)
	providerOperands := providerPolicy["odrl:constraint"].(map[string]any)["odrl:rightOperand"].([]any)
	require.Equal(t, []any{
		map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-DEU"},
		map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-AUT"},
		map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-CHE"},
	}, providerOperands)

	currencyPolicy := creationPipelinePolicyBySuffix(
		t,
		published["dcs:policies"].([]any),
		"policy-payment-currency-eur",
	)
	currencyOperand := currencyPolicy["odrl:constraint"].(map[string]any)["odrl:rightOperand"]
	require.Equal(t, map[string]any{
		"@id": "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR",
	}, currencyOperand)

	availabilityPolicy := creationPipelinePolicyBySuffix(
		t,
		published["dcs:policies"].([]any),
		"policy-availability-minimum",
	)
	availabilityOperand := availabilityPolicy["odrl:constraint"].(map[string]any)["odrl:rightOperand"].(map[string]any)
	require.Equal(t, "xsd:decimal", availabilityOperand["@type"])
	require.Equal(t, "99.9", availabilityOperand["@value"])
}

func TestWorkflowMaterializesApprovedTemplateSnapshotIntoFinalContract(t *testing.T) {
	subTemplate := creationPipelineTemplate()
	frameTemplate := map[string]any{
		"@context": subTemplate["@context"],
		"@id":      creationTemplateDID + "-frame",
		"@type":    "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":               "dcs:TemplateMetadata",
			"dcs:title":           "DACH Frame Agreement",
			"dcs:templateType":    "dcs:FrameContract",
			"dcs:templateVersion": 1,
			"dcs:subTemplates": []any{
				map[string]any{
					"@id":          creationTemplateDID,
					"dcs:version":  1,
					"dcs:name":     "DACH Service Agreement",
					"dcs:template": subTemplate,
				},
			},
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": []any{
				map[string]any{
					"@id":             creationTemplateDID + "-frame#block-service",
					"@type":           "dcs:ApprovedTemplate",
					"dcs:templateDid": creationTemplateDID,
					"dcs:version":     1,
				},
			},
			"dcs:layout": []any{
				map[string]any{
					"@id":          creationTemplateDID + "-frame#block-root",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": creationTemplateDID + "-frame#block-service"}}},
				},
				map[string]any{
					"@id":          creationTemplateDID + "-frame#block-service",
					"dcs:children": map[string]any{"@list": []any{}},
				},
			},
		},
		"dcs:contractData": []any{},
		"dcs:policies":     []any{},
	}
	frameDID := creationTemplateDID + "-frame"
	persistedFrame, err := validation.NormalizeTemplateDataForPersistence(
		newCreationPipelineJSON(t, frameTemplate),
		frameDID,
	)
	require.NoError(t, err)
	contractDraft, err := convertTemplateDataToContractData(persistedFrame, frameDID)
	require.NoError(t, err)

	var contractData map[string]any
	require.NoError(t, json.Unmarshal(*contractDraft, &contractData))
	contractData["semanticConditionValues"] = creationPipelineValues()
	persistedContract, err := validation.NormalizeContractDataForPersistence(
		newCreationPipelineJSON(t, contractData),
		creationContractDID,
		true,
	)
	require.NoError(t, err)

	now := time.Date(2026, time.June, 19, 12, 0, 0, 0, time.UTC)
	published, err := semanticmapper.MaterializeStoredContractJSONLD(contractdb.Contract{
		DID:             creationContractDID,
		ContractVersion: 1,
		State:           "APPROVED",
		CreatedBy:       "test-participant",
		CreatedAt:       now,
		UpdatedAt:       now,
		ContractData:    persistedContract,
	}, semanticmapper.DefaultProfile())
	require.NoError(t, err)

	raw, err := json.Marshal(published)
	require.NoError(t, err)
	pretty, err := json.MarshalIndent(published, "", "  ")
	require.NoError(t, err)
	t.Logf("Workflow Published JSON-LD:\n%s", pretty)
	require.NotContains(t, string(raw), "dcs:ApprovedTemplate")
	require.NotContains(t, string(raw), "dcs:subTemplates")
	require.NotContains(t, string(raw), `"dcs:template":`)
	require.NotContains(t, string(raw), "semanticConditionValues")
	require.NotContains(t, string(raw), "dcs:DataRequirement")
	require.NotContains(t, string(raw), "dcs:RequirementField")

	require.NotEmpty(t, published["dcs:contractData"])
	require.NotEmpty(t, published["dcs:contractFields"])
	require.NotEmpty(t, published["dcs:policies"])
	structure := published["dcs:documentStructure"].(map[string]any)
	blocks := structure["dcs:blocks"].([]any)
	require.NotEmpty(t, blocks)
	require.True(t, creationPipelineHasBlockType(blocks, "dcs:Section"))
	require.True(t, creationPipelineHasBlockType(blocks, "dcs:Clause"))

	fields := published["dcs:contractFields"].([]any)
	assertCreationPipelinePolicyOperandsExist(t, published["dcs:policies"].([]any), fields)
	fieldIDs := creationPipelineObjectIDs(fields)
	for _, rawBlock := range blocks {
		block := rawBlock.(map[string]any)
		content, _ := block["dcs:content"].(map[string]any)
		segments, _ := content["@list"].([]any)
		for _, segment := range segments {
			placeholder, ok := segment.(map[string]any)
			if !ok || placeholder["@type"] != "dcs:Placeholder" {
				continue
			}
			binding := placeholder["dcs:bindsTo"].(map[string]any)["@id"].(string)
			require.Truef(t, fieldIDs[binding], "placeholder binding %s must reference ContractField", binding)
		}
	}

	directTemplate, err := validation.NormalizeTemplateDataForPersistence(
		newCreationPipelineJSON(t, subTemplate),
		creationTemplateDID,
	)
	require.NoError(t, err)
	directDraft, err := convertTemplateDataToContractData(directTemplate, creationTemplateDID)
	require.NoError(t, err)
	var directData map[string]any
	require.NoError(t, json.Unmarshal(*directDraft, &directData))
	directData["semanticConditionValues"] = creationPipelineValues()
	directStored, err := validation.NormalizeContractDataForPersistence(
		newCreationPipelineJSON(t, directData),
		creationContractDID,
		true,
	)
	require.NoError(t, err)
	directPublished, err := semanticmapper.MaterializeStoredContractJSONLD(contractdb.Contract{
		DID:             creationContractDID,
		ContractVersion: 1,
		State:           "APPROVED",
		CreatedBy:       "test-participant",
		CreatedAt:       now,
		UpdatedAt:       now,
		ContractData:    directStored,
	}, semanticmapper.DefaultProfile())
	require.NoError(t, err)
	require.Equal(t, directPublished["dcs:contractData"], published["dcs:contractData"])
	require.Equal(t, directPublished["dcs:contractFields"], published["dcs:contractFields"])
	require.Equal(t,
		creationPipelinePolicySemantics(directPublished["dcs:policies"].([]any)),
		creationPipelinePolicySemantics(published["dcs:policies"].([]any)),
	)
}

func creationPipelineTemplate() map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		},
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:title":        "DACH Service Agreement",
			"dcs:templateType": "dcs:SubContract",
		},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": creationPipelineBlocks(),
			"dcs:layout": creationPipelineLayout(),
		},
		"dcs:contractData": creationPipelineRequirements(),
		"dcs:policies":     creationPipelinePolicyDefinitions(),
	}
}

func creationPipelineBlocks() []any {
	return []any{
		creationPipelineSection("party", "Party"),
		creationPipelineSection("customer", "Customer"),
		creationPipelineClause("customer", []any{
			"Customer ",
			creationPipelinePlaceholder("{{customer.legalName}}", "customer", "legalName"),
			" from ",
			creationPipelinePlaceholder("{{customer.country}}", "customer", "country"),
		}),
		creationPipelineSection("provider", "Provider"),
		creationPipelineClause("provider", []any{
			"Provider ",
			creationPipelinePlaceholder("{{provider.legalName}}", "provider", "legalName"),
			" from ",
			creationPipelinePlaceholder("{{provider.country}}", "provider", "country"),
		}),
		creationPipelineSection("payment", "Payment"),
		creationPipelineClause("payment", []any{
			"Payment ",
			creationPipelinePlaceholder("{{payment.amount}}", "payment", "amount"),
			" ",
			creationPipelinePlaceholder("{{payment.currency}}", "payment", "currency"),
		}),
		creationPipelineSection("availability", "Availability"),
		creationPipelineClause("availability", []any{
			"Availability must be at least ",
			creationPipelinePlaceholder("{{availability.availability}}", "availability", "availability"),
			" percent.",
		}),
	}
}

func creationPipelineLayout() []any {
	return []any{
		creationPipelineLayoutNode("root", true, "party", "payment", "availability"),
		creationPipelineLayoutNode("party", false, "customer", "provider"),
		creationPipelineLayoutNode("customer", false, "clause-customer"),
		creationPipelineLayoutNode("clause-customer", false),
		creationPipelineLayoutNode("provider", false, "clause-provider"),
		creationPipelineLayoutNode("clause-provider", false),
		creationPipelineLayoutNode("payment", false, "clause-payment"),
		creationPipelineLayoutNode("clause-payment", false),
		creationPipelineLayoutNode("availability", false, "clause-availability"),
		creationPipelineLayoutNode("clause-availability", false),
	}
}

func creationPipelineRequirements() []any {
	return []any{
		creationPipelineRequirement("customer", "Customer", "CompanyParty", "customer",
			creationPipelineField("customer", "legalName", "company.legalName"),
			creationPipelineField("customer", "country", "company.location.country"),
		),
		creationPipelineRequirement("provider", "Provider", "CompanyParty", "provider",
			creationPipelineField("provider", "legalName", "company.legalName"),
			creationPipelineField("provider", "country", "company.location.country"),
		),
		creationPipelineRequirement("payment", "Payment", "PaymentTerm", "",
			creationPipelineField("payment", "amount", "contract.payment.amount"),
			creationPipelineField("payment", "currency", "contract.payment.currency"),
		),
		creationPipelineRequirement("availability", "Availability", "SLO", "",
			creationPipelineField("availability", "availability", "service.sla.availability"),
		),
	}
}

func creationPipelinePolicyDefinitions() []any {
	return []any{
		creationPipelinePolicy(
			"provider-country-dach",
			"provider",
			"country",
			"odrl:isAnyOf",
			[]any{
				map[string]any{"@value": "DEU", "@type": "xsd:string"},
				map[string]any{"@value": "AUT", "@type": "xsd:string"},
				map[string]any{"@value": "CHE", "@type": "xsd:string"},
			},
		),
		creationPipelinePolicy(
			"payment-currency-eur",
			"payment",
			"currency",
			"odrl:eq",
			map[string]any{"@value": "EUR", "@type": "xsd:string"},
		),
		creationPipelinePolicy(
			"availability-minimum",
			"availability",
			"availability",
			"odrl:gteq",
			map[string]any{"@value": 99.9, "@type": "xsd:decimal"},
		),
	}
}

func creationPipelineValues() []any {
	return []any{
		creationPipelineValue("clause-customer", "customer", "legalName", "Customer GmbH"),
		creationPipelineValue("clause-customer", "customer", "country", "DEU"),
		creationPipelineValue("clause-provider", "provider", "legalName", "Provider AG"),
		creationPipelineValue("clause-provider", "provider", "country", "AUT"),
		creationPipelineValue("clause-payment", "payment", "amount", 1250.0),
		creationPipelineValue("clause-payment", "payment", "currency", "EUR"),
		creationPipelineValue("clause-availability", "availability", "availability", 99.9),
	}
}

func assertCreationPipelineLayout(t *testing.T, raw *datatype.JSON) {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	layout := structure["dcs:layout"].([]any)

	require.Equal(t,
		[]string{"party", "payment", "availability"},
		creationPipelineChildNames(t, layout, "root"),
	)
	require.Equal(t,
		[]string{"customer", "provider"},
		creationPipelineChildNames(t, layout, "party"),
	)
	for _, group := range []string{"customer", "provider", "payment", "availability"} {
		require.Equal(t,
			[]string{"clause-" + group},
			creationPipelineChildNames(t, layout, group),
		)
	}
}

func assertCreationPipelinePolicies(t *testing.T, raw *datatype.JSON) {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policies := data["dcs:policies"].([]any)

	dach := creationPipelinePolicyBySuffix(t, policies, "policy-provider-country-dach")
	dachConstraint := dach["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:isAnyOf", dachConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, []any{
		map[string]any{"@value": "DEU", "@type": "xsd:string"},
		map[string]any{"@value": "AUT", "@type": "xsd:string"},
		map[string]any{"@value": "CHE", "@type": "xsd:string"},
	}, dachConstraint["odrl:rightOperand"])

	currency := creationPipelinePolicyBySuffix(t, policies, "policy-payment-currency-eur")
	currencyConstraint := currency["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:eq", currencyConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, map[string]any{"@value": "EUR", "@type": "xsd:string"}, currencyConstraint["odrl:rightOperand"])

	availability := creationPipelinePolicyBySuffix(t, policies, "policy-availability-minimum")
	availabilityConstraint := availability["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:gteq", availabilityConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, map[string]any{"@value": 99.9, "@type": "xsd:decimal"}, availabilityConstraint["odrl:rightOperand"])
}

func creationPipelineSection(id string, title string) map[string]any {
	return map[string]any{
		"@id":       creationTemplateDID + "#block-" + id,
		"@type":     "dcs:Section",
		"dcs:title": title,
	}
}

func creationPipelineClause(group string, content []any) map[string]any {
	return map[string]any{
		"@id":         creationTemplateDID + "#block-clause-" + group,
		"@type":       "dcs:Clause",
		"dcs:content": map[string]any{"@list": content},
	}
}

func creationPipelinePlaceholder(token string, conditionID string, parameterName string) map[string]any {
	return map[string]any{
		"@type":       "dcs:Placeholder",
		"dcs:token":   token,
		"dcs:bindsTo": map[string]any{"@id": creationPipelineFieldID(conditionID, parameterName)},
	}
}

func creationPipelineLayoutNode(id string, root bool, children ...string) map[string]any {
	childRefs := make([]any, 0, len(children))
	for _, child := range children {
		childRefs = append(childRefs, map[string]any{"@id": creationTemplateDID + "#block-" + child})
	}
	node := map[string]any{
		"@id":          creationTemplateDID + "#block-" + id,
		"dcs:children": map[string]any{"@list": childRefs},
	}
	if root {
		node["dcs:isRoot"] = true
	}
	return node
}

func creationPipelineRequirement(
	conditionID string,
	name string,
	entityType string,
	entityRole string,
	fields ...map[string]any,
) map[string]any {
	rawFields := make([]any, len(fields))
	for index, field := range fields {
		rawFields[index] = field
	}
	requirement := map[string]any{
		"@id":               creationTemplateDID + "#requirement-" + conditionID,
		"@type":             "dcs:DataRequirement",
		"dcs:conditionId":   conditionID,
		"dcs:name":          name,
		"dcs:schemaVersion": "v1",
		"dcs:entityType":    entityType,
		"dcs:fields":        rawFields,
	}
	if entityRole != "" {
		requirement["dcs:entityRole"] = entityRole
	}
	return requirement
}

func creationPipelineField(conditionID string, parameterName string, semanticPath string) map[string]any {
	return map[string]any{
		"@id":               creationPipelineFieldID(conditionID, parameterName),
		"@type":             "dcs:RequirementField",
		"dcs:parameterName": parameterName,
		"dcs:domainField": map[string]any{
			"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-" + creationPipelineSlug(semanticPath),
		},
		"dcs:semanticPath": semanticPath,
		"dcs:required":     true,
	}
}

func creationPipelineFieldID(conditionID string, parameterName string) string {
	return creationTemplateDID + "#field-" + conditionID + "-" + parameterName
}

func creationPipelinePolicy(
	id string,
	conditionID string,
	parameterName string,
	operator string,
	rightOperand any,
) map[string]any {
	return map[string]any{
		"@id":   creationTemplateDID + "#policy-" + id,
		"@type": "odrl:Duty",
		"odrl:constraint": map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": creationPipelineFieldID(conditionID, parameterName)},
			"odrl:operator":     map[string]any{"@id": operator},
			"odrl:rightOperand": rightOperand,
		},
	}
}

func creationPipelineValue(blockID string, conditionID string, parameterName string, value any) map[string]any {
	return map[string]any{
		"blockId":        blockID,
		"conditionId":    conditionID,
		"parameterName":  parameterName,
		"parameterValue": value,
	}
}

func creationPipelineChildNames(t *testing.T, layout []any, parent string) []string {
	t.Helper()
	parentID := creationContractDID + "#block-" + parent
	for _, rawNode := range layout {
		node := rawNode.(map[string]any)
		if node["@id"] != parentID {
			continue
		}
		children := node["dcs:children"].(map[string]any)["@list"].([]any)
		result := make([]string, 0, len(children))
		for _, rawChild := range children {
			childID := rawChild.(map[string]any)["@id"].(string)
			result = append(result, childID[len(creationContractDID+"#block-"):])
		}
		return result
	}
	require.Failf(t, "layout node not found", "missing %s", parentID)
	return nil
}

func creationPipelinePolicyBySuffix(t *testing.T, policies []any, suffix string) map[string]any {
	t.Helper()
	expectedID := creationContractDID + "#" + suffix
	for _, rawPolicy := range policies {
		policy := rawPolicy.(map[string]any)
		if policy["@id"] == expectedID {
			return policy
		}
	}
	require.Failf(t, "policy not found", "missing %s", expectedID)
	return nil
}

func creationPipelineObjectByID(t *testing.T, objects []any, id string) map[string]any {
	t.Helper()
	for _, rawObject := range objects {
		object := rawObject.(map[string]any)
		if object["@id"] == id {
			return object
		}
	}
	require.Failf(t, "contract data object not found", "missing %s", id)
	return nil
}

func creationPipelineObjectIDs(objects []any) map[string]bool {
	result := map[string]bool{}
	for _, rawObject := range objects {
		object, ok := rawObject.(map[string]any)
		if !ok {
			continue
		}
		id, _ := object["@id"].(string)
		result[id] = true
	}
	return result
}

func creationPipelineHasBlockType(blocks []any, blockType string) bool {
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if ok && block["@type"] == blockType {
			return true
		}
	}
	return false
}

func creationPipelinePolicySemantics(policies []any) []any {
	result := make([]any, 0, len(policies))
	for _, rawPolicy := range policies {
		policy, ok := rawPolicy.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, policy["odrl:constraint"])
	}
	return result
}

func assertCreationPipelineContractField(
	t *testing.T,
	fields []any,
	id string,
	value any,
	sourceObjectID string,
	path string,
) {
	t.Helper()
	field := creationPipelineObjectByID(t, fields, id)
	require.Equal(t, "dcs:ContractField", field["@type"])
	require.Equal(t, map[string]any{"@id": sourceObjectID}, field["dcs:sourceObject"])
	require.Equal(t, path, field["dcs:path"])
	domainField := field["dcs:domainField"].(map[string]any)
	require.Equal(t,
		map[string]any{"@id": validation.SemanticDataType(domainField["@id"].(string))},
		field["dcs:dataType"],
	)
	require.NotContains(t, field, "dcs:value")
}

func assertCreationPipelinePolicyOperandsExist(t *testing.T, policies []any, fields []any) {
	t.Helper()
	fieldIDs := map[string]bool{}
	for _, rawField := range fields {
		fieldIDs[rawField.(map[string]any)["@id"].(string)] = true
	}
	for _, rawPolicy := range policies {
		policy := rawPolicy.(map[string]any)
		constraint := policy["odrl:constraint"].(map[string]any)
		leftOperand := constraint["odrl:leftOperand"].(map[string]any)["@id"].(string)
		require.Truef(t, fieldIDs[leftOperand], "policy operand %s must reference an existing ContractField", leftOperand)
		require.Contains(t, leftOperand, creationContractDID+"#field-")
		require.NotContains(t, leftOperand, creationTemplateDID)
	}
}

func creationPipelineMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func creationPipelineSlug(value string) string {
	result := make([]rune, 0, len(value))
	separator := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			if separator && len(result) > 0 {
				result = append(result, '-')
			}
			result = append(result, char)
			separator = false
			continue
		}
		separator = true
	}
	return string(result)
}

func creationPipelineClauseContentByID(t *testing.T, blocks []any, id string) []any {
	t.Helper()
	for _, rawBlock := range blocks {
		block := rawBlock.(map[string]any)
		if block["@id"] != id {
			continue
		}
		return block["dcs:content"].(map[string]any)["@list"].([]any)
	}
	require.Failf(t, "clause not found", "missing %s", id)
	return nil
}

func creationPipelinePublishedPlaceholder(fieldID string) map[string]any {
	return map[string]any{
		"@type":       "dcs:Placeholder",
		"dcs:bindsTo": map[string]any{"@id": fieldID},
	}
}

func newCreationPipelineJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &raw
}
