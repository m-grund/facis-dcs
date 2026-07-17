package validation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	ontologyTTL      string
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

// ActiveDomainOntology defaults to the hub's seed SLA ontology asset, so
// fixtures that only override shapes keep a working domain-field index.
func (f fixtureShapeSource) ActiveDomainOntology(context.Context) (string, int, error) {
	if f.ontologyTTL != "" {
		return f.ontologyTTL, 1, nil
	}
	return mustReadRepoFile("backend/internal/semantichub/assets/facis-sla-ontology.ttl"), 1, nil
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
		"@type":           "odrl:Agreement",
		"odrl:profile":    map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
		"odrl:obligation": rules,
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
						"@id":                fieldID,
						"@type":              "dcs:RequirementField",
						"dcs:parameterName":  parameterName,
						"dcs:parameterValue": actualValue,
					},
				},
			},
		},
		"dcs:policies": wrapODRLSet(policies),
	}
}

// setInlineFieldValue sets a submitted value inline on the requirement field
// with the given @id, wherever it is declared (including composed
// sub-templates) — the shape the audit reads now that values live on the
// field rather than a separate semanticConditionValues array.
func setInlineFieldValue(node any, fieldID string, value any) bool {
	switch n := node.(type) {
	case map[string]any:
		if id, _ := n["@id"].(string); id == fieldID {
			if _, isField := n["dcs:parameterName"]; isField {
				n["dcs:parameterValue"] = value
				return true
			}
		}
		for _, child := range n {
			if setInlineFieldValue(child, fieldID, value) {
				return true
			}
		}
	case []any:
		for _, child := range n {
			if setInlineFieldValue(child, fieldID, value) {
				return true
			}
		}
	}
	return false
}

// applyInlineFieldValues sets each {forField, parameterValue} entry inline on
// the field it references.
func applyInlineFieldValues(contract map[string]any, entries []any) {
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fieldID, _ := entry["forField"].(string)
		setInlineFieldValue(contract, fieldID, entry["parameterValue"])
	}
}

// deleteInlineFieldValue removes the inline value from the field with the
// given @id.
func deleteInlineFieldValue(node any, fieldID string) bool {
	switch n := node.(type) {
	case map[string]any:
		if id, _ := n["@id"].(string); id == fieldID {
			if _, isField := n["dcs:parameterName"]; isField {
				delete(n, "dcs:parameterValue")
				return true
			}
		}
		for _, child := range n {
			if deleteInlineFieldValue(child, fieldID) {
				return true
			}
		}
	case []any:
		for _, child := range n {
			if deleteInlineFieldValue(child, fieldID) {
				return true
			}
		}
	}
	return false
}

// clearInlineFieldValues removes every inline submitted value in the document.
func clearInlineFieldValues(node any) {
	switch n := node.(type) {
	case map[string]any:
		delete(n, "dcs:parameterValue")
		for _, child := range n {
			clearInlineFieldValues(child)
		}
	case []any:
		for _, child := range n {
			clearInlineFieldValues(child)
		}
	}
}

func odrlDuty(id, fieldID, operator string, rightOperand any) map[string]any {
	return map[string]any{
		"@id":         id,
		"@type":       "odrl:Duty",
		"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
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
	}
	applyInlineFieldValues(contract, []any{
		map[string]any{"forField": countryFieldID, "parameterValue": "DEU"},
		map[string]any{"forField": lawFieldID, "parameterValue": "DE"},
		map[string]any{"forField": paymentFieldID, "parameterValue": float64(9500)},
	})

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
	contract["dcs:contractData"] = append(contract["dcs:contractData"].([]any),
		slaRequirement("condition-legal", "contract.jurisdiction"),
		slaRequirement("condition-service", "service.sla.availability", "service.sla.responseTime", "service.sla.resolutionTime"),
		slaRequirement("condition-signature", "signature.requiredLevel"),
	)
	applyInlineFieldValues(contract, []any{
		map[string]any{"forField": slaFieldID("condition-legal", "contract.jurisdiction"), "parameterValue": "DEU"},
		map[string]any{"forField": slaFieldID("condition-service", "service.sla.availability"), "parameterValue": 99.95},
		map[string]any{"forField": slaFieldID("condition-service", "service.sla.responseTime"), "parameterValue": 10},
		map[string]any{"forField": slaFieldID("condition-service", "service.sla.resolutionTime"), "parameterValue": 120},
		map[string]any{"forField": slaFieldID("condition-signature", "signature.requiredLevel"), "parameterValue": "AES"},
	})

	findings, err := AuditContractContent(context.Background(), contract, nil, ContractContentAuditMetadata{})
	policy, policyErr := normalizeContractContentPolicy(context.Background(), nil, ContractContentAuditMetadata{})

	require.NoError(t, err)
	require.NoError(t, policyErr)
	require.True(t, policy.EnforceCanonicalShapes)
	require.True(t, policy.EnforceValidationProfile)
	require.NotEmpty(t, findings)
	// The default policy document enables both the Semantic Hub's canonical
	// SHACL shapes and the SLA validation profile — a fully compliant
	// canonical contract produces zero SHACL violations.
	for _, finding := range findings {
		if finding.ShapesVersion > 0 {
			require.NotEqual(t, "error", finding.Severity, finding.Message)
		}
	}
	require.Positive(t, policy.ProfileVersion)
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
	canonicalTTL := mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl")
	clauseCatalogTTL := mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl")
	restore := swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   canonicalTTL + "\n\n" + clauseCatalogTTL,
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
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
	setInlineFieldValue(contract, "urn:uuid:field-company-country", "USA")

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "urn:uuid:policy-country", "error"))
}

func TestAuditContractContentFlagsCanonicalContractMissingSemanticValue(t *testing.T) {
	contract := canonicalAuditContract()
	clearInlineFieldValues(contract)

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
	policy := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)[0].(map[string]any)
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
	setInlineFieldValue(contract, "urn:uuid:field-company-country", "USA")

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
	clearInlineFieldValues(contract)

	err := ValidateContractPolicySatisfaction(contract, ContractContentAuditMetadata{})

	var policyErr ContractPolicySatisfactionError
	require.ErrorAs(t, err, &policyErr)
	require.Len(t, policyErr.Findings, 2)
	require.Equal(t, "urn:uuid:policy-country", policyErr.Findings[0].RuleID)
	require.Equal(t, "urn:uuid:policy-postal-code", policyErr.Findings[1].RuleID)
}

func TestAuditContractContentReadsCanonicalRuntimeValuesByParameterName(t *testing.T) {
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
	deleteInlineFieldValue(contract, slaFieldID("condition-service", "service.sla.availability"))
	deleteInlineFieldValue(contract, slaFieldID("condition-legal", "contract.jurisdiction"))

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
	restore := swapShapeSource(t, fixtureShapeSource{shapesTTL: slaShapesTTL, profileYAML: "id: t\nversion: t\nrules: []\n", contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld")})
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
	return func() {
		activeShapeSource = original
		ResetDomainOntologyCache()
	}
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
						"@id":                countryFieldID,
						"@type":              "dcs:RequirementField",
						"dcs:parameterName":  "country",
						"dcs:domainField":    map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
						"dcs:required":       true,
						"dcs:blockId":        "urn:uuid:block-clause-1",
						"dcs:parameterValue": "DEU",
					},
					map[string]any{
						"@id":                postalCodeFieldID,
						"@type":              "dcs:RequirementField",
						"dcs:parameterName":  "postalCode",
						"dcs:domainField":    map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-postalCode"},
						"dcs:required":       true,
						"dcs:blockId":        "urn:uuid:block-clause-1",
						"dcs:parameterValue": "91448",
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
	}
}

// slaRequirement declares a data requirement whose fields carry dotted
// semantic-path parameter names, matching how SLA statement rules address
// runtime values.
func slaRequirement(conditionID string, parameterNames ...string) map[string]any {
	fields := make([]any, 0, len(parameterNames))
	for _, parameterName := range parameterNames {
		fields = append(fields, map[string]any{
			"@id":               slaFieldID(conditionID, parameterName),
			"@type":             "dcs:RequirementField",
			"dcs:parameterName": parameterName,
			"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#" + conditionID},
			"dcs:required":      true,
		})
	}
	return map[string]any{
		"@id":               "urn:uuid:req-" + conditionID,
		"@type":             "dcs:DataRequirement",
		"dcs:conditionId":   conditionID,
		"dcs:name":          conditionID,
		"dcs:schemaVersion": "v1",
		"dcs:fields":        fields,
	}
}

func slaFieldID(conditionID, parameterName string) string {
	return "urn:uuid:field-" + conditionID + "-" + strings.ReplaceAll(parameterName, ".", "-")
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
	contract["dcs:contractData"] = []any{
		slaRequirement("condition-service", "service.sla.availability"),
		slaRequirement("condition-legal", "contract.jurisdiction"),
	}
	applyInlineFieldValues(contract, []any{
		map[string]any{"forField": "urn:uuid:field-condition-customer-legal-name", "parameterValue": "Firma A"},
		map[string]any{"forField": "urn:uuid:field-condition-customer-country", "parameterValue": "DEU"},
		map[string]any{"forField": "urn:uuid:field-condition-provider-legal-name", "parameterValue": "Firma B"},
		map[string]any{"forField": "urn:uuid:field-condition-provider-country", "parameterValue": "DEU"},
		map[string]any{"forField": slaFieldID("condition-service", "service.sla.availability"), "parameterValue": 99.5},
		map[string]any{"forField": slaFieldID("condition-legal", "contract.jurisdiction"), "parameterValue": "DEU"},
	})
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

// TestAuditContractEvaluatesLogicalConstraint proves the enforcement engine
// evaluates ODRL logical constraints (LogicalConstraint, IM §2.6) recursively:
// an odrl:or is satisfied when any branch holds and violated only when none do.
func TestAuditContractEvaluatesLogicalConstraint(t *testing.T) {
	fieldID := "urn:dcs:field:amount"
	orConstraint := map[string]any{
		"@type": "odrl:LogicalConstraint",
		"odrl:or": []any{
			map[string]any{
				"@type":             "odrl:Constraint",
				"odrl:leftOperand":  map[string]any{"@id": fieldID},
				"odrl:operator":     map[string]any{"@id": "odrl:lteq"},
				"odrl:rightOperand": float64(500),
			},
			map[string]any{
				"@type":             "odrl:Constraint",
				"odrl:leftOperand":  map[string]any{"@id": fieldID},
				"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
				"odrl:rightOperand": float64(1000),
			},
		},
	}
	duty := func() map[string]any {
		return map[string]any{
			"@id":             "FACIS-LOGICAL-OR",
			"@type":           "odrl:Duty",
			"dcs:prose":       map[string]any{"@id": "urn:uuid:block-clause-1"},
			"odrl:action":     map[string]any{"@id": "dcs:provideCompliantValue"},
			"odrl:constraint": []any{orConstraint},
		}
	}

	// 400 satisfies the first branch → the or holds → no violation.
	ok := odrlContract(fieldID, "payment", "amount", []any{duty()}, float64(400))
	findings, err := AuditContractContent(context.Background(), ok, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}

	// 700 satisfies neither branch → the or is violated.
	bad := odrlContract(fieldID, "payment", "amount", []any{duty()}, float64(700))
	violated, err := AuditContractContent(context.Background(), bad, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.True(t, hasFindingSeverity(violated, "FACIS-LOGICAL-OR", "error"))
}

// TestAuditContractEvaluatesNestedDuty proves the enforcement engine audits a
// Permission's nested duties (ODRL IM §2.5): the duty is recorded as a use-time
// obligation, and its own constraints are evaluated as obligations — satisfied
// when the value holds, flagged when it does not.
func TestAuditContractEvaluatesNestedDuty(t *testing.T) {
	fieldID := "urn:dcs:field:amount"
	permission := func() map[string]any {
		return map[string]any{
			"@id":         "FACIS-PERMISSION-WITH-DUTY",
			"@type":       "odrl:Permission",
			"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
			"odrl:action": map[string]any{"@id": "odrl:use"},
			"odrl:duty": []any{
				map[string]any{
					"@id":         "FACIS-DUTY-COMPENSATE",
					"@type":       "odrl:Duty",
					"odrl:action": map[string]any{"@id": "odrl:compensate"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": fieldID},
							"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
							"odrl:rightOperand": float64(1000),
						},
					},
				},
			},
		}
	}

	// 1500 ≥ 1000 → the duty obligation is met → no violation, and the
	// permission records its duty as a use-time obligation.
	ok := odrlContract(fieldID, "payment", "amount", []any{permission()}, float64(1500))
	findings, err := AuditContractContent(context.Background(), ok, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
	require.True(t, hasFindingSeverity(findings, "FACIS-PERMISSION-WITH-DUTY", "info"), "duty recorded as use-time obligation")

	// 500 < 1000 → the duty obligation is unmet → the duty is flagged.
	bad := odrlContract(fieldID, "payment", "amount", []any{permission()}, float64(500))
	violated, err := AuditContractContent(context.Background(), bad, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.True(t, hasFindingSeverity(violated, "FACIS-DUTY-COMPENSATE", "error"), "unmet duty obligation flagged")
}

// TestAuditContractEnforcesIsPartOf proves the isPartOf operator — offered by
// the clause builder — is actually enforced by the contract policy check: a
// value in the enumerated set passes, one outside it is flagged.
func TestAuditContractEnforcesIsPartOf(t *testing.T) {
	fieldID := "urn:dcs:field:country"
	duty := func() map[string]any {
		return map[string]any{
			"@id":         "FACIS-ISPARTOF",
			"@type":       "odrl:Duty",
			"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
			"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
			"odrl:constraint": []any{
				map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  map[string]any{"@id": fieldID},
					"odrl:operator":     map[string]any{"@id": "odrl:isPartOf"},
					"odrl:rightOperand": []any{"DEU", "AUT"},
				},
			},
		}
	}

	ok := odrlContract(fieldID, "region", "country", []any{duty()}, "DEU")
	findings, err := AuditContractContent(context.Background(), ok, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}

	bad := odrlContract(fieldID, "region", "country", []any{duty()}, "ZZZ")
	violated, err := AuditContractContent(context.Background(), bad, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.True(t, hasFindingSeverity(violated, "FACIS-ISPARTOF", "error"), "value outside the isPartOf set flagged")
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

// TestAuditContractAcceptsTempoSpatialAccessPolicy proves the SRS Appendix C
// policy audits cleanly once instantiated from the access-grant template: a
// Permission to use bounded by an ANDed spatial and dateTime context
// constraint whose boundaries are the negotiated region and deadline fields.
// The context operands are accepted (never flagged "nonexistent field") and
// deferred to use-time, and each negotiated boundary resolves to its filled
// contract value.
func TestAuditContractAcceptsTempoSpatialAccessPolicy(t *testing.T) {
	countryFieldID := "urn:dcs:field:permitted-country"
	deadlineFieldID := "urn:dcs:field:access-deadline"

	permission := map[string]any{
		"@id":         "FACIS-CONTRACT-APPENDIX-C",
		"@type":       "odrl:Permission",
		"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:action": map[string]any{"@id": "odrl:use"},
		"odrl:constraint": []any{
			map[string]any{
				"@type":             "odrl:Constraint",
				"odrl:leftOperand":  map[string]any{"@id": "odrl:spatial"},
				"odrl:operator":     map[string]any{"@id": "odrl:eq"},
				"odrl:rightOperand": map[string]any{"@id": countryFieldID},
			},
			map[string]any{
				"@type":             "odrl:Constraint",
				"odrl:leftOperand":  map[string]any{"@id": "odrl:dateTime"},
				"odrl:operator":     map[string]any{"@id": "odrl:lteq"},
				"odrl:rightOperand": map[string]any{"@id": deadlineFieldID},
			},
		},
	}

	contract := map[string]any{
		"@id":   "urn:facis:dcs:contract:appendix-c",
		"@type": "dcs:Contract",
		"dcs:contractData": []any{
			map[string]any{
				"@id": "urn:dcs:req:access", "@type": "dcs:DataRequirement", "dcs:conditionId": "access",
				"dcs:fields": []any{
					map[string]any{"@id": countryFieldID, "@type": "dcs:RequirementField", "dcs:parameterName": "permittedCountry", "dcs:parameterValue": "DE"},
					map[string]any{"@id": deadlineFieldID, "@type": "dcs:RequirementField", "dcs:parameterName": "accessDeadline", "dcs:parameterValue": "2025-05-10T23:59:59"},
				},
			},
		},
		"dcs:policies": map[string]any{
			"@type":           "odrl:Agreement",
			"odrl:profile":    map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:permission": []any{permission},
		},
	}

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}

	var spatial, temporal *PolicyFinding
	for i := range findings {
		switch findings[i].Path {
		case odrlIRI + "spatial":
			spatial = &findings[i]
		case odrlIRI + "dateTime":
			temporal = &findings[i]
		}
	}
	require.NotNil(t, spatial, "spatial context constraint audited")
	require.NotNil(t, temporal, "dateTime context constraint audited")
	require.Contains(t, fmt.Sprint(spatial.ExpectedValue), "DE", "negotiated region boundary resolved to the filled value")
	require.Equal(t, "lte", temporal.Operator)
}
