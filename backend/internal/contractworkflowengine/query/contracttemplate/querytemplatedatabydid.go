package contracttemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type GetTemplateDataByDIDQry struct {
	DID string
}

type GetTemplateDataByDIDHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *GetTemplateDataByDIDHandler) Handle(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, error) {
	templateData, version, err := h.getTemplateData(ctx, qry)
	if err != nil {
		return nil, err
	}
	return convertTemplateDataToContractData(templateData, qry.DID, version)
}

func (h *GetTemplateDataByDIDHandler) getTemplateData(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, int, error) {
	templateData, version, err := h.getContractTemplateDataFromDB(ctx, qry.DID)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read template data from DB: %w", err)
	}

	return templateData, version, nil
}

func (h *GetTemplateDataByDIDHandler) getContractTemplateDataFromDB(ctx context.Context, templateDID string) (*datatype.JSON, int, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	templateData, err := h.CTRepo.ReadContractTemplateDataByID(ctx, tx, templateDID)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read contract template data: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, 0, fmt.Errorf("could not commit transaction: %w", err)
	}
	return templateData.TemplateData, templateData.TemplateVersion, nil
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
