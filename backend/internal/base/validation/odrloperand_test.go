package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeODRLOperandUsesTaxonomyIRIs(t *testing.T) {
	require.Equal(t, map[string]any{
		"@id": "https://w3id.org/facis/dcs/taxonomy/v1#country-AUT",
	}, TypeODRLOperand("company.location.country", "AUT"))
	require.Equal(t, map[string]any{
		"@id": "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR",
	}, TypeODRLOperand("contract.payment.currency", "EUR"))
}

func TestSemanticDataTypeUsesOntologyAndXSDTypes(t *testing.T) {
	require.Equal(t, "dcs:CountryReference", SemanticDataType("company.location.country"))
	require.Equal(t, "dcs:CurrencyReference", SemanticDataType("contract.payment.currency"))
	require.Equal(t, "xsd:decimal", SemanticDataType("service.sla.availability"))
	require.Equal(t, "xsd:date", SemanticDataType("contract.payment.dueDate"))
	require.Equal(t, "xsd:string", SemanticDataType("company.legalName"))
}

func TestSemanticObjectTypeUsesDomainFieldOntologyType(t *testing.T) {
	require.Equal(t, "dcs:PaymentTerm", SemanticObjectType("contract.payment.amount"))
	require.Equal(t, "dcs:SLO", SemanticObjectType("service.sla.availability"))
	require.Equal(t, "dcs:CompanyParty", SemanticObjectType("company.legalName"))
}

func TestTypeODRLOperandUsesTypedLiterals(t *testing.T) {
	require.Equal(t, map[string]any{
		"@type":  "xsd:decimal",
		"@value": "99.9",
	}, TypeODRLOperand("service.sla.availability", 99.9))
	require.Equal(t, map[string]any{
		"@type":  "xsd:date",
		"@value": "2027-12-31",
	}, TypeODRLOperand("contract.payment.dueDate", "2027-12-31"))
	require.Equal(t, map[string]any{
		"@type":  "xsd:boolean",
		"@value": "true",
	}, TypeODRLOperand("", true))
	require.Equal(t, map[string]any{
		"@type":  "xsd:dateTime",
		"@value": "2027-12-31T23:59:59Z",
	}, TypeODRLOperand("", "2027-12-31T23:59:59Z"))
}

func TestTypeODRLOperandUsesOntologyIRIsAndFreeTextLiterals(t *testing.T) {
	require.Equal(t, map[string]any{
		"@id": "https://w3id.org/facis/dcs/ontology/v1#Provider",
	}, TypeODRLOperand("", "dcs:Provider"))
	require.Equal(t, map[string]any{
		"@type":  "xsd:string",
		"@value": "individually negotiated",
	}, TypeODRLOperand("", "individually negotiated"))
}
