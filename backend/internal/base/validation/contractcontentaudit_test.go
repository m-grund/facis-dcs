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
		"service": map[string]any{
			"sla": map[string]any{
				"availability":   99.95,
				"responseTime":   10,
				"resolutionTime": 120,
			},
		},
		"signature": map[string]any{
			"requiredLevel": "AES",
		},
	}

	findings, err := AuditContractContent(contract, nil, ContractContentAuditMetadata{})
	policy, policyErr := normalizeContractContentPolicy(nil, ContractContentAuditMetadata{})

	require.NoError(t, err)
	require.NoError(t, policyErr)
	require.NotEmpty(t, policy.SHACLFiles)
	require.NotEmpty(t, policy.SHACLShapes)
	require.NotEmpty(t, findings)
	require.Contains(t, policyFindingRuleIDs(findings), "dcs:ContractShape-PROP-002")
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

func hasFindingSeverity(findings []PolicyFinding, ruleID string, severity string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity {
			return true
		}
	}
	return false
}
