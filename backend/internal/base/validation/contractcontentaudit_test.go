package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func odrlContract(fieldID, conditionID, parameterName string, policies []any, actualValue any) map[string]any {
	return map[string]any{
		"dcs:contractData": []any{
			map[string]any{
				"@id":             "urn:dcs:req:test",
				"@type":           "dcs:DataRequirement",
				"dcs:conditionId": conditionID,
				"dcs:fields": []any{
					map[string]any{
						"@id":               fieldID,
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": parameterName,
					},
				},
			},
		},
		"dcs:policies":            policies,
		"semanticConditionValues": []any{map[string]any{"conditionId": conditionID, "parameterName": parameterName, "parameterValue": actualValue}},
	}
}

func odrlDuty(id, fieldID, operator string, rightOperand any) map[string]any {
	return map[string]any{
		"@id":   id,
		"@type": "odrl:Duty",
		"odrl:constraint": map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": fieldID},
			"odrl:operator":     map[string]any{"@id": operator},
			"odrl:rightOperand": rightOperand,
		},
	}
}

func emptyPolicy() map[string]any {
	return map[string]any{"policySetId": "facis.dcs.contract.content.static", "version": "test"}
}

func TestAuditContractContentFlagsBlacklistedCountry(t *testing.T) {
	fieldID := "urn:dcs:field:provider-country"
	contract := odrlContract(fieldID, "provider", "country",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-001", fieldID, "odrl:isNoneOf", []any{"RUS"})},
		"RUS",
	)

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-CONTRACT-STATIC-001")
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-001", "error"))
}

func TestAuditContractContentAcceptsCompliantContract(t *testing.T) {
	countryFieldID := "urn:dcs:field:company-country"
	lawFieldID := "urn:dcs:field:contract-law"
	paymentFieldID := "urn:dcs:field:payment-amount"

	contract := map[string]any{
		"@context": []any{"https://w3id.org/facis/sla/ontology"},
		"@id":      "urn:facis:dcs:contract:sla:example-001",
		"@type":    []any{"dcs:Contract", "sla:ServiceLevelAgreement"},
		"dcs:contractData": []any{
			map[string]any{
				"@id": "urn:dcs:req:company", "@type": "dcs:DataRequirement", "dcs:conditionId": "company",
				"dcs:fields": []any{map[string]any{"@id": countryFieldID, "@type": "dcs:RequirementField", "dcs:parameterName": "country"}},
			},
			map[string]any{
				"@id": "urn:dcs:req:contract", "@type": "dcs:DataRequirement", "dcs:conditionId": "contract",
				"dcs:fields": []any{
					map[string]any{"@id": lawFieldID, "@type": "dcs:RequirementField", "dcs:parameterName": "governingLaw"},
					map[string]any{"@id": paymentFieldID, "@type": "dcs:RequirementField", "dcs:parameterName": "amount"},
				},
			},
		},
		"dcs:policies": []any{
			odrlDuty("FACIS-CONTRACT-STATIC-001", countryFieldID, "odrl:isNoneOf", []any{"RUS"}),
			odrlDuty("FACIS-CONTRACT-STATIC-002", lawFieldID, "odrl:isAnyOf", []any{"DE", "AT", "CH"}),
			odrlDuty("FACIS-CONTRACT-STATIC-003", paymentFieldID, "odrl:lteq", float64(10000)),
		},
		"semanticConditionValues": []any{
			map[string]any{"conditionId": "company", "parameterName": "country", "parameterValue": "DEU"},
			map[string]any{"conditionId": "contract", "parameterName": "governingLaw", "parameterValue": "DE"},
			map[string]any{"conditionId": "contract", "parameterName": "amount", "parameterValue": float64(9500)},
		},
	}

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
}

func TestAuditContractContentFlagsExceededMaximum(t *testing.T) {
	fieldID := "urn:dcs:field:liability-cap"
	contract := odrlContract(fieldID, "liability", "capAmount",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-003", fieldID, "odrl:lteq", float64(100000))},
		float64(150000),
	)

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-003", "error"))
}

func TestAuditContractContentFlagsInvalidJurisdiction(t *testing.T) {
	fieldID := "urn:dcs:field:jurisdiction"
	contract := odrlContract(fieldID, "contract", "jurisdiction",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-COUNTRY", fieldID, "odrl:isAnyOf", []any{"DEU", "AUT", "CHE"})},
		"ZZZ",
	)

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-COUNTRY", "error"))
}

func TestAuditContractContentAcceptsValidJurisdiction(t *testing.T) {
	fieldID := "urn:dcs:field:jurisdiction"
	contract := odrlContract(fieldID, "contract", "jurisdiction",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-COUNTRY", fieldID, "odrl:isAnyOf", []any{"DEU", "AUT", "CHE"})},
		"DEU",
	)

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-COUNTRY", "info"))
}

func TestAuditContractContentLoadsDefaultPolicyDocument(t *testing.T) {
	contract := canonicalAuditContract()
	contract["semanticConditionValues"] = append(contract["semanticConditionValues"].([]any),
		map[string]any{"conditionId": "condition-legal", "parameterName": "contract.jurisdiction", "parameterValue": "DEU"},
		map[string]any{"conditionId": "condition-service", "parameterName": "service.sla.availability", "parameterValue": 99.95},
		map[string]any{"conditionId": "condition-service", "parameterName": "service.sla.responseTime", "parameterValue": 10},
		map[string]any{"conditionId": "condition-service", "parameterName": "service.sla.resolutionTime", "parameterValue": 120},
		map[string]any{"conditionId": "condition-signature", "parameterName": "signature.requiredLevel", "parameterValue": "AES"},
	)

	findings, err := AuditContractContent(contract, nil, ContractContentAuditMetadata{})
	policy, policyErr := normalizeContractContentPolicy(nil, ContractContentAuditMetadata{})

	require.NoError(t, err)
	require.NoError(t, policyErr)
	require.NotEmpty(t, policy.SHACLFiles)
	require.NotEmpty(t, policy.SHACLShapes)
	require.NotEmpty(t, findings)
	require.Contains(t, policyFindingRuleIDs(findings), "dcs:CanonicalContractShape-PROP-002")
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-POLICY-003", "info"))
}

func TestAuditContractContentValidatesJSONLDAndSHACL(t *testing.T) {
	contract := map[string]any{
		"@context": []any{"https://w3id.org/facis/sla/ontology"},
		"@id":      "urn:facis:dcs:contract:sla:example-001",
		"@type":    []any{"dcs:Contract", "sla:ServiceLevelAgreement"},
		"parties": []any{
			map[string]any{"@type": "dcs:CompanyParty", "role": "supplier"},
			map[string]any{"@type": "dcs:CompanyParty", "role": "customer"},
		},
		"contract": map[string]any{
			"jurisdiction": "DEU",
		},
	}
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.structure-semantics",
		"version":     "test",
		"shaclShapes": []any{
			map[string]any{
				"id":          "FACIS-CONTRACT-SHACL-SLA",
				"title":       "SLA contract must satisfy semantic shape",
				"targetClass": "dcs:Contract",
				"properties": []any{
					map[string]any{"path": "contract.jurisdiction", "minCount": 1, "datatype": "xsd:string", "name": "Jurisdiction"},
					map[string]any{"path": "parties", "minCount": 2, "class": "dcs:CompanyParty", "name": "Contract parties"},
				},
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-SHACL-SLA-PROP-001", "info"))
	require.False(t, hasFindingSeverity(findings, "FACIS-CONTRACT-SHACL-SLA-PROP-002", "error"))
}

func TestAuditContractContentFlagsSHACLViolations(t *testing.T) {
	contract := map[string]any{
		"@context": "https://w3id.org/facis/sla/ontology",
		"@id":      "urn:facis:dcs:contract:sla:example-001",
		"@type":    "dcs:Contract",
		"parties": []any{
			map[string]any{"@type": "dcs:Organization", "role": "supplier"},
		},
	}
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.structure-semantics",
		"version":     "test",
		"shacl": map[string]any{
			"shapes": []any{
				map[string]any{
					"id":          "FACIS-CONTRACT-SHACL-SLA",
					"title":       "SLA contract must satisfy semantic shape",
					"targetClass": "dcs:Contract",
					"property": []any{
						map[string]any{"path": "contract.jurisdiction", "minCount": 1, "name": "Jurisdiction"},
						map[string]any{"path": "parties", "class": "dcs:CompanyParty", "name": "Contract parties"},
					},
				},
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-SHACL-SLA-PROP-001", "error"))
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-SHACL-SLA-PROP-002", "error"))
}

func TestAuditContractContentEvaluatesExternalODRLPolicies(t *testing.T) {
	fieldID := "urn:dcs:field:provider-country"
	contract := odrlContract(fieldID, "provider", "country", nil, "RUS")

	policy := map[string]any{
		"policySetId": "facis.dcs.contract.content.static",
		"version":     "test",
		"dcs:policies": []any{
			odrlDuty("FACIS-EXT-001", fieldID, "odrl:isNoneOf", []any{"RUS"}),
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-EXT-001", "error"))
}

func TestAuditContractContentAcceptsCanonicalContractODRLValues(t *testing.T) {
	contract := canonicalAuditContract()

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "info"))
	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-postal-code", "info"))
	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
}

func TestAuditContractContentFlagsCanonicalContractODRLViolation(t *testing.T) {
	contract := canonicalAuditContract()
	values := contract["semanticConditionValues"].([]any)
	values[0].(map[string]any)["parameterValue"] = "USA"

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "error"))
}

func TestAuditContractContentFlagsCanonicalContractMissingSemanticValue(t *testing.T) {
	contract := canonicalAuditContract()
	contract["semanticConditionValues"] = []any{}

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "error"))
	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-postal-code", "error"))
	finding := requirePolicyFinding(t, findings, "urn:uuid:policy-country")
	require.Equal(t, "in", finding.Operator)
	require.Equal(t, []any{"DEU", "AUT", "CHE"}, finding.ExpectedValues)
	require.Empty(t, finding.ActualValue)
	require.Contains(t, finding.Requirement, "must be one of DEU, AUT, CHE")
}

func TestAuditContractContentFlagsCanonicalPolicyWithUnknownField(t *testing.T) {
	contract := canonicalAuditContract()
	policy := contract["dcs:policies"].([]any)[0].(map[string]any)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:missing-field"}

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "error"))
}

func TestValidateContractPolicySatisfactionAcceptsSatisfiedEmbeddedODRLPolicies(t *testing.T) {
	contract := canonicalAuditContract()

	err := ValidateContractPolicySatisfaction(contract, ContractContentAuditMetadata{})

	require.NoError(t, err)
}

func TestValidateContractPolicySatisfactionRejectsEmbeddedODRLViolation(t *testing.T) {
	contract := canonicalAuditContract()
	values := contract["semanticConditionValues"].([]any)
	values[0].(map[string]any)["parameterValue"] = "USA"

	err := ValidateContractPolicySatisfaction(contract, ContractContentAuditMetadata{})

	var policyErr ContractPolicySatisfactionError
	require.ErrorAs(t, err, &policyErr)
	require.Len(t, policyErr.Findings, 1)
	require.Equal(t, "urn:uuid:policy-country", policyErr.Findings[0].RuleID)
	require.Equal(t, "error", policyErr.Findings[0].Severity)
	require.Contains(t, err.Error(), "contract policy validation failed")
}

func TestValidateContractPolicySatisfactionRejectsMissingRequiredEmbeddedODRLValue(t *testing.T) {
	contract := canonicalAuditContract()
	contract["semanticConditionValues"] = []any{}

	err := ValidateContractPolicySatisfaction(contract, ContractContentAuditMetadata{})

	var policyErr ContractPolicySatisfactionError
	require.ErrorAs(t, err, &policyErr)
	require.Len(t, policyErr.Findings, 2)
	require.Equal(t, "urn:uuid:policy-country", policyErr.Findings[0].RuleID)
	require.Equal(t, "urn:uuid:policy-postal-code", policyErr.Findings[1].RuleID)
}

func TestAuditContractContentMapsCanonicalSemanticValuesToPartyShape(t *testing.T) {
	contract := canonicalAuditContractWithTemplateParties()
	minCount := 2
	maxCount := 2
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.structure-semantics",
		"version":     "test",
		"shacl": map[string]any{
			"shapes": []any{
				map[string]any{
					"id":          "dcs:ContractShape",
					"title":       "Contract shape",
					"targetClass": "dcs:Contract",
					"property": []any{
						map[string]any{
							"id":       "dcs:ContractShape-PARTY",
							"path":     "dcs:party",
							"minCount": minCount,
							"maxCount": maxCount,
							"class":    "dcs:CompanyParty",
							"node":     "dcs:CompanyPartyShape",
						},
					},
				},
				map[string]any{
					"id":          "dcs:CompanyPartyShape",
					"title":       "Company party shape",
					"targetClass": "dcs:CompanyParty",
					"property": []any{
						map[string]any{"id": "dcs:CompanyPartyShape-ROLE", "path": "dcs:role", "minCount": 1, "maxCount": 1, "datatype": "xsd:string", "in": []any{"provider", "customer"}},
						map[string]any{"id": "dcs:CompanyPartyShape-LEGAL", "path": "dcs:legalName", "minCount": 1, "maxCount": 1, "datatype": "xsd:string"},
					},
				},
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
	require.True(t, hasFindingSeverity(findings, "dcs:ContractShape-PARTY", "info"))
}

func TestAuditContractContentReadsCanonicalRuntimeValuesBySemanticPath(t *testing.T) {
	contract := canonicalAuditContractWithTemplateParties()
	findings := auditContractValidationProfile(contract, ValidationProfile{
		ID:      "runtime-values",
		Version: "test",
		Rules: []ValidationRule{
			{
				ID:       "jurisdiction-allowed",
				Type:     ValidationRuleValueIn,
				Severity: "error",
				Target:   "contract.jurisdiction",
				Values:   []string{"DEU", "AUT", "CHE"},
			},
			{
				ID:       "availability-minimum",
				Type:     ValidationRuleComparison,
				Severity: "error",
				Target:   "service.sla.availability",
				Operator: "gte",
				Value:    99.5,
			},
		},
	})

	require.True(t, hasFindingSeverity(findings, "jurisdiction-allowed", "info"))
	require.True(t, hasFindingSeverity(findings, "availability-minimum", "info"))
	availabilityFinding := requirePolicyFinding(t, findings, "availability-minimum")
	require.Equal(t, "gte", availabilityFinding.Operator)
	require.Equal(t, 99.5, availabilityFinding.ActualValue)
	require.Equal(t, 99.5, availabilityFinding.ExpectedValue)
	require.Equal(t, "service.sla.availability must be >= 99.5", availabilityFinding.Requirement)
	jurisdictionFinding := requirePolicyFinding(t, findings, "jurisdiction-allowed")
	require.Equal(t, "in", jurisdictionFinding.Operator)
	require.Equal(t, []any{"DEU", "AUT", "CHE"}, jurisdictionFinding.ExpectedValues)
	require.Equal(t, "DEU", jurisdictionFinding.ActualValue)
}

func TestAuditContractContentShowsPolicyDetailsForMissingRuntimeValue(t *testing.T) {
	contract := canonicalAuditContractWithTemplateParties()
	values := contract["semanticConditionValues"].([]any)
	contract["semanticConditionValues"] = values[:len(values)-2]

	findings := auditContractValidationProfile(contract, ValidationProfile{
		ID:      "runtime-values",
		Version: "test",
		Rules: []ValidationRule{
			{
				ID:       "availability-minimum",
				Type:     ValidationRuleComparison,
				Severity: "error",
				Message:  "Service availability must satisfy policy minimum.",
				Target:   "service.sla.availability",
				Operator: "gte",
				Value:    99.9,
			},
		},
	})

	finding := requirePolicyFinding(t, findings, "availability-minimum")
	require.Equal(t, "error", finding.Severity)
	require.Equal(t, "gte", finding.Operator)
	require.Equal(t, 99.9, finding.ExpectedValue)
	require.Empty(t, finding.ActualValue)
	require.Equal(t, "service.sla.availability must be >= 99.9", finding.Requirement)
}

func TestAuditContractContentShowsPolicyDetailsForLowRuntimeValue(t *testing.T) {
	contract := canonicalAuditContractWithTemplateParties()

	findings := auditContractValidationProfile(contract, ValidationProfile{
		ID:      "runtime-values",
		Version: "test",
		Rules: []ValidationRule{
			{
				ID:       "availability-minimum",
				Type:     ValidationRuleComparison,
				Severity: "error",
				Target:   "service.sla.availability",
				Operator: "gte",
				Value:    99.9,
			},
		},
	})

	finding := requirePolicyFinding(t, findings, "availability-minimum")
	require.Equal(t, "error", finding.Severity)
	require.Equal(t, "gte", finding.Operator)
	require.Equal(t, 99.5, finding.ActualValue)
	require.Equal(t, 99.9, finding.ExpectedValue)
	require.Equal(t, "service.sla.availability must be >= 99.9", finding.Requirement)
}

func TestAuditContractContentShowsPolicyDetailsForTypedODRLRightOperand(t *testing.T) {
	contract := canonicalAuditContract()

	findings, err := AuditContractContent(contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	finding := requirePolicyFinding(t, findings, "urn:uuid:policy-postal-code")
	require.Equal(t, "eq", finding.Operator)
	require.Equal(t, "91448", finding.ActualValue)
	require.Equal(t, "91448", finding.ExpectedValue)
	require.Equal(t, "urn:uuid:field-company-postal-code must equal 91448", finding.Requirement)
}

func TestAuditContractContentShowsPolicyDetailsForSHACLConstraints(t *testing.T) {
	contract := map[string]any{
		"@type": "dcs:Contract",
		"parties": []any{
			map[string]any{"@type": "dcs:CompanyParty"},
		},
		"service": map[string]any{"sla": map[string]any{"availability": 98.5}},
	}
	minCount := 2
	minInclusive := 99.9
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.structure-semantics",
		"version":     "test",
		"shaclShapes": []any{
			map[string]any{
				"id":          "FACIS-CONTRACT-SHACL-SLA",
				"title":       "SLA contract must satisfy semantic shape",
				"targetClass": "dcs:Contract",
				"properties": []any{
					map[string]any{"id": "party-count", "path": "parties", "minCount": minCount, "name": "Contract parties"},
					map[string]any{"id": "availability-min", "path": "service.sla.availability", "minInclusive": minInclusive, "name": "Availability"},
				},
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	partyFinding := requirePolicyFinding(t, findings, "party-count")
	require.Equal(t, "minCount", partyFinding.Operator)
	require.Equal(t, 1, partyFinding.ActualValue)
	require.Equal(t, minCount, partyFinding.ExpectedValue)
	require.Equal(t, "parties requires at least 2 value(s)", partyFinding.Requirement)
	availabilityFinding := requirePolicyFinding(t, findings, "availability-min")
	require.Equal(t, "gte", availabilityFinding.Operator)
	require.Equal(t, 98.5, availabilityFinding.ActualValue)
	require.Equal(t, minInclusive, availabilityFinding.ExpectedValue)
	require.Equal(t, "service.sla.availability must be >= 99.9", availabilityFinding.Requirement)
}

func canonicalAuditContract() map[string]any {
	countryFieldID := "urn:uuid:field-company-country"
	postalCodeFieldID := "urn:uuid:field-company-postal-code"
	return map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "did:example:contract",
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@type":       "dcs:ContractMetadata",
			"dcs:title":   "Canonical audit contract",
			"dcs:version": 1,
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "urn:uuid:block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Company country: ",
						map[string]any{"@type": "dcs:Placeholder", "dcs:bindsTo": map[string]any{"@id": countryFieldID}},
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
				"@id":               "urn:uuid:req-company",
				"@type":             "dcs:DataRequirement",
				"dcs:conditionId":   "company",
				"dcs:name":          "Company",
				"dcs:schemaVersion": "v1",
				"dcs:fields": []any{
					map[string]any{
						"@id":               countryFieldID,
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": "country",
						"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
						"dcs:required":      true,
					},
					map[string]any{
						"@id":               postalCodeFieldID,
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": "postalCode",
						"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-postalCode"},
						"dcs:required":      true,
					},
				},
			},
		},
		"dcs:policies": []any{
			odrlDuty("urn:uuid:policy-country", countryFieldID, "odrl:isAnyOf", []any{
				map[string]any{"@type": "xsd:string", "@value": "DEU"},
				map[string]any{"@type": "xsd:string", "@value": "AUT"},
				map[string]any{"@type": "xsd:string", "@value": "CHE"},
			}),
			odrlDuty("urn:uuid:policy-postal-code", postalCodeFieldID, "odrl:eq", map[string]any{"@type": "xsd:string", "@value": "91448"}),
		},
		"semanticConditionValues": []any{
			map[string]any{"blockId": "urn:uuid:block-clause-1", "conditionId": "company", "parameterName": "country", "parameterValue": "DEU"},
			map[string]any{"blockId": "urn:uuid:block-clause-1", "conditionId": "company", "parameterName": "postalCode", "parameterValue": "91448"},
		},
	}
}

func canonicalAuditContractWithTemplateParties() map[string]any {
	contract := canonicalAuditContract()
	contract["dcs:contractData"] = []any{}
	contract["dcs:policies"] = []any{}
	contract["dcs:metadata"] = map[string]any{
		"@type":     "dcs:ContractMetadata",
		"dcs:title": "Canonical audit contract",
		"dcs:subTemplates": []any{
			map[string]any{
				"@id":         "did:example:template",
				"dcs:version": 1,
				"dcs:template": map[string]any{
					"@type": "dcs:ContractTemplate",
					"dcs:contractData": []any{
						companyPartyRequirement("condition-customer", "customer"),
						companyPartyRequirement("condition-provider", "provider"),
					},
				},
			},
		},
	}
	contract["semanticConditionValues"] = []any{
		map[string]any{"conditionId": "condition-customer", "parameterName": "company.legalName", "parameterValue": "Firma A"},
		map[string]any{"conditionId": "condition-customer", "parameterName": "company.location.country", "parameterValue": "DEU"},
		map[string]any{"conditionId": "condition-provider", "parameterName": "company.legalName", "parameterValue": "Firma B"},
		map[string]any{"conditionId": "condition-provider", "parameterName": "company.location.country", "parameterValue": "DEU"},
		map[string]any{"conditionId": "condition-service", "parameterName": "service.sla.availability", "parameterValue": 99.5},
		map[string]any{"conditionId": "condition-legal", "parameterName": "contract.jurisdiction", "parameterValue": "DEU"},
	}
	return contract
}

func companyPartyRequirement(conditionID string, role string) map[string]any {
	return map[string]any{
		"@id":               "urn:uuid:req-" + conditionID,
		"@type":             "dcs:DataRequirement",
		"dcs:conditionId":   conditionID,
		"dcs:name":          role + " party",
		"dcs:schemaVersion": "v1",
		"dcs:entityType":    "CompanyParty",
		"dcs:entityRole":    role,
		"dcs:fields": []any{
			map[string]any{
				"@id":               "urn:uuid:field-" + conditionID + "-legal-name",
				"@type":             "dcs:RequirementField",
				"dcs:parameterName": "company.legalName",
				"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-legalName"},
				"dcs:required":      true,
			},
			map[string]any{
				"@id":               "urn:uuid:field-" + conditionID + "-country",
				"@type":             "dcs:RequirementField",
				"dcs:parameterName": "company.location.country",
				"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
				"dcs:required":      true,
			},
		},
	}
}

func hasFindingSeverity(findings []PolicyFinding, ruleID string, severity string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity {
			return true
		}
	}
	return false
}

func requirePolicyFinding(t *testing.T, findings []PolicyFinding, ruleID string) PolicyFinding {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	require.Failf(t, "finding not found", "ruleID %q not found in %v", ruleID, policyFindingRuleIDs(findings))
	return PolicyFinding{}
}
