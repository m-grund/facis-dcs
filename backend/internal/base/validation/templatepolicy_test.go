package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestAuditTemplatePoliciesReturnsWarningsForMissingFinishedTemplateFields(t *testing.T) {
	findings, err := AuditTemplatePolicies(canonicalTemplateData(t), TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "subContract",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	require.NotEmpty(t, findings)
	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-TPL-LEGAL-001")
	require.NotContains(t, policyFindingRuleIDs(findings), "FACIS-TPL-PARTY-001")
}

func TestAuditTemplatePoliciesAcceptsRequiredDomainFields(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	requirement := decoded["dcs:contractData"].([]any)[0].(map[string]any)
	fields := requirement["dcs:fields"].([]any)
	requirement["dcs:fields"] = append(fields,
		canonicalRequirementField("jurisdiction", "contract.jurisdiction"),
		canonicalRequirementField("signature-level", "signature.requiredLevel"),
	)
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

func canonicalRequirementField(id string, semanticPath string) map[string]any {
	return map[string]any{
		"@id":               "urn:uuid:field-" + id,
		"@type":             "dcs:RequirementField",
		"dcs:parameterName": id,
		"dcs:domainField":   map[string]any{"@id": "urn:ontology:" + id},
		"dcs:semanticPath":  semanticPath,
		"dcs:required":      true,
	}
}

func policyFindingRuleIDs(findings []PolicyFinding) []string {
	ruleIDs := make([]string, len(findings))
	for i, finding := range findings {
		ruleIDs[i] = finding.RuleID
	}
	return ruleIDs
}
