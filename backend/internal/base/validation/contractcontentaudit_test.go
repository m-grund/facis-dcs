package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditContractContentFlagsBlacklistedCountry(t *testing.T) {
	contract := map[string]any{
		"@id": "urn:facis:dcs:contract:example-001",
		"provider": map[string]any{
			"company": map[string]any{
				"location": map[string]any{
					"country": "RUS",
				},
			},
		},
		"contract": map[string]any{
			"jurisdiction": "DEU",
		},
		"signature": map[string]any{
			"requiredLevel": "AES",
		},
	}
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.content.static",
		"version":     "test",
		"rules": []any{
			map[string]any{
				"id":           "FACIS-CONTRACT-STATIC-001",
				"title":        "Provider country must not be blacklisted",
				"builtin":      "value_not_in",
				"semanticPath": "provider.company.location.country",
				"values":       []any{"RUS"},
				"ontologyTerm": "dcs:CountryCode",
				"requirement":  "DCS-FR-PACM-03",
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-CONTRACT-STATIC-001")
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-001", "error"))
}

func TestAuditContractContentAcceptsCompliantContract(t *testing.T) {
	contract := map[string]any{
		"@context": []any{"https://w3id.org/facis/sla/ontology"},
		"@id":      "urn:facis:dcs:contract:sla:example-001",
		"@type":    []any{"dcs:Contract", "sla:ServiceLevelAgreement"},
		"parties": []any{
			map[string]any{"@type": "dcs:Company", "role": "supplier"},
			map[string]any{"@type": "dcs:Company", "role": "customer"},
		},
		"company": map[string]any{
			"location": map[string]any{
				"country": "DEU",
			},
		},
		"contract": map[string]any{
			"jurisdiction": "DEU",
			"governingLaw": "DE",
			"payment": map[string]any{
				"amount": 9500,
			},
		},
		"signature": map[string]any{
			"requiredLevel": "QES",
		},
	}
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.content.static",
		"version":     "test",
		"rules": []any{
			map[string]any{"id": "FACIS-CONTRACT-STATIC-001", "title": "Company country must not be blacklisted", "builtin": "value_not_in", "semanticPath": "company.location.country", "values": []any{"RUS"}, "ontologyTerm": "dcs:CountryCode"},
			map[string]any{"id": "FACIS-CONTRACT-STATIC-002", "title": "Governing law must be allowed", "builtin": "value_in", "semanticPath": "contract.governingLaw", "values": []any{"DE", "AT", "CH"}, "ontologyTerm": "dcs:Contract"},
			map[string]any{"id": "FACIS-CONTRACT-STATIC-003", "title": "Payment amount must satisfy policy maximum", "builtin": "max_number", "semanticPath": "contract.payment.amount", "max": 10000, "ontologyTerm": "dcs:PaymentTerms"},
			map[string]any{"id": "FACIS-CONTRACT-STATIC-004", "title": "Signature level must satisfy policy", "builtin": "signature_level_at_least", "semanticPath": "signature.requiredLevel", "required": "AES", "ontologyTerm": "dcs:SignatureLevelCode"},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
}

func TestAuditContractContentReadsJSONLDSemanticPathThresholds(t *testing.T) {
	contract := map[string]any{
		"company": map[string]any{
			"location": map[string]any{
				"country": "DEU",
			},
		},
		"contract": map[string]any{
			"jurisdiction": "DEU",
		},
		"semanticValues": []any{
			map[string]any{
				"semanticPath": "contract.liability.capAmount",
				"hasThreshold": map[string]any{
					"hasTargetValue": 150000,
				},
			},
		},
		"signature": map[string]any{
			"requiredLevel": "AES",
		},
	}
	policy := map[string]any{
		"policySetId": "facis.dcs.contract.content.static",
		"version":     "test",
		"rules": []any{
			map[string]any{"id": "FACIS-CONTRACT-STATIC-003", "title": "Liability cap must satisfy policy maximum", "builtin": "max_number", "semanticPath": "contract.liability.capAmount", "max": 100000, "ontologyTerm": "dcs:LiabilityTerms"},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-003", "error"))
}

func TestAuditContractContentRequiresExplicitPolicyRules(t *testing.T) {
	findings, err := AuditContractContent(map[string]any{}, nil, ContractContentAuditMetadata{})

	require.NoError(t, err)
	require.NotEmpty(t, findings)
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-JSONLD-001", "error"))
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-SHACL-CORE", "error"))
}

func TestAuditContractContentValidatesJSONLDAndSHACL(t *testing.T) {
	contract := map[string]any{
		"@context": []any{"https://w3id.org/facis/sla/ontology"},
		"@id":      "urn:facis:dcs:contract:sla:example-001",
		"@type":    []any{"dcs:Contract", "sla:ServiceLevelAgreement"},
		"parties": []any{
			map[string]any{"@type": "dcs:Company", "role": "supplier"},
			map[string]any{"@type": "dcs:Company", "role": "customer"},
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
					map[string]any{"path": "parties", "minCount": 2, "class": "dcs:Company", "name": "Contract parties"},
				},
			},
		},
	}

	findings, err := AuditContractContent(contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-JSONLD-001", "info"))
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
						map[string]any{"path": "parties", "class": "dcs:Company", "name": "Contract parties"},
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

func hasFindingSeverity(findings []PolicyFinding, ruleID string, severity string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity {
			return true
		}
	}
	return false
}
