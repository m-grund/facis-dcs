package template

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetAllMetadataQry struct {
	Offset int
	Limit  int
}

type GetAllMetadataHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplatesCountStatement = `
MATCH (n:ContractTemplate)
RETURN count(n) AS total
`

// TODO: fix FC GraphDB issue
const retrieveTemplatesStatementTemplate = `
MATCH (ct:ContractTemplate)
OPTIONAL MATCH (ct)-[:metadata]->(m)
RETURN {
  did: head(ct.claimsGraphUri),
  document_number: m.documentNumber,
  version: m.templateVersion,
  schema_version: m.schemaVersion,
  name: m.name,
  description: m.description,
  template_type: m.templateType,
  participant_id: m.createdBy,
  created_at: m.createdAt,
  updated_at: m.updatedAt
} AS n
SKIP %d
LIMIT %d
`

func (h *GetAllMetadataHandler) Handle(qry GetAllMetadataQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	countResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement: retrieveTemplatesCountStatement,
	})
	if err != nil {
		return nil, err
	}

	totalCount := countResp.TotalCount

	limit := qry.Limit
	if limit < 1 {
		limit = totalCount
	}

	statement := fmt.Sprintf(retrieveTemplatesStatementTemplate, qry.Offset, limit)
	dataResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement: statement,
	})
	if err != nil {
		return nil, err
	}

	items := make([]*templatecatalogueintegration.TemplateCatalogueItem, 0, len(dataResp.Items))
	for _, item := range dataResp.Items {
		if ct := projectionMap(item); ct != nil {
			if mapped := mapCatalogueItem(ct); mapped != nil {
				items = append(items, mapped)
			}
		}
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

func projectionMap(row map[string]interface{}) map[string]interface{} {
	if row == nil {
		return nil
	}
	for _, value := range row {
		if mapped, ok := value.(map[string]interface{}); ok {
			return mapped
		}
	}
	return nil
}

func mapCatalogueItem(ct map[string]interface{}) *templatecatalogueintegration.TemplateCatalogueItem {
	if ct == nil {
		return nil
	}
	did := ptr.StringFromMap(ct, "did")
	if strings.TrimSpace(did) == "" {
		return nil
	}
	return &templatecatalogueintegration.TemplateCatalogueItem{
		Did:            did,
		DocumentNumber: ptr.Ref(ptr.StringFromMap(ct, "document_number")),
		Version:        ptr.Ref(ptr.IntFromMap(ct, "version")),
		SchemaVersion:  ptr.Ref(ptr.IntFromMap(ct, "schema_version")),
		Name:           ptr.Ref(ptr.StringFromMap(ct, "name")),
		Description:    ptr.Ref(ptr.StringFromMap(ct, "description")),
		TemplateType:   ptr.Ref(ptr.StringFromMap(ct, "template_type")),
		ParticipantID:  ptr.Ref(ptr.StringFromMap(ct, "participant_id")),
		CreatedAt:      ptr.Ref(ptr.StringFromMap(ct, "created_at")),
		UpdatedAt:      ptr.Ref(ptr.StringFromMap(ct, "updated_at")),
	}
}

func parseTemplateDataJSON(templateDataJSON string) (any, error) {
	if strings.TrimSpace(templateDataJSON) == "" {
		return nil, nil
	}

	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(templateDataJSON), &templateData); err != nil {
		return nil, fmt.Errorf("unmarshal templateDataJSON failed: %w", err)
	}

	return templateData, nil
}
