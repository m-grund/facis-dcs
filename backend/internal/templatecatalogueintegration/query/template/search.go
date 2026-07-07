package template

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	catalogueevents "digital-contracting-service/internal/templatecatalogueintegration/event"
)

type SearchQry struct {
	DID            string
	DocumentNumber string
	Version        int
	Name           string
	Description    string
	Offset         int
	Limit          int
	RetrievedBy    string
	HolderDID      string
	UserRoles      userrole.UserRoles
}

type SearchHandler struct {
	DB       *sqlx.DB
	FCClient *client.FederatedCatalogueClient
}

const searchTemplatesCountStatementTemplate = `
MATCH (ct)
WHERE ct.templateUuid IS NOT NULL
  AND head(ct.claimsGraphUri) IS NOT NULL
%s
RETURN count(ct) AS total
`

const searchTemplatesStatementTemplate = `
MATCH (ct)
WHERE ct.templateUuid IS NOT NULL
  AND head(ct.claimsGraphUri) IS NOT NULL
%s
RETURN {
  did: head(ct.claimsGraphUri),
  name: ct.name,
  description: ct.description,
  version: ct.version,
  state: ct.state,
  template_uuid: ct.templateUuid
} AS n
SKIP %d
LIMIT %d
`

func (h *SearchHandler) Handle(ctx context.Context, qry SearchQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	whereClause, params := buildSearchWhereClause(qry)
	where := formatSearchWhereSection(whereClause)

	countStatement := fmt.Sprintf(searchTemplatesCountStatementTemplate, where)
	countResp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement:  countStatement,
		Parameters: params,
	})
	if err != nil {
		return nil, err
	}

	totalCount := countResp.TotalCount

	limit := qry.Limit
	if limit < 1 {
		limit = totalCount
	}

	statement := fmt.Sprintf(searchTemplatesStatementTemplate, where, qry.Offset, limit)
	dataResp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement:  statement,
		Parameters: params,
	})
	if err != nil {
		return nil, err
	}

	if h.DB != nil {
		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not create transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		evt := catalogueevents.SearchEvent{
			RetrievedBy: qry.RetrievedBy,
			OccurredAt:  time.Now().UTC(),
			HolderDID:   qry.HolderDID,
			UserRoles:   qry.UserRoles,
		}
		err = event.Create(ctx, tx, evt, componenttype.TemplateCatalogueIntegration)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}
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

func formatSearchWhereSection(whereClause string) string {
	if whereClause == "" {
		return ""
	}
	return "AND " + whereClause
}

func buildSearchWhereClause(qry SearchQry) (string, map[string]string) {
	conditions := make([]string, 0, 4)
	params := make(map[string]string)

	if value := strings.TrimSpace(qry.DID); value != "" {
		conditions = append(conditions, "toLower(head(ct.claimsGraphUri)) CONTAINS toLower($did)")
		params["did"] = value
	}
	if qry.Version > 0 {
		conditions = append(conditions, "ct.version = toString($version)")
		params["version"] = strconv.Itoa(qry.Version)
	}
	if value := strings.TrimSpace(qry.Name); value != "" {
		conditions = append(conditions, "toLower(coalesce(ct.name, '')) CONTAINS toLower($name)")
		params["name"] = value
	}
	if value := strings.TrimSpace(qry.Description); value != "" {
		conditions = append(conditions, "toLower(coalesce(ct.description, '')) CONTAINS toLower($description)")
		params["description"] = value
	}

	return strings.Join(conditions, " AND "), params
}
