package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
)

func TestApprove_ApproveContractInReviewedState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    approver,
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract : %v", err)
	}

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
	contract, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract : %v", err)
	}

	assert.Equal(t, contractstate.Approved, contract.State)
}

func TestApprove_ApproveContractInReviewedStateStoresArchiveEntry(t *testing.T) {
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

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	approver := "Test User 1"
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    approver,
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to approve contract: %v", err)
	}

	type archiveEntry struct {
		Count         int    `db:"count"`
		StoredBy      string `db:"stored_by"`
		ArchiveStatus string `db:"archive_status"`
	}
	var entry archiveEntry
	err = db.GetContext(ctx, &entry, `
		SELECT COUNT(*) AS count,
		       COALESCE(MAX(stored_by), '') AS stored_by,
		       COALESCE(MAX(archive_status::text), '') AS archive_status
		FROM contract_archive_entries
		WHERE did = $1 AND contract_version = $2
	`, *did, 1)
	if err != nil {
		t.Fatalf("Failed to read archive entry: %v", err)
	}

	assert.Equal(t, 1, entry.Count)
	assert.Equal(t, approver, entry.StoredBy)
	assert.Equal(t, "STORED", entry.ArchiveStatus)

	var eventCount int
	err = db.GetContext(ctx, &eventCount, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE component = $1
		  AND event_type = $2
		  AND did = $3
		  AND event_data->>'stored_by' = $4
		  AND (event_data->>'contract_version')::int = $5
	`, componenttype.ContractStorageArchive.String(), eventtype.StoreArchived.String(), *did, approver, 1)
	if err != nil {
		t.Fatalf("Failed to read archive store event: %v", err)
	}

	assert.Equal(t, 1, eventCount)
}

func TestApprove_ApproveNonExistingContract(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractInReviewedStateWithInvalidUser(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	creator := "Test User"

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 1")

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 2",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestApprove_ApproveContractInDraftState(t *testing.T) {

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

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractInApprovedState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Approved, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractAfterUpdate(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().Add(-5 * time.Second),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
