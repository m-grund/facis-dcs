package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestAuditTemplatePoliciesReturnsWarningsForMissingFinishedTemplateFields(t *testing.T) {
	findings, err := AuditTemplatePolicies(validTemplateData(t), TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "subContract",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	require.NotEmpty(t, findings)
	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-TPL-LEGAL-001")
	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-TPL-PARTY-001")
}

func TestAuditTemplatePoliciesAcceptsRequiredDomainFields(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["semanticConditions"] = []any{
		semanticCondition("jurisdiction", SchemaContractV1, "contract.jurisdiction", "string"),
		semanticCondition("country", SchemaPartyV1, "company.location.country", "string"),
		semanticCondition("signatureLevel", SchemaSignatureV1, "signature.requiredLevel", "string"),
	}
	blocks := decoded["documentBlocks"].([]any)
	clause := blocks[0].(map[string]any)
	clause["conditionIds"] = []any{"jurisdiction", "country", "signatureLevel"}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "subContract",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	ruleIDs := policyFindingRuleIDs(findings)
	require.NotContains(t, ruleIDs, "FACIS-TPL-LEGAL-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-PARTY-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-SIGN-001")
}

func semanticCondition(id string, schemaRef string, semanticPath string, paramType string) map[string]any {
	return map[string]any{
		"conditionId":   id,
		"conditionName": id,
		"schemaVersion": "v1",
		"parameters": []any{
			map[string]any{
				"parameterName": semanticPath,
				"type":          paramType,
				"schemaRef":     schemaRef,
				"semanticPath":  semanticPath,
				"isRequired":    true,
				"operators":     []any{},
			},
		},
	}
}

func policyFindingRuleIDs(findings []PolicyFinding) []string {
	ruleIDs := make([]string, len(findings))
	for i, finding := range findings {
		ruleIDs[i] = finding.RuleID
	}
	return ruleIDs
}
