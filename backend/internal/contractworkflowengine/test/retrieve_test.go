package test

import (
	"context"
	"slices"
	"sort"
	"testing"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"

	"github.com/stretchr/testify/assert"
)

func TestRetrieve_RetrieveContractById(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Draft, creator)

	qry := contract.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	contractItem, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractItem.DID, *did)
	assert.Equal(t, contractstate.Draft, contractItem.State)
}

func TestRetrieve_RetrieveNonExistingContractById(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Draft, creator)

	did2, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}
	qry := contract.GetByIDQry{
		DID:         *did2,
		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
	}
	_, err = queryHandler.Handle(ctx, qry)

	assert.NotNil(t, err)
}

func TestRetrieve_RetrieveAllContracts(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	dids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		did, err := base.GetDID(datatype.ContractResourceType)
		if err != nil {
			t.Fatalf("Failed to get new DID: %v", err)
		}
		dids = append(dids, *did)
		createContract(t, db, repo, did, contractstate.Draft, creator)
	}
	sort.Strings(dids)

	state := contractstate.Draft
	qry := contract.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
		State:       &state,
	}
	queryHandler := contract.GetAllMetaDataByFilterHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	for _, ct := range result {
		assert.Equal(t, contractstate.Draft, ct.State)

		if !slices.Contains(dids, ct.DID) {
			t.Errorf("DID not found in retrieved contract: %v", ct.DID)
		}
	}
}
func TestRetrieve_RetrieveArchivedContracts(t *testing.T) {
	db := setupTestDB(t)
	cleanupContractTable(t, db)

	creator := "Test User"
	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	approvedDID, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	createContract(t, db, repo, approvedDID, contractstate.Approved, creator)

	excludedStates := []contractstate.ContractState{
		contractstate.Draft,
		contractstate.Negotiation,
		contractstate.Submitted,
		contractstate.Reviewed,
		contractstate.Rejected,
		contractstate.Terminated,
		contractstate.Expired,
	}
	for _, state := range excludedStates {
		did, err := base.GetDID(datatype.ContractResourceType)
		if err != nil {
			t.Fatalf("Failed to get new DID: %v", err)
		}
		createContract(t, db, repo, did, state, creator)
	}

	qry := contract.GetArchivedContractsQry{
		RetrievedBy: creator,
	}
	queryHandler := contract.GetArchivedContractsHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to retrieve archived contracts: %v", err)
	}

	assert.Len(t, result.Contracts, 1)
	assert.Equal(t, *approvedDID, result.Contracts[0].DID)
	assert.Equal(t, contractstate.Approved, result.Contracts[0].State)
}

func TestRetrieve_SearchArchivedContractsByName(t *testing.T) {
	db := setupTestDB(t)
	cleanupContractTable(t, db)

	creator := "Test User"
	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	approvedDID, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}
	createContract(t, db, repo, approvedDID, contractstate.Approved, creator)

	draftDID, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}
	createContract(t, db, repo, draftDID, contractstate.Draft, creator)

	_, err = db.Exec("UPDATE contracts SET name = $2 WHERE did = $1", *approvedDID, "Archived Contract")
	if err != nil {
		t.Fatalf("Failed to update approved contract name: %v", err)
	}
	_, err = db.Exec("UPDATE contracts SET name = $2 WHERE did = $1", *draftDID, "Archived Contract")
	if err != nil {
		t.Fatalf("Failed to update draft contract name: %v", err)
	}

	qry := contract.SearchArchivedContractsQry{
		RetrievedBy: creator,
		Name:        "Archived",
	}
	queryHandler := contract.GetArchivedContractsHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}

	result, err := queryHandler.Search(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to search archived contracts: %v", err)
	}

	assert.Len(t, result.Contracts, 1)
	assert.Equal(t, *approvedDID, result.Contracts[0].DID)
	assert.Equal(t, contractstate.Approved, result.Contracts[0].State)
}
