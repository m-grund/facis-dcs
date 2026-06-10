package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOntologyDomainFieldsExcludeContractPartyRole(t *testing.T) {
	fields, err := loadOntologyDomainFields()
	require.NoError(t, err)

	_, ok := fields["company.role"]
	require.False(t, ok)

	_, ok = fields[expandOntologyResource("dcst:field-company-role")]
	require.False(t, ok)

	_, ok = fields["company_role"]
	require.False(t, ok)

	require.Equal(t, []string{"supplier", "customer", "provider", "client"}, ontologyRuntime.EntityRoleAllowedValues)

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
