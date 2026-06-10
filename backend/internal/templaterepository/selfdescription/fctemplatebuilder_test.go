package selfdescription

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestBuildTemplateResourceSelfDescriptionIncludesSemanticEntityContext(t *testing.T) {
	templateData, err := datatype.NewJSON(map[string]any{
		"templateDataVersion": 1,
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "customer",
				"conditionName": "Customer",
				"schemaVersion": "v1",
				"entityType":    "CompanyParty",
				"entityRole":    "customer",
				"parameters": []any{
					map[string]any{
						"parameterName": "legalName",
						"type":          "string",
						"schemaRef":     "facis.dcs.party.v1",
						"semanticPath":  "company.legalName",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	selfDescription := BuildTemplateResourceSelfDescription(TemplateResourceInput{
		ParticipantID:  "did:web:facis.example:participant:1",
		DID:            "did:web:facis.example:template:1",
		DocumentNumber: "DOC-1",
		Version:        1,
		TemplateType:   "CONTRACT",
		Name:           "Customer template",
		Description:    "Template with customer entity metadata",
		CreatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		TemplateData:   &templateData,
	})

	credentials := selfDescription["verifiableCredential"].([]interface{})
	credential := credentials[0].(map[string]interface{})
	subject := credential["credentialSubject"].(map[string]interface{})
	context := subject["@context"].(map[string]interface{})

	require.Equal(t, "dcs-template-data:entityType", context["entityType"])
	require.Equal(t, "dcs-template-data:entityRole", context["entityRole"])
	require.Equal(t, "dcs-template-data:schemaRef", context["schemaRef"])
	require.Equal(t, "dcs-template-data:semanticPath", context["semanticPath"])
	require.Equal(t, "dcs-template-data:fixedValue", context["fixedValue"])

	var embeddedTemplateData map[string]any
	rawTemplateData := subject["dcs-template:templateData"].(*datatype.JSON)
	require.NoError(t, json.Unmarshal(*rawTemplateData, &embeddedTemplateData))
	condition := embeddedTemplateData["semanticConditions"].([]any)[0].(map[string]any)
	require.Equal(t, "CompanyParty", condition["entityType"])
	require.Equal(t, "customer", condition["entityRole"])
}
