package negotiationmerging

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

func MergeChangeRequests(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, nRepo db.NegotiationRepo, did string, contractVersion int) (*db.ContractUpdateData, error) {
	changeRequests, err := nRepo.ReadAllAcceptedByContractDIDAndVersion(ctx, tx, did, contractVersion)
	if err != nil {
		return nil, err
	}

	contract, err := cRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, err
	}

	var contractData map[string]any
	err = json.Unmarshal(*contract.ContractData, &contractData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal contract data: %w", err)
	}
	if contractData == nil {
		contractData = map[string]any{}
	}

	updateData := db.ContractUpdateData{
		DID: contract.DID,
	}
	for _, changeRequest := range changeRequests {

		var change ChangeRequest
		if err := json.Unmarshal(*changeRequest.ChangeRequest, &change); err != nil {
			return nil, fmt.Errorf("could not unmarshal change request: %w", err)
		}

		if change.Name != nil {
			updateData.Name = change.Name
		}

		if change.Description != nil {
			updateData.Description = change.Description
		}

		if change.StartDate != nil {
			sDate, err := time.Parse(time.RFC3339, *change.StartDate)
			if err != nil {
				return nil, err
			}
			updateData.StartDate = &sDate
		}

		if change.ExpDate != nil {
			eDate, err := time.Parse(time.RFC3339, *change.ExpDate)
			if err != nil {
				return nil, err
			}
			updateData.ExpDate = &eDate
		}

		if change.ExpNoticePeriod != nil {
			updateData.ExpNoticePeriod = change.ExpNoticePeriod
		}

		if change.ExpPolicy != nil {
			updateData.ExpPolicy = change.ExpPolicy
		}

		if change.ContractData != nil {
			semanticConditionValues, err := readSemanticConditionValues(contractData)
			if err != nil {
				return nil, err
			}

			for _, value := range change.ContractData.SemanticConditionValues {
				semanticConditionValues = upsertSemanticConditionValue(semanticConditionValues, value)
			}
			contractData["semanticConditionValues"] = semanticConditionValues

			newContractData, err := datatype.NewJSON(contractData)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal contract data: %w", err)
			}
			normalizedContractData, err := validation.NormalizeContractDataForPersistence(&newContractData, contract.DID, true)
			if err != nil {
				return nil, fmt.Errorf("contract data validation failed after merging change requests: %w", err)
			}
			updateData.ContractData = normalizedContractData
		}
	}

	return &updateData, nil
}

func readSemanticConditionValues(contractData map[string]any) ([]SemanticConditionValue, error) {
	raw, ok := contractData["semanticConditionValues"]
	if !ok || raw == nil {
		return []SemanticConditionValue{}, nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal semantic condition values: %w", err)
	}
	var values []SemanticConditionValue
	if err := json.Unmarshal(bytes, &values); err != nil {
		return nil, fmt.Errorf("failed to unmarshal semantic condition values: %w", err)
	}
	return values, nil
}

func upsertSemanticConditionValue(values []SemanticConditionValue, newValue SemanticConditionValue) []SemanticConditionValue {
	for i, existing := range values {
		if existing.BlockID == newValue.BlockID &&
			existing.ParameterName == newValue.ParameterName &&
			existing.ConditionID == newValue.ConditionID {

			values[i] = newValue // update
			return values
		}
	}
	return append(values, newValue) // insert
}
