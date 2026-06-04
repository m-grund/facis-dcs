package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
)

const testArchiveSnapshotCID = "bafy-test-archive-snapshot"

type archiveSnapshotStorerStub struct {
	cid      string
	payloads []any
}

func (s *archiveSnapshotStorerStub) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	s.payloads = append(s.payloads, data)
	cid := s.cid
	if cid == "" {
		cid = testArchiveSnapshotCID
	}
	result := &ipfs.IPFSResult{}
	result.Identifier.Value = cid
	return result, nil
}

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
		DB:         db,
		CRepo:      repo.CRepo,
		ATRepo:     repo.ATRepo,
		IPFSStorer: &archiveSnapshotStorerStub{},
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
	storer := &archiveSnapshotStorerStub{}
	handler := command.Approver{
		DB:         db,
		CRepo:      repo.CRepo,
		ATRepo:     repo.ATRepo,
		IPFSStorer: storer,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to approve contract: %v", err)
	}

	type archiveEntry struct {
		StoredBy          string        `db:"stored_by"`
		ArchiveStatus     string        `db:"archive_status"`
		ContentHash       string        `db:"content_hash"`
		SnapshotCID       string        `db:"snapshot_cid"`
		ContractSnapshot  datatype.JSON `db:"contract_snapshot"`
		SignatureMetadata datatype.JSON `db:"signature_metadata"`
		CredentialHashes  datatype.JSON `db:"credential_hashes"`
		Evidence          datatype.JSON `db:"evidence"`
	}
	var count int
	err = db.GetContext(ctx, &count, `
		SELECT COUNT(*)
		FROM contract_archive_entries
		WHERE did = $1 AND contract_version = $2
	`, *did, 1)
	if err != nil {
		t.Fatalf("Failed to count archive entries: %v", err)
	}

	var entry archiveEntry
	err = db.GetContext(ctx, &entry, `
		SELECT stored_by, archive_status, content_hash, snapshot_cid, contract_snapshot,
		       signature_metadata, credential_hashes, evidence
		FROM contract_archive_entries
		WHERE did = $1 AND contract_version = $2
		LIMIT 1
	`, *did, 1)
	if err != nil {
		t.Fatalf("Failed to read archive entry: %v", err)
	}

	assert.Equal(t, 1, count)
	assert.Equal(t, approver, entry.StoredBy)
	assert.Equal(t, "STORED", entry.ArchiveStatus)
	assert.True(t, strings.HasPrefix(entry.ContentHash, "sha256:"))
	assert.Equal(t, testArchiveSnapshotCID, entry.SnapshotCID)
	assert.True(t, entry.ContractSnapshot.IsNotNullValue())
	assert.True(t, entry.SignatureMetadata.IsNotNullValue())
	assert.True(t, entry.CredentialHashes.IsNotNullValue())
	assert.True(t, entry.Evidence.IsNotNullValue())
	if assert.Len(t, storer.payloads, 1) {
		assert.Equal(t, string(entry.ContractSnapshot), string(storer.payloads[0].(datatype.JSON)))
	}

	var snapshot map[string]any
	err = json.Unmarshal(entry.ContractSnapshot, &snapshot)
	if err != nil {
		t.Fatalf("Failed to decode archive snapshot: %v", err)
	}
	assert.Equal(t, *did, snapshot["did"])
	assert.Equal(t, "APPROVED", snapshot["state"])

	var evidence map[string]any
	err = json.Unmarshal(entry.Evidence, &evidence)
	if err != nil {
		t.Fatalf("Failed to decode archive evidence: %v", err)
	}
	assert.Equal(t, approver, evidence["approved_by"])
	assert.Equal(t, true, evidence["signed_pdf_out_of_scope"])

	var eventCount int
	err = db.GetContext(ctx, &eventCount, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE component = $1
		  AND event_type = $2
		  AND did = $3
		  AND event_data->>'stored_by' = $4
		  AND (event_data->>'contract_version')::int = $5
		  AND event_data->>'content_hash' = $6
		  AND event_data->>'archive_status' = $7
		  AND event_data->>'snapshot_cid' = $8
	`, componenttype.ContractStorageArchive.String(), eventtype.StoreArchived.String(), *did, approver, 1, entry.ContentHash, "STORED", entry.SnapshotCID)
	if err != nil {
		t.Fatalf("Failed to read archive store event: %v", err)
	}

	assert.Equal(t, 1, eventCount)
}

func TestApprove_ArchiveEntryIsAppendOnly(t *testing.T) {
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

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET content_hash = 'sha256:changed'
		WHERE did = $1
	`, *did)
	assert.Error(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET snapshot_cid = 'bafy-changed'
		WHERE did = $1
	`, *did)
	assert.Error(t, err)

	_, err = db.ExecContext(ctx, `
		DELETE FROM contract_archive_entries
		WHERE did = $1
	`, *did)
	assert.Error(t, err)

	var count int
	err = db.GetContext(ctx, &count, `
		SELECT COUNT(*)
		FROM contract_archive_entries
		WHERE did = $1
	`, *did)
	if err != nil {
		t.Fatalf("Failed to count archive entries: %v", err)
	}
	assert.Equal(t, 1, count)
}

func TestApprove_FinalApproveRequiresArchiveSnapshotIPFSStorer(t *testing.T) {
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

	handler := command.Approver{
		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, command.ApproveCmd{
		DID:        *did,
		UpdatedAt:  time.Now().UTC(),
		ApprovedBy: approver,
	})
	assert.Error(t, err)

	var archiveCount int
	err = db.GetContext(ctx, &archiveCount, `
		SELECT COUNT(*)
		FROM contract_archive_entries
		WHERE did = $1
	`, *did)
	if err != nil {
		t.Fatalf("Failed to count archive entries: %v", err)
	}
	assert.Equal(t, 0, archiveCount)
}

func TestApprove_ArchiveEntryValidatesStatusTransitions(t *testing.T) {
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

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET archive_status = 'RETAINED'
		WHERE did = $1
	`, *did)
	assert.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET archive_status = 'STORED'
		WHERE did = $1
	`, *did)
	assert.Error(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET archive_status = 'DELETION_REQUESTED'
		WHERE did = $1
	`, *did)
	assert.Error(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET archive_status = 'DELETION_REQUESTED',
		    deleted_by = $2,
		    deletion_reason = $3
		WHERE did = $1
	`, *did, creator, "retention policy allows removal")
	assert.NoError(t, err)
}

func TestApprove_ArchiveEntryBlocksDeletionBeforeRetentionUntil(t *testing.T) {
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

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET retention_until = NOW() + INTERVAL '1 day'
		WHERE did = $1
	`, *did)
	assert.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entries
		SET archive_status = 'DELETED',
		    deleted_at = NOW(),
		    deleted_by = $2,
		    deletion_reason = $3
		WHERE did = $1
	`, *did, creator, "test deletion")
	assert.Error(t, err)
}

func TestApprove_ArchiveEntryEventsAreAppendOnly(t *testing.T) {
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

	var archiveEntryID string
	err = db.GetContext(ctx, &archiveEntryID, `
		SELECT id::text
		FROM contract_archive_entries
		WHERE did = $1
	`, *did)
	if err != nil {
		t.Fatalf("Failed to read archive entry ID: %v", err)
	}

	const eventHash = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	_, err = db.ExecContext(ctx, `
		INSERT INTO contract_archive_entry_events (
			archive_entry_id, event_type, actor, reason, event_hash
		) VALUES ($1, $2, $3, $4, $5)
	`, archiveEntryID, "RETENTION_REVIEWED", creator, "test event", eventHash)
	assert.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		UPDATE contract_archive_entry_events
		SET reason = 'changed'
		WHERE event_hash = $1
	`, eventHash)
	assert.Error(t, err)

	_, err = db.ExecContext(ctx, `
		DELETE FROM contract_archive_entry_events
		WHERE event_hash = $1
	`, eventHash)
	assert.Error(t, err)
}

func TestApprove_BuildArchiveEntryHashIsDeterministic(t *testing.T) {
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

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `UPDATE contracts SET state = $2 WHERE did = $1`, *did, contractstate.Approved.String())
	if err != nil {
		t.Fatalf("Failed to approve contract directly: %v", err)
	}
	approvedContract, err := repo.CRepo.ReadDataByID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read approved contract: %v", err)
	}

	first, err := command.BuildArchiveEntry(approvedContract, creator)
	if err != nil {
		t.Fatalf("Failed to build first archive entry: %v", err)
	}
	second, err := command.BuildArchiveEntry(approvedContract, creator)
	if err != nil {
		t.Fatalf("Failed to build second archive entry: %v", err)
	}

	assert.Equal(t, first.ContentHash, second.ContentHash)
	assert.Equal(t, string(first.ContractSnapshot), string(second.ContractSnapshot))
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
