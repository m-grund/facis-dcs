package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOntologyDomainFieldsIncludeContractPartyRole(t *testing.T) {
	fields, err := loadOntologyDomainFields()
	require.NoError(t, err)

	role, ok := fields["company.role"]
	require.True(t, ok)
	require.Equal(t, SchemaPartyV1, role.SchemaRef)
	require.Equal(t, "string", role.Type)
	require.Equal(t, "company.role", role.DomainPath)
	require.Equal(t, expandOntologyResource("dcst:field-company-role"), role.OntologyTerm)
	require.Equal(t, "party.role", role.StatementField)
	require.NotNil(t, role.Constraint)
	require.Equal(t, []string{"supplier", "customer", "provider", "client"}, role.Constraint.AllowedValues)

	uriRole, ok := fields[expandOntologyResource("dcst:field-company-role")]
	require.True(t, ok)
	require.Equal(t, role, uriRole)

	legacyRole, ok := fields["company_role"]
	require.True(t, ok)
	require.Equal(t, role, legacyRole)

	paymentAmount, ok := fields[expandOntologyResource("dcst:field-contract-payment-amount")]
	require.True(t, ok)
	require.Equal(t, "payment.amount", paymentAmount.StatementField)
}

func TestParseOntologyDomainFieldsRequiresReferencedConstraints(t *testing.T) {
	_, err := parseOntologyDomainFields(`
dcst:field-company-role a dcs:DomainField ;
  dcs:semanticPath "company.role" ;
  dcs:schemaRef "facis.dcs.party.v1" ;
  dcs:parameterType "string" ;
  dcs:hasValueConstraint dcst:missing-constraint ;
  rdfs:label "Company Contract Role" .
`)

	require.ErrorContains(t, err, "unknown value constraint")
}
