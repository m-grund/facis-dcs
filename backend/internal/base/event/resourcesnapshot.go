package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"

	"github.com/jmoiron/sqlx"
)

var resourceSnapshotEventTypes = map[string]map[string]bool{
	componenttype.ContractWorkflowEngine.String(): {
		"CREATE_CONTRACT":           true,
		"SUBMIT_CONTRACT":           true,
		"NEGOTIATE_CONTRACT":        true,
		"ACCEPT_RESPOND_CONTRACT":   true,
		"REJECT_RESPOND_CONTRACT":   true,
		"INCREASE_CONTRACT_VERSION": true,
		"APPROVE_CONTRACT":          true,
		"REJECT_CONTRACT":           true,
		"VERIFY_CONTRACT":           true,
		"UPDATE_CONTRACT":           true,
		"REVIEW_CONTRACT":           true,
		"TERMINATE_CONTRACT":        true,
		"RECORD_EVIDENCE":           true,
	},
	componenttype.ContractTemplateRepo.String(): {
		"CREATE_CONTRACT_TEMPLATE":   true,
		"SUBMIT_CONTRACT_TEMPLATE":   true,
		"APPROVE_CONTRACT_TEMPLATE":  true,
		"REJECT_CONTRACT_TEMPLATE":   true,
		"VERIFY_CONTRACT_TEMPLATE":   true,
		"UPDATE_CONTRACT_TEMPLATE":   true,
		"ARCHIVE_CONTRACT_TEMPLATE":  true,
		"REGISTER_CONTRACT_TEMPLATE": true,
	},
}

func shouldStoreResourceData(component string, eventType string) bool {
	return resourceSnapshotEventTypes[component][eventType]
}

func readCurrentResourceData(ctx context.Context, tx *sqlx.Tx, component string, did *string) (json.RawMessage, error) {
	if did == nil || len(*did) <= 1 {
		return nil, nil
	}

	var query string
	switch component {
	case componenttype.ContractTemplateRepo.String():
		query = `
			SELECT jsonb_build_object(
				'did', did,
				'document_number', document_number,
				'version', version,
				'state', state,
				'template_type', template_type,
				'name', name,
				'description', description,
				'created_by', created_by,
				'created_at', created_at,
				'updated_at', updated_at,
				'template_data', template_data
			)
			FROM contract_templates
			WHERE did = $1
		`
	case componenttype.ContractWorkflowEngine.String():
		query = `
			SELECT jsonb_build_object(
				'did', did,
				'contract_version', contract_version,
				'state', state,
				'name', name,
				'description', description,
				'created_by', created_by,
				'created_at', created_at,
				'updated_at', updated_at,
				'expiration_date', expiration_date,
				'contract_data', contract_data
			)
			FROM contracts_effective
			WHERE did = $1
		`
	default:
		return nil, nil
	}

	var data datatype.JSON
	if err := tx.GetContext(ctx, &data, query, *did); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read current resource data: %w", err)
	}

	return json.RawMessage(data), nil
}

func eventDataWithResourceData(eventData []byte, resourceData json.RawMessage) ([]byte, error) {
	if len(resourceData) == 0 {
		return eventData, nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(eventData, &payload); err != nil {
		return nil, fmt.Errorf("decode event data for resource snapshot: %w", err)
	}
	payload["resource_data"] = resourceData

	result, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode event data with resource snapshot: %w", err)
	}
	return result, nil
}

func splitResourceData(eventData []byte) (json.RawMessage, []byte) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(eventData, &payload); err != nil {
		return nil, eventData
	}

	resourceData := payload["resource_data"]
	if len(resourceData) == 0 {
		return nil, eventData
	}

	delete(payload, "resource_data")
	sanitizedEventData, err := json.Marshal(payload)
	if err != nil {
		return resourceData, eventData
	}

	return resourceData, sanitizedEventData
}
