package test

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwecommands "digital-contracting-service/internal/contractworkflowengine/command"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/signingmanagement/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/db/pg"
	"log"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type TestRepo struct {
	CRepo    db.ContractRepo
	CWECRepo cwedb.ContractRepo
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

	t.Cleanup(func() { database.Close() })

	return database
}

func NewTestRepo() *TestRepo {
	return &TestRepo{
		CRepo:    &pg.PostgresContractRepo{},
		CWECRepo: &cwepg.PostgresContractRepo{},
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

	cmd := cwecommands.CreateCmd{
		DID:          *did,
		CreatedBy:    createdBy,
		Name:         &name,
		Description:  &description,
		ContractData: &jsonContractData,
	}
	createHandler := cwecommands.Creator{
		DB:    db,
		CRepo: repo.CWECRepo,
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

func createTestContractWithData(t *testing.T, db *sqlx.DB, repo *TestRepo, did *string, state contractstate.ContractState, createdBy string, name string, description string, contractData map[string]interface{}) {
	jsonContractData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON data: %v", err)
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
		CRepo: repo.CWECRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract: %v", err)
	}

	updateStatement := `UPDATE contracts SET
        	state = $2
    	WHERE did = $1
`

	_, err = db.Exec(updateStatement, *did, state)
	if err != nil {
		t.Fatalf("Failed to update template state: %v", err)
	}
}
