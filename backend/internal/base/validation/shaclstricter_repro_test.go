package validation

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
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
