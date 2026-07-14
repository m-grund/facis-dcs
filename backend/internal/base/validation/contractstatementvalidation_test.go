package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func validContractStatementsForValidation() []map[string]any {
	return []map[string]any{
		{
			"@id":       "party-provider",
			"@type":     ontologyRuntime.RoleEntityType,
			"role":      canonicalEntityRole("provider"),
			"legalName": "Provider GmbH",
		},
		{
			"@id":       "party-customer",
			"@type":     ontologyRuntime.RoleEntityType,
			"role":      canonicalEntityRole("customer"),
			"legalName": "Customer GmbH",
		},
		{
			"@id":      "payment-main",
			"@type":    statementTypeByStatementField("payment.amount"),
			"amount":   1000.0,
			"currency": "EUR",
			"dueDate":  "2026-06-19",
		},
		{
			"@id":          "slo-availability",
			"@type":        statementTypeByStatementField("slo.availability"),
			"availability": 99.9,
		},
	}
}

func validationIssueIDs(issues []ValidationIssue) []string {
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.RuleID
	}
	return ids
}

func TestValidateContractStatementsAcceptsValidContract(t *testing.T) {
	issues := ValidateContractStatements(validContractStatementsForValidation(), defaultContractStatementValidationProfile())
	require.Empty(t, issues)
}

func TestValidateContractStatementsReportsMissingProvider(t *testing.T) {
	statements := []map[string]any{}
	for _, statement := range validContractStatementsForValidation() {
		if statement["@id"] == "party-provider" {
			continue
		}
		statements = append(statements, statement)
	}

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.Contains(t, validationIssueIDs(issues), "exactly-one-provider")
}

func TestValidateContractStatementsReportsDuplicateProvider(t *testing.T) {
	statements := append(validContractStatementsForValidation(), map[string]any{
		"@id":   "party-provider-2",
		"@type": ontologyRuntime.RoleEntityType,
		"role":  canonicalEntityRole("provider"),
	})

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.Equal(t, []string{"exactly-one-provider"}, validationIssueIDs(issues))
}

func TestValidateContractStatementsReportsMissingPaymentField(t *testing.T) {
	statements := validContractStatementsForValidation()
	delete(statements[2], "dueDate")

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.Equal(t, []string{"payment-required"}, validationIssueIDs(issues))
	require.Equal(t, "payment-main", issues[0].StatementID)
}

func TestValidateContractStatementsReportsNonPositivePaymentAmount(t *testing.T) {
	statements := validContractStatementsForValidation()
	statements[2]["amount"] = 0.0

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.Equal(t, []string{"payment-amount-positive"}, validationIssueIDs(issues))
	require.Equal(t, "payment-main", issues[0].StatementID)
}

func TestValidateContractStatementsReportsMissingSLO(t *testing.T) {
	statements := []map[string]any{}
	for _, statement := range validContractStatementsForValidation() {
		if statement["@id"] == "slo-availability" {
			continue
		}
		statements = append(statements, statement)
	}

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.Equal(t, []string{"availability-slo-required"}, validationIssueIDs(issues))
}

func TestValidateContractStatementsReportsMultipleValidationFailures(t *testing.T) {
	statements := []map[string]any{
		{
			"@id":    "payment-main",
			"@type":  statementTypeByStatementField("payment.amount"),
			"amount": 1000.0,
		},
	}

	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())

	require.ElementsMatch(t, []string{
		"exactly-one-provider",
		"exactly-one-customer",
		"payment-required",
		"availability-slo-required",
	}, validationIssueIDs(issues))
}

func TestValidateContractStatementsReportsUnknownRuleType(t *testing.T) {
	issues := ValidateContractStatements(validContractStatementsForValidation(), ValidationProfile{
		ID:      "test",
		Version: "1",
		Rules: []ValidationRule{
			{ID: "unknown", Type: "rego", Severity: "error"},
		},
	})

	require.Len(t, issues, 1)
	require.Equal(t, "unknown", issues[0].RuleID)
	require.Contains(t, issues[0].Message, "unknown validation rule type")
}

func TestValidateContractStatementsSupportsReusableRuleTypes(t *testing.T) {
	statements := append(validContractStatementsForValidation(), map[string]any{
		"@id":       "party-provider-duplicate-name",
		"@type":     ontologyRuntime.RoleEntityType,
		"role":      canonicalEntityRole("reseller"),
		"legalName": "Provider GmbH",
	})
	profile := ValidationProfile{
		ID:      "test",
		Version: "1",
		Rules: []ValidationRule{
			{
				ID:    "payment-exists",
				Type:  ValidationRuleExists,
				Where: map[string]any{"@type": statementTypeByStatementField("payment.amount")},
			},
			{
				ID:       "provider-role",
				Type:     ValidationRuleFieldValue,
				Target:   "role",
				Where:    map[string]any{"@id": "party-provider"},
				Operator: "eq",
				Value:    canonicalEntityRole("provider"),
			},
			{
				ID:       "positive-payment",
				Type:     ValidationRuleComparison,
				Target:   "amount",
				Where:    map[string]any{"@type": statementTypeByStatementField("payment.amount")},
				Operator: "gt",
				Value:    0,
			},
			{
				ID:     "unique-party-names",
				Type:   ValidationRuleUnique,
				Target: "legalName",
				Where:  map[string]any{"@type": ontologyRuntime.RoleEntityType},
			},
		},
	}

	issues := ValidateContractStatements(statements, profile)

	require.Equal(t, []string{"unique-party-names"}, validationIssueIDs(issues))
	require.Equal(t, "party-provider-duplicate-name", issues[0].StatementID)
}

func TestLoadValidationProfileRejectsInvalidDefinitions(t *testing.T) {
	_, err := LoadValidationProfileJSON([]byte(`{
		"id": "broken",
		"version": "1",
		"rules": [
			{"id": "missing-target", "type": "comparison", "operator": "gt", "value": 0}
		]
	}`))

	require.ErrorContains(t, err, "requires a target")
}

func TestLoadValidationProfileSupportsDefaultYAMLJSONAndYAML(t *testing.T) {
	yamlPath := filepath.Join("..", "..", "..", "..", "docs", "semantic-ontology", "validation", "facis.sla.basic.v1.yaml")
	raw, err := os.ReadFile(yamlPath)
	require.NoError(t, err)

	defaultProfile, err := LoadValidationProfileYAML(raw)
	require.NoError(t, err)
	require.Equal(t, "facis.sla.basic.v1", defaultProfile.ID)
	require.Contains(t, validationIssueIDs(ValidateContractStatements(
		[]map[string]any{{"@id": "payment-main", "@type": statementTypeByStatementField("payment.amount"), "amount": 0}},
		defaultProfile,
	)), "payment-amount-positive")

	fileProfile, err := LoadValidationProfileFile(yamlPath)
	require.NoError(t, err)
	require.Equal(t, defaultProfile.ID, fileProfile.ID)

	jsonProfile, err := LoadValidationProfileJSON([]byte(`{
		"id": "facis.marketplace.contract.v1",
		"version": "1",
		"rules": [
			{"id": "payment-exists", "type": "exists", "severity": "error", "where": {"@type": "https://w3id.org/facis/dcs/ontology/v1#PaymentTerm"}}
		]
	}`))
	require.NoError(t, err)
	require.Equal(t, "facis.marketplace.contract.v1", jsonProfile.ID)

	yamlProfile, err := LoadValidationProfileYAML([]byte(`
id: facis.marketplace.contract.v1
version: "1"
rules:
  - id: payment-exists
    type: exists
    severity: error
    where:
      "@type": https://w3id.org/facis/dcs/ontology/v1#PaymentTerm
`))
	require.NoError(t, err)
	require.Equal(t, "facis.marketplace.contract.v1", yamlProfile.ID)
}

func TestStatementQueryUtilities(t *testing.T) {
	statements := validContractStatementsForValidation()

	require.True(t, MatchesWhereClause(statements[0], map[string]any{"role": canonicalEntityRole("provider")}))
	require.Len(t, FindStatements(statements, map[string]any{"@type": ontologyRuntime.RoleEntityType}), 2)
	require.Equal(t, 1, CountStatements(statements, map[string]any{"@type": statementTypeByStatementField("payment.amount")}))
	require.Len(t, FilterStatements(statements, func(statement map[string]any) bool {
		_, ok := statement["availability"]
		return ok
	}), 1)
}
