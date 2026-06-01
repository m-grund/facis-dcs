package test

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	database "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
)

type TestRepo struct {
	CRepo  database.ContractRepo
	RTRepo database.ReviewTaskRepo
	ATRepo database.ApprovalTaskRepo
	NTRepo database.NegotiationTaskRepo
	NRepo  database.NegotiationRepo
}

func setupTestDB(t *testing.T) *sqlx.DB {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatalf("DATABASE_URL isn't set")
	}

	database, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		log.Fatalln(err)
	}

	t.Cleanup(func() {
		err := database.Close()
		if err != nil {
			t.Errorf("could not close database connection")
		}
	})

	return database
}

func NewTestRepo() *TestRepo {
	return &TestRepo{
		CRepo:  &pg.PostgresContractRepo{},
		RTRepo: &pg.PostgresReviewTaskRepo{},
		ATRepo: &pg.PostgresApprovalTaskRepo{},
		NTRepo: &pg.PostgresNegotiationTaskRepo{},
		NRepo:  &pg.PostgresNegotiationRepo{},
	}
}

func cleanupContractTable(t *testing.T, db *sqlx.DB) {
	cleanApprovalTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_approval_task;
`
	_, err := db.Exec(cleanApprovalTasksStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanReviewTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_review_task;
`
	_, err = db.Exec(cleanReviewTasksStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanNegotiationTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_negotiation_task;
`
	_, err = db.Exec(cleanNegotiationTasksStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanNegotiationsStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_negotiations;
`
	_, err = db.Exec(cleanNegotiationsStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanTableStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contracts;
`
	_, err = db.Exec(cleanTableStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}
}

func createContract(t *testing.T, db *sqlx.DB, repo *TestRepo, did *string, state contractstate.ContractState, createdBy string) {
	name := "Test Contract"
	description := "Test Description"

	contractData := map[string]interface{}{
		"key": "value",
	}
	jsonContractData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON contract data: %v", err)
	}

	ctx := context.Background()

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    createdBy,
		Name:         &name,
		Description:  &description,
		ContractData: &jsonContractData,
	}
	createHandler := command.Creator{
		DB:    db,
		CRepo: repo.CRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract: %v", err)
	}

	updateStatement := `UPDATE contracts SET
        	state = $2
    	WHERE did = $1
`

	_, err = db.Exec(updateStatement, cmd.DID, state)
	if err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}
}

func createNegotiationTasks(t *testing.T, ctx context.Context, db *sqlx.DB, repo *TestRepo, did string, state negotiationtaskstate.NegotiationTaskState, submittedBy string, negotiators []string) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	for _, negotiator := range negotiators {
		negotiationTask := database.NegotiationTaskData{
			DID:        did,
			Negotiator: negotiator,
			State:      state.String(),
			CreatedBy:  submittedBy,
		}
		_, err = repo.NTRepo.Create(ctx, tx, negotiationTask)
		if err != nil {
			t.Fatalf("Failed to create negotiation task: %v", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

func createReviewTasks(t *testing.T, ctx context.Context, db *sqlx.DB, repo *TestRepo, did string, state reviewtaskstate.ReviewTaskState, submittedBy string, reviewers []string) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	for _, reviewer := range reviewers {
		reviewTask := database.ReviewTaskData{
			DID:       did,
			Reviewer:  reviewer,
			State:     state.String(),
			CreatedBy: submittedBy,
		}
		_, err = repo.RTRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			t.Fatalf("Failed to create review task: %v", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

func createApprovalTasks(t *testing.T, ctx context.Context, db *sqlx.DB, repo *TestRepo, did string, state approvaltaskstate.ApprovalTaskState, submittedBy string, approver string) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	approvalTask := database.ApprovalTaskData{
		DID:       did,
		Approver:  approver,
		State:     state.String(),
		CreatedBy: submittedBy,
	}
	_, err = repo.ATRepo.Create(ctx, tx, approvalTask)
	if err != nil {
		t.Fatalf("Failed to create review task: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}
