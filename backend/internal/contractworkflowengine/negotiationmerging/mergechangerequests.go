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

func MergeChangeRequests(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, nRepo db.NegotiationRepo, did string, contractVersion *int) error {
	changeRequests, err := nRepo.ReadAllAcceptedByContractDIDAndVersion(ctx, tx, did, contractVersion)
	if err != nil {
		return err
	}

	contract, err := cRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return err
	}

	var contractData ContractData
	err = json.Unmarshal(*contract.ContractData, &contractData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal contract data: %w", err)
	}

	mergedChanges := false
	updateData := db.ContractUpdateData{
		DID: contract.DID,
	}
	for _, changeRequest := range changeRequests {

		var change ChangeRequest
		if err := json.Unmarshal(*changeRequest.ChangeRequest, &change); err != nil {
			return fmt.Errorf("could not unmarshal change request: %w", err)
		}

		updateData.Name = change.Name
		updateData.Description = change.Description

		if change.ExpDate != nil {
			eDate, err := time.Parse(time.RFC3339, *change.ExpDate)
			if err != nil {
				return err
			}
			updateData.ExpDate = &eDate
		}

		updateData.ExpNoticePeriod = change.ExpNoticePeriod
		updateData.ExpPolicy = change.ExpPolicy

		if change.ContractData != nil {

			for _, value := range change.ContractData.SemanticConditionValues {
				upsertSemanticConditionValue(&contractData, value)
			}

			newContractData, err := datatype.NewJSON(contractData)
			if err != nil {
				return fmt.Errorf("failed to marshal contract data: %w", err)
			}
			updateData.ContractData = &newContractData
		}

		mergedChanges = true
	}

	if mergedChanges {
		err = cRepo.Update(ctx, tx, updateData)
		if err != nil {
			return err
		}
	}

	return nil
}

func upsertSemanticConditionValue(contract *ContractData, newValue SemanticConditionValue) {
	for i, existing := range contract.SemanticConditionValues {
		if existing.BlockID == newValue.BlockID && existing.ParameterName == newValue.ParameterName {
			contract.SemanticConditionValues[i] = newValue // update
			return
		}
	}
	contract.SemanticConditionValues = append(contract.SemanticConditionValues, newValue) // insert
}
