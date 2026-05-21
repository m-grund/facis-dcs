package negotiationmerging

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
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

	var contractData ContractData
	err = json.Unmarshal(*contract.ContractData, &contractData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal contract data: %w", err)
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

			for _, value := range change.ContractData.SemanticConditionValues {
				upsertSemanticConditionValue(&contractData, value)
			}

			newContractData, err := datatype.NewJSON(contractData)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal contract data: %w", err)
			}
			updateData.ContractData = &newContractData
		}
	}

	return &updateData, nil
}

func upsertSemanticConditionValue(contract *ContractData, newValue SemanticConditionValue) {
	for i, existing := range contract.SemanticConditionValues {
		if existing.BlockID == newValue.BlockID &&
			existing.ParameterName == newValue.ParameterName &&
			existing.ConditionID == newValue.ConditionID {

			contract.SemanticConditionValues[i] = newValue // update
			return
		}
	}
	contract.SemanticConditionValues = append(contract.SemanticConditionValues, newValue) // insert
}
