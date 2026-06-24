package template

import (
	"context"
	"fmt"
	"strconv"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetByIDQry struct {
	DID     string
	Version int
}

type GetByIDHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// TODO: fix FC GraphDB issue
const retrieveTemplateByIDStatement = `
MATCH (ct:ContractTemplate)
WHERE head(ct.claimsGraphUri) = $did
OPTIONAL MATCH (m:TemplateMetadata {did: head(ct.claimsGraphUri)})
WHERE toInteger(coalesce(m.templateVersion, ct.templateVersion, ct.version, $version)) = toInteger($version)
RETURN {
  did: head(ct.claimsGraphUri)),
  document_number: ct.documentNumber,
  version: ct.version,
  schema_version: ct.schemaVersion,
  template_data_json: ct.templateDataJSON,
  name: ct.name,
  description: ct.description,
  template_type: ct.templateType,
  participant_id: ct.participantId,
  created_at: ct.createdAt,
  updated_at: ct.updatedAt
} AS n
LIMIT 1
`

func (h *GetByIDHandler) Handle(qry GetByIDQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.DID == "" {
		return nil, fmt.Errorf("did is empty")
	}
	if qry.Version < 1 {
		return nil, fmt.Errorf("version must be greater than 0")
	}

	resp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement: retrieveTemplateByIDStatement,
		Parameters: map[string]string{
			"did":     qry.DID,
			"version": strconv.Itoa(qry.Version),
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, nil
	}

	n := projectionMap(resp.Items[0])
	if n == nil {
		return nil, fmt.Errorf("query projection missing projected map for did=%s", qry.DID)
	}

	templateData, err := parseTemplateDataJSON(ptr.StringFromMap(n, "template_data_json"))
	if err != nil {
		return nil, err
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse{
		Did:            ptr.StringFromMap(n, "did"),
		DocumentNumber: ptr.Ref(ptr.StringFromMap(n, "document_number")),
		Version:        ptr.Ref(ptr.IntFromMap(n, "version")),
		SchemaVersion:  ptr.Ref(ptr.IntFromMap(n, "schema_version")),
		TemplateData:   templateData,
		Name:           ptr.Ref(ptr.StringFromMap(n, "name")),
		Description:    ptr.Ref(ptr.StringFromMap(n, "description")),
		TemplateType:   ptr.Ref(ptr.StringFromMap(n, "template_type")),
		ParticipantID:  ptr.Ref(ptr.StringFromMap(n, "participant_id")),
		CreatedAt:      ptr.Ref(ptr.StringFromMap(n, "created_at")),
		UpdatedAt:      ptr.Ref(ptr.StringFromMap(n, "updated_at")),
	}, nil
}
