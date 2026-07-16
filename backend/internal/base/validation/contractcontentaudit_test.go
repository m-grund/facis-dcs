package validation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureShapeSource is a ShapeSource backed by the real Semantic Hub
// authoring files (docs/semantic-ontology/...) — the same content the hub
// seeds itself with (ADR-8) — installed process-wide in TestMain
// (documentdata_test.go) so tests exercise the real goRDFlib SHACL engine
// (ADR-9) end to end without needing a live database.
type fixtureShapeSource struct {
	shapesTTL        string
	profileYAML      string
	contextJSON      string
	externalContexts map[string]string
}

func (f fixtureShapeSource) ActiveShapes(context.Context) (string, int, error) {
	return f.shapesTTL, 1, nil
}

func (f fixtureShapeSource) ActiveProfile(context.Context) (string, int, error) {
	return f.profileYAML, 1, nil
}

func (f fixtureShapeSource) ActiveContext(context.Context) (string, int, error) {
	return f.contextJSON, 1, nil
}

func (f fixtureShapeSource) ShapesAt(_ context.Context, _ int) (string, error) {
	return f.shapesTTL, nil
}

func (f fixtureShapeSource) ContextAt(_ context.Context, _ int) (string, error) {
	return f.contextJSON, nil
}

func (f fixtureShapeSource) ContextByIRI(_ context.Context, iri string) (string, error) {
	if content, ok := f.externalContexts[iri]; ok {
		return content, nil
	}
	return "", fmt.Errorf("context %q is not registered", iri)
}

// mustReadRepoFile climbs from the package directory to find a repo-root
// relative path (go test's working directory is the package source
// directory) — hard-fails loudly rather than silently skipping if the
// authoring file has moved, matching the "never soft-fail a required
// dependency" rule.
func mustReadRepoFile(relPath string) string {
	candidates := []string{
		relPath,
		filepath.Join("..", "..", "..", relPath),
		filepath.Join("..", "..", "..", "..", relPath),
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			return string(data)
		}
	}
	panic("test fixture: could not find " + relPath + " from any candidate path")
}

// wrapODRLSet encloses rule nodes in the canonical odrl:Set shape
// (validateODRLPoliciesShape rejects bare non-empty rule arrays); an empty
// rule list yields the canonical empty "no policies yet" array.
func wrapODRLSet(rules []any) any {
	if len(rules) == 0 {
		return []any{}
	}
	return map[string]any{
		"@type":        "odrl:Set",
		"uid":          "did:example:contract",
		"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
		"odrl:duty":    rules,
	}
}

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
		"dcs:policies":            wrapODRLSet(policies),
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

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-CONTRACT-STATIC-001")
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-001", "error"))
}

func TestAuditContractContentAcceptsCompliantContract(t *testing.T) {
	countryFieldID := "urn:dcs:field:company-country"
	lawFieldID := "urn:dcs:field:contract-law"
	paymentFieldID := "urn:dcs:field:payment-amount"

	contract := map[string]any{
		"@context": map[string]any{"sla": "https://w3id.org/facis/sla/ontology#"},
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
		"dcs:policies": wrapODRLSet([]any{
			odrlDuty("FACIS-CONTRACT-STATIC-001", countryFieldID, "odrl:isNoneOf", []any{"RUS"}),
			odrlDuty("FACIS-CONTRACT-STATIC-002", lawFieldID, "odrl:isAnyOf", []any{"DE", "AT", "CH"}),
			odrlDuty("FACIS-CONTRACT-STATIC-003", paymentFieldID, "odrl:lteq", float64(10000)),
		}),
		"semanticConditionValues": []any{
			map[string]any{"conditionId": "company", "parameterName": "country", "parameterValue": "DEU"},
			map[string]any{"conditionId": "contract", "parameterName": "governingLaw", "parameterValue": "DE"},
			map[string]any{"conditionId": "contract", "parameterName": "amount", "parameterValue": float64(9500)},
		},
	}

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
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

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-003", "error"))
}

func TestAuditContractContentFlagsInvalidJurisdiction(t *testing.T) {
	fieldID := "urn:dcs:field:jurisdiction"
	contract := odrlContract(fieldID, "contract", "jurisdiction",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-COUNTRY", fieldID, "odrl:isAnyOf", []any{"DEU", "AUT", "CHE"})},
		"ZZZ",
	)

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-STATIC-COUNTRY", "error"))
}

func TestAuditContractContentAcceptsValidJurisdiction(t *testing.T) {
	fieldID := "urn:dcs:field:jurisdiction"
	contract := odrlContract(fieldID, "contract", "jurisdiction",
		[]any{odrlDuty("FACIS-CONTRACT-STATIC-COUNTRY", fieldID, "odrl:isAnyOf", []any{"DEU", "AUT", "CHE"})},
		"DEU",
	)

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
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

	findings, err := AuditContractContent(context.Background(), contract, nil, ContractContentAuditMetadata{})
	policy, policyErr := normalizeContractContentPolicy(context.Background(), nil, ContractContentAuditMetadata{})

	require.NoError(t, err)
	require.NoError(t, policyErr)
	require.True(t, policy.EnforceCanonicalShapes)
	require.True(t, policy.EnforceValidationProfile)
	require.NotEmpty(t, findings)
	// The default policy document enables both the Semantic Hub's canonical
	// SHACL shapes (goRDFlib, ADR-9) and the SLA validation profile — a
	// fully compliant canonical contract produces zero SHACL violations
	// (SHACL only reports non-conformance) and an "info" profile finding.
	for _, finding := range findings {
		if finding.ShapesVersion > 0 {
			require.NotEqual(t, "error", finding.Severity, finding.Message)
		}
	}
	require.True(t, hasFindingSeverity(findings, "FACIS-CONTRACT-POLICY-003", "info"))
}

func TestAuditContractContentSHACLReportsRealSHACLCoreViolations(t *testing.T) {
	// dcs:metadata is missing dcs:title entirely — a real SHACL sh:minCount
	// violation the deleted hand-rolled subset matcher's replacement
	// (goRDFlib, ADR-9) must catch via the hub's canonical shapes.
	contract := canonicalAuditContract()
	contract["dcs:metadata"] = map[string]any{
		"@type":       "dcs:ContractMetadata",
		"dcs:version": 1,
	}

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)

	shaclFindings := shaclOnlyFindings(findings)
	require.NotEmpty(t, shaclFindings)
	titleFinding := requirePolicyFinding(t, shaclFindings, "title-MinCountConstraintComponent")
	require.Equal(t, "error", titleFinding.Severity)
	require.Contains(t, titleFinding.Message, "requires a title")
}

func TestAuditContractContentSHACLAcceptsCompliantCanonicalContract(t *testing.T) {
	contract := canonicalAuditContract()

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.Empty(t, shaclOnlyFindings(findings))
}

// TestAuditContractContentValidatesTypedClauses is the Phase 3 (ADR-10)
// acceptance criterion: a dcs:PaymentClause instance is validated by the
// SAME shapes graph as the rest of a contract (semantichub.HubShapeSource
// concatenates the canonical shapes with the clause catalog at runtime;
// this test mirrors that by concatenating the two authoring files) — one
// source of truth between the template builder's palette
// (GET /semantic/clauses) and server-side enforcement.
func TestAuditContractContentValidatesTypedClauses(t *testing.T) {
	canonicalTTL := mustReadRepoFile("docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl")
	clauseCatalogTTL := mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl")
	restore := swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   canonicalTTL + "\n\n" + clauseCatalogTTL,
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld"),
	})
	defer restore()

	invalidClause := map[string]any{
		"@context":     map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "xsd": "http://www.w3.org/2001/XMLSchema#"},
		"@id":          "urn:facis:dcs:clause:payment-001",
		"@type":        "dcs:PaymentClause",
		"dcs:amount":   map[string]any{"@value": -5, "@type": "xsd:integer"},
		"dcs:currency": "EUR",
	}
	findings, err := AuditContractContent(context.Background(), invalidClause, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	finding := requirePolicyFinding(t, findings, "amount-MinInclusiveConstraintComponent")
	require.Equal(t, "error", finding.Severity)

	validClause := map[string]any{
		"@context":     map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "xsd": "http://www.w3.org/2001/XMLSchema#"},
		"@id":          "urn:facis:dcs:clause:payment-002",
		"@type":        "dcs:PaymentClause",
		"dcs:amount":   map[string]any{"@value": 100, "@type": "xsd:integer"},
		"dcs:currency": "EUR",
	}
	okFindings, err := AuditContractContent(context.Background(), validClause, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, okFindings)
}

func mapPolicy(enforceShapes, enforceProfile bool) map[string]any {
	return map[string]any{
		"policySetId":              "facis.dcs.contract.structure-semantics",
		"version":                  "test",
		"enforceCanonicalShapes":   enforceShapes,
		"enforceValidationProfile": enforceProfile,
	}
}

func shaclOnlyFindings(findings []PolicyFinding) []PolicyFinding {
	out := make([]PolicyFinding, 0, len(findings))
	for _, f := range findings {
		if f.ShapesVersion > 0 {
			out = append(out, f)
		}
	}
	return out
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

	findings, err := AuditContractContent(context.Background(), contract, policy, ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-EXT-001", "error"))
}

func TestAuditContractContentAcceptsCanonicalContractODRLValues(t *testing.T) {
	contract := canonicalAuditContract()

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
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

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "error"))
}

func TestAuditContractContentFlagsCanonicalContractMissingSemanticValue(t *testing.T) {
	contract := canonicalAuditContract()
	contract["semanticConditionValues"] = []any{}

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
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
	policy := contract["dcs:policies"].(map[string]any)["odrl:duty"].([]any)[0].(map[string]any)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:missing-field"}

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
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

func TestAuditContractContentReadsCanonicalRuntimeValuesBySemanticPath(t *testing.T) {
	contract := canonicalAuditContractWithTemplateParties()
	findings := auditContractValidationProfile(contract, map[string]any{}, ValidationProfile{
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

	findings := auditContractValidationProfile(contract, map[string]any{}, ValidationProfile{
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

	findings := auditContractValidationProfile(contract, map[string]any{}, ValidationProfile{
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

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	finding := requirePolicyFinding(t, findings, "urn:uuid:policy-postal-code")
	require.Equal(t, "eq", finding.Operator)
	require.Equal(t, "91448", finding.ActualValue)
	require.Equal(t, "91448", finding.ExpectedValue)
	require.Equal(t, "urn:uuid:field-company-postal-code must equal 91448", finding.Requirement)
}

// TestAuditContractContentSHACLRejectsWrongDatatype is the Phase 2 (ADR-9)
// acceptance criterion: a genuine xsd:integer datatype constraint the
// deleted subset matcher's replacement (goRDFlib) enforces, with a finding
// naming the focus node and the violated constraint. Uses its own
// ShapeSource (not the package fixture) to exercise a shapes graph the
// hub-seeded canonical shape doesn't declare — standing in for "register a
// stricter hub shapes version" without a live database.
func TestAuditContractContentSHACLRejectsWrongDatatype(t *testing.T) {
	const slaShapesTTL = `
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix sh:  <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

dcs:SLAAgreementShape
  a sh:NodeShape ;
  sh:targetClass dcs:SLAAgreement ;
  sh:property [
    sh:path dcs:availability ;
    sh:datatype xsd:integer ;
    sh:minInclusive 0 ;
  ] .
`
	restore := swapShapeSource(t, fixtureShapeSource{shapesTTL: slaShapesTTL, profileYAML: "id: t\nversion: t\nrules: []\n", contextJSON: mustReadRepoFile("docs/semantic-ontology/contexts/facis-dcs-context.jsonld")})
	defer restore()

	badContract := map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":      "urn:facis:dcs:sla:example-001",
		"@type":    "dcs:SLAAgreement",
		"dcs:availability": map[string]any{
			"@value": "ninety-nine",
		},
	}
	findings, err := AuditContractContent(context.Background(), badContract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	finding := requirePolicyFinding(t, findings, "availability-DatatypeConstraintComponent")
	require.Equal(t, "error", finding.Severity)
	require.Contains(t, finding.Path, "availability")
	require.Contains(t, finding.Message, "urn:facis:dcs:sla:example-001") // names the focus node

	goodContract := map[string]any{
		"@context":         map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "xsd": "http://www.w3.org/2001/XMLSchema#"},
		"@id":              "urn:facis:dcs:sla:example-002",
		"@type":            "dcs:SLAAgreement",
		"dcs:availability": map[string]any{"@value": 99, "@type": "xsd:integer"},
	}
	okFindings, err := AuditContractContent(context.Background(), goodContract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, okFindings)
}

// swapShapeSource installs a temporary ShapeSource for the duration of a
// single test and returns a restore func for the shared fixture — the
// package var is process-wide, so tests using a different shapes graph must
// not run in Parallel with each other.
func swapShapeSource(t *testing.T, s ShapeSource) func() {
	t.Helper()
	original := activeShapeSource
	SetShapeSource(s)
	return func() { activeShapeSource = original }
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
		"dcs:policies": wrapODRLSet([]any{
			odrlDuty("urn:uuid:policy-country", countryFieldID, "odrl:isAnyOf", []any{
				map[string]any{"@type": "xsd:string", "@value": "DEU"},
				map[string]any{"@type": "xsd:string", "@value": "AUT"},
				map[string]any{"@type": "xsd:string", "@value": "CHE"},
			}),
			odrlDuty("urn:uuid:policy-postal-code", postalCodeFieldID, "odrl:eq", map[string]any{"@type": "xsd:string", "@value": "91448"}),
		}),
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
