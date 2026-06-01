package test

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"testing"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

func TestVerify_VerifyContractTemplateAsReviewer(t *testing.T) {

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

	reviewers := []string{"Test User 1"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	cmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: reviewers[0],
	}
	handler := command.Verifier{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to verify contract template: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	exists, err := repo.RTRepo.AnyTasksInState(ctx, tx, *did, reviewtaskstate.Verified.String())
	if err != nil {
		t.Fatalf("Failed to check existence of review tasks: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatal("could not commit transaction: %w", err)
	}

	assert.True(t, exists)
}

func TestVerify_VerifyNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: "Test User 1",
	}
	handler := command.Verifier{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
