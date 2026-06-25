package contracttemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

type GetTemplateDataByDIDQry struct {
	DID string
}

type GetTemplateDataByDIDHandler struct {
	Ctx      context.Context
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	FCClient *fcclient.FederatedCatalogueClient
}

// TODO: fix FC GraphDB issue
const retrieveTemplateDataJSONByDIDStatement = `
MATCH (ct:ContractTemplate)
WHERE head(ct.claimsGraphUri) = $did
OPTIONAL MATCH (m:TemplateMetadata {did: $did})
RETURN ct.templateDataJSON AS template_data_json, ct.version AS version
LIMIT 1
`

func (h *GetTemplateDataByDIDHandler) Handle(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, error) {
	templateData, version, err := h.getTemplateData(ctx, qry)
	if err != nil {
		return nil, err
	}
	return convertTemplateDataToContractData(templateData, qry.DID, version)
}

func (h *GetTemplateDataByDIDHandler) getTemplateData(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, int, error) {
	templateData, version, err := h.getFrameContractTemplateDataFromDB(ctx, qry.DID)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read template data from DB: %w", err)
	}

	if templateData == nil && h.FCClient != nil {
		templateData, version, err = h.getTemplateDataFromFC(qry)
		if err != nil {
			return nil, 0, fmt.Errorf("could not read template data from FC: %w", err)
		}
	}

	return templateData, version, nil
}

func (h *GetTemplateDataByDIDHandler) getFrameContractTemplateDataFromDB(ctx context.Context, templateDID string) (*datatype.JSON, int, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	templateData, err := h.CTRepo.ReadFrameContractTemplateDataByID(ctx, tx, templateDID)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read frame contract template data: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, 0, fmt.Errorf("could not commit transaction: %w", err)
	}
	return templateData.TemplateData, templateData.Version, nil
}

func (h *GetTemplateDataByDIDHandler) getTemplateDataFromFC(qry GetTemplateDataByDIDQry) (*datatype.JSON, int, error) {
	resp, err := h.FCClient.Query(h.Ctx, fcclient.QueryRequest{
		Statement: retrieveTemplateDataJSONByDIDStatement,
		Parameters: map[string]string{
			"did": qry.DID,
		},
	})
	if err != nil {
		return nil, 0, err
	}
	if resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, 0, fmt.Errorf("template data not found for did %s", qry.DID)
	}

	row := resp.Items[0]
	templateDataJSONString := bindingString(row, "template_data_json")
	if strings.TrimSpace(templateDataJSONString) == "" {
		return nil, 0, fmt.Errorf("templateDataJSON is empty for did %s", qry.DID)
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal([]byte(templateDataJSONString), &templateDataMap); err != nil {
		return nil, 0, fmt.Errorf("unmarshal templateDataJSON failed: %w", err)
	}

	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal template data failed: %w", err)
	}

	return &templateData, bindingInt(row, "version"), nil
}

func bindingString(row map[string]interface{}, variable string) string {
	if row == nil {
		return ""
	}

	raw, ok := row[variable]
	if !ok {
		return ""
	}

	switch typed := raw.(type) {
	case string:
		return typed
	case map[string]interface{}:
		if value, ok := typed["value"].(string); ok {
			return value
		}
	}

	return ""
}

func bindingInt(row map[string]interface{}, variable string) int {
	raw := bindingString(row, variable)
	if raw == "" {
		if v, ok := row[variable]; ok {
			switch typed := v.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			}
		}
		return 0
	}

	n, err := strconv.Atoi(raw)
	if err == nil {
		return n
	}

	return 0
}

func convertTemplateDataToContractData(raw *datatype.JSON, templateDID string, templateVersions ...int) (*datatype.JSON, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return raw, nil
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*raw, &templateDataMap); err != nil {
		return nil, fmt.Errorf("unmarshal template data failed: %w", err)
	}

	if _, ok := templateDataMap["dcs:documentStructure"]; !ok {
		return nil, errors.New("template data must use the canonical dcs:documentStructure envelope")
	}

	templateDataMap["@type"] = "dcs:Contract"
	if metadata, ok := templateDataMap["dcs:metadata"].(map[string]interface{}); ok {
		metadata["@type"] = "dcs:ContractMetadata"
	}
	templateDataMap["sourceTemplate"] = map[string]interface{}{
		"did": templateDID,
	}

	if len(templateVersions) > 0 && templateVersions[0] > 0 {
		templateDataMap["sourceTemplate"].(map[string]interface{})["version"] = templateVersions[0]
	}

	if metadata, ok := templateDataMap["dcs:metadata"].(map[string]interface{}); ok {
		if _, exists := templateDataMap["sourceTemplate"].(map[string]interface{})["version"]; !exists {
			if version, exists := metadata["dcs:templateVersion"]; exists {
				templateDataMap["sourceTemplate"].(map[string]interface{})["version"] = version
			}
		}
	}
	templateDataMap["derivedFromTemplate"] = templateDID
	templateDataMap["semanticConditionValues"] = []any{}

	contractData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal converted contract data failed: %w", err)
	}

	return validation.NormalizeContractData(&contractData, false)
}
