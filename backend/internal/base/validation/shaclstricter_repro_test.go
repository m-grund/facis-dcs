package validation

import (
	"context"
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
