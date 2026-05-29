package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRegister_RegisterContractTemplateDataInValidState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry := contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Registered, contractTemplate.State)
}

func TestRegister_RegisterNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.RegisterCmd{
		DID:          *did,
		UpdatedAt:    time.Now().UTC(),
		RegisteredBy: "Test User 1",
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInDraftState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		CTRepo: repo.CTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInSubmittedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInRejectedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Rejected, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInReviewedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInRegisteredState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestRegister_RegisterContractTemplateDataInArchivedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)

	cmd := command.RegisterCmd{
		DID:          *did,
		RegisteredBy: creator,
		UpdatedAt:    time.Now().UTC(),
	}
	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
