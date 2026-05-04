package contractworkflowengine

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"encoding/json"
	"fmt"

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

	mergedChanges := false
	updateData := db.ContractUpdateData{
		DID: contract.DID,
	}
	for _, changeRequest := range changeRequests {
		changes, err := changeRequestToMap(changeRequest.ChangeRequest)
		if err != nil {
			return err
		}

		name, ok := changes["name"]
		if ok {
			s := name.(string)
			updateData.Name = &s
		}

		description, ok := changes["description"]
		if ok {
			s := description.(string)
			updateData.Description = &s
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

func changeRequestToMap(req *datatype.JSON) (map[string]interface{}, error) {
	if req == nil {
		return nil, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(*req, &result); err != nil {
		return nil, fmt.Errorf("could not unmarshal change request: %w", err)
	}
	return result, nil
}
