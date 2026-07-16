package validation

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuditContractContentFlagsShInNarrowedTitle mirrors the BDD scenario
// "Activating a stricter SHACL shapes version changes what NEW contracts get
// flagged for" (features/23_semantic_hub/semantic_hub.feature): the stricter
// v2 shapes are the genesis canonical shapes with an sh:in restriction
// injected on dcs:ContractMetadataShape's dcs:title property — a compliant
// canonical contract must then produce a title-InConstraintComponent error.
func TestAuditContractContentFlagsShInNarrowedTitle(t *testing.T) {
	canonicalTTL := mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl")
	anchor := "sh:path dcs:title ;"
	require.Contains(t, canonicalTTL, anchor)
	stricterTTL := strings.Replace(
		canonicalTTL,
		anchor,
		anchor+"\n    sh:in ( \"IMPOSSIBLE-BDD-TITLE-VALUE-NO-CONTRACT-HAS-THIS\" ) ;",
		1,
	)
	require.NotEqual(t, canonicalTTL, stricterTTL)

	restore := swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   stricterTTL,
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
	})
	defer restore()

	contract := canonicalAuditContract()
	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)

	finding := requirePolicyFinding(t, findings, "title-InConstraintComponent")
	require.Equal(t, "error", finding.Severity)
}

func TestAuditContractContentResolvesRegisteredExternalContext(t *testing.T) {
	external := "https://example.org/bdd/external-context"
	restore := swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
		externalContexts: map[string]string{
			external: `{"@context": {"ex": "https://example.org/ns#"}}`,
		},
	})
	defer restore()

	contract := canonicalAuditContract()
	contract["@context"] = []any{contract["@context"], external}
	contract["ex:externalNote"] = "annotated via an externally anchored context"

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, shaclOnlyFindings(findings))
}

func TestAuditContractContentRejectsUnregisteredExternalContext(t *testing.T) {
	contract := canonicalAuditContract()
	contract["@context"] = []any{contract["@context"], "https://example.org/never-registered"}

	_, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.ErrorContains(t, err, "not registered in the Semantic Hub")
}

func TestNormalizeTemplateDataRejectsUnregisteredExternalContext(t *testing.T) {
	doc := map[string]any{
		"@context": []any{
			map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
			"https://example.org/never-registered",
		},
		"@type":        "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{"@type": "dcs:TemplateMetadata", "dcs:title": "External ctx"},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{}},
			"dcs:layout": []any{map[string]any{"@id": "urn:uuid:block-root", "dcs:isRoot": true, "dcs:children": map[string]any{"@list": []any{}}}},
		},
	}
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "not registered in the Semantic Hub")
}

func TestAuditContractContentResolvesW3IDContextLocally(t *testing.T) {
	contract := canonicalAuditContract()
	contract["@context"] = []any{SchemaJSONLDContextV1, contract["@context"]}

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, shaclOnlyFindings(findings))
}

// gaiaXCatalogShapes mirrors a Gaia-X Trust Framework participant shape
// registered into the hub's clause catalog: foreign namespace, nested
// sh:node group, sh:in value set.
const gaiaXCatalogShapes = `@prefix gx: <https://w3id.org/gaia-x/development#> .
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

gx:LegalParticipantShape
  a sh:NodeShape ;
  sh:targetClass gx:LegalParticipant ;
  sh:property [
    sh:path gx:legalName ;
    sh:datatype xsd:string ;
    sh:minCount 1 ;
  ] ;
  sh:property [
    sh:path gx:headquarterAddress ;
    sh:minCount 1 ;
    sh:node gx:AddressShape ;
  ] .

gx:AddressShape
  a sh:NodeShape ;
  sh:property [
    sh:path gx:countryCode ;
    sh:datatype xsd:string ;
    sh:in ( "DE" "FR" "NL" ) ;
    sh:minCount 1 ;
  ] .
`

func gaiaXParticipantInstance() map[string]any {
	return map[string]any{
		"@id":   "urn:uuid:aa11bb22-0000-0000-0000-000000000001",
		"@type": "https://w3id.org/gaia-x/development#LegalParticipant",
		"https://w3id.org/gaia-x/development#legalName": "Musterfirma GmbH",
		"https://w3id.org/gaia-x/development#headquarterAddress": map[string]any{
			"https://w3id.org/gaia-x/development#countryCode": "DE",
		},
	}
}

func swapGaiaXShapeSource(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL: mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl") +
			"\n\n" + gaiaXCatalogShapes,
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
	})
}

func TestAuditContractContentValidatesGaiaXParticipantClause(t *testing.T) {
	restore := swapGaiaXShapeSource(t)
	defer restore()

	contract := canonicalAuditContract()
	contract["dcs:typedClause"] = gaiaXParticipantInstance()

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, shaclOnlyFindings(findings))
}

func TestAuditContractContentFlagsGaiaXParticipantViolations(t *testing.T) {
	restore := swapGaiaXShapeSource(t)
	defer restore()

	participant := gaiaXParticipantInstance()
	delete(participant, "https://w3id.org/gaia-x/development#legalName")
	participant["https://w3id.org/gaia-x/development#headquarterAddress"] = map[string]any{
		"https://w3id.org/gaia-x/development#countryCode": "XX",
	}
	contract := canonicalAuditContract()
	contract["dcs:typedClause"] = participant

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)

	missingName := requirePolicyFinding(t, findings, "legalName-MinCountConstraintComponent")
	require.Equal(t, "error", missingName.Severity)
	// sh:node violations surface as a NodeConstraintComponent result on the
	// outer property (SHACL spec behavior).
	wrongCountry := requirePolicyFinding(t, findings, "headquarterAddress-NodeConstraintComponent")
	require.Equal(t, "error", wrongCountry.Severity)
}

// slaAuditContract is a canonical contract carrying real SLA content —
// typed CompanyParty, PaymentTerm, SLAAgreement, and SLO nodes — the
// facis.sla.basic profile's statement rules evaluate against.
func slaAuditContract() map[string]any {
	contract := canonicalAuditContract()
	contract["dcs:hasSLA"] = map[string]any{
		"@id":   "urn:uuid:sla-1",
		"@type": "dcs:SLAAgreement",
		"dcs:hasService": map[string]any{
			"@id":   "urn:uuid:service-1",
			"@type": "dcs:Service",
			"dcs:hasSLO": map[string]any{
				"@id":              "urn:uuid:slo-availability",
				"@type":            "dcs:SLO",
				"dcs:availability": 99.9,
			},
		},
	}
	contract["dcs:contractParties"] = []any{
		map[string]any{
			"@id":           "urn:uuid:party-provider",
			"@type":         "dcs:CompanyParty",
			"dcs:role":      map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"},
			"dcs:legalName": "Provider GmbH",
		},
		map[string]any{
			"@id":           "urn:uuid:party-customer",
			"@type":         "dcs:CompanyParty",
			"dcs:role":      map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#role-customer"},
			"dcs:legalName": "Customer GmbH",
		},
	}
	contract["dcs:paymentTerm"] = map[string]any{
		"@id":          "urn:uuid:payment-1",
		"@type":        "dcs:PaymentTerm",
		"dcs:amount":   1000.0,
		"dcs:currency": "EUR",
		"dcs:dueDate":  "2026-12-01",
	}
	return contract
}

var slaStatementRuleIDs = []string{
	"exactly-one-provider", "exactly-one-customer",
	"availability-slo-required", "payment-required", "payment-amount-positive",
}

func TestAuditContractContentEvaluatesSLAProfileStatements(t *testing.T) {
	contract := slaAuditContract()
	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(false, true), ContractContentAuditMetadata{})
	require.NoError(t, err)
	for _, finding := range findings {
		if finding.Severity == "error" {
			require.NotContains(t, slaStatementRuleIDs, finding.RuleID, finding.Message)
		}
	}

	broken := slaAuditContract()
	broken["dcs:paymentTerm"].(map[string]any)["dcs:amount"] = 0.0
	broken["dcs:contractParties"] = append(broken["dcs:contractParties"].([]any), map[string]any{
		"@id":           "urn:uuid:party-provider-2",
		"@type":         "dcs:CompanyParty",
		"dcs:role":      map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"},
		"dcs:legalName": "Second Provider GmbH",
	})
	findings, err = AuditContractContent(context.Background(), broken, mapPolicy(false, true), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Contains(t, policyFindingRuleIDs(findings), "payment-amount-positive")
	require.Contains(t, policyFindingRuleIDs(findings), "exactly-one-provider")

	// A contract without SLA content stays out of the SLA profile's scope.
	plain := canonicalAuditContract()
	findings, err = AuditContractContent(context.Background(), plain, mapPolicy(false, true), ContractContentAuditMetadata{})
	require.NoError(t, err)
	for _, finding := range findings {
		require.NotContains(t, slaStatementRuleIDs, finding.RuleID, finding.Message)
	}
}

func TestEvaluateKPIViolationBindsMetricByParameterName(t *testing.T) {
	contract := canonicalAuditContract()

	violated, err := EvaluateKPIViolation(context.Background(), contract, "country", "USA")
	require.NoError(t, err)
	require.True(t, violated, "USA is outside the isAnyOf set the country field's Duty declares")

	violated, err = EvaluateKPIViolation(context.Background(), contract, "country", "DEU")
	require.NoError(t, err)
	require.False(t, violated)

	violated, err = EvaluateKPIViolation(context.Background(), contract, "unbound-metric", "1")
	require.NoError(t, err)
	require.False(t, violated, "a metric no RequirementField declares binds to nothing")
}

func TestODRLRulesRequireProseBacking(t *testing.T) {
	contract := canonicalAuditContract()
	rules := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)
	delete(rules[0].(map[string]any), "dcs:prose")

	// The audit's SHACL pass flags the unbacked rule.
	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	finding := requirePolicyFinding(t, findings, "prose-MinCountConstraintComponent")
	require.Equal(t, "error", finding.Severity)

	// The authoring-time structural gate rejects it outright.
	template := canonicalTemplateData(t)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(*template, &doc))
	rule := doc["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)[0].(map[string]any)
	delete(rule, "dcs:prose")
	raw, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	_, err = NormalizeTemplateData(&raw)
	require.ErrorContains(t, err, "dcs:prose")
}

func TestClauseCatalogObligationShapeEnforcesActionVocabulary(t *testing.T) {
	restore := swapShapeSource(t, fixtureShapeSource{
		shapesTTL: mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl") +
			"\n\n" + mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
	})
	defer restore()

	contract := canonicalAuditContract()
	rules := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)
	rules[0].(map[string]any)["odrl:action"] = map[string]any{"@id": "https://evil.example/never-declared-action"}

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	finding := requirePolicyFinding(t, findings, "action-InConstraintComponent")
	require.Equal(t, "error", finding.Severity)
}
