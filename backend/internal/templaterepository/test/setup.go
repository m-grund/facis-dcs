package test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	database "digital-contracting-service/internal/templaterepository/db"
	"digital-contracting-service/internal/templaterepository/db/pg"
)

type TestRepo struct {
	CTRepo database.ContractTemplateRepo
	RTRepo database.ReviewTaskRepo
	ATRepo database.ApprovalTaskRepo
}

func setupTestDB(t *testing.T) *sqlx.DB {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatalf("DATABASE_URL isn't set")
	}

	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		log.Fatalln(err)
	}

	t.Cleanup(func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("could not close database connection")
		}
	})

	return db
}

func NewTestRepo() *TestRepo {
	return &TestRepo{
		CTRepo: &pg.PostgresContractTemplateRepo{},
		RTRepo: &pg.PostgresReviewTaskRepo{},
		ATRepo: &pg.PostgresApprovalTaskRepo{},
	}
}

func cleanupContractTemplateTable(t *testing.T, db *sqlx.DB) {
	cleanApprovalTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_templates_approval_task;
`
	_, err := db.Exec(cleanApprovalTasksStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanReviewTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_templates_review_task;
`
	_, err = db.Exec(cleanReviewTasksStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanTableStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_templates;
`
	_, err = db.Exec(cleanTableStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}
}

func createContractTemplate(t *testing.T, db *sqlx.DB, repo *TestRepo, did *string, state contracttemplatestate.ContractTemplateState, createdBy string) {
	name := "Test Contract Template"
	description := "Test Description"

	templateData := map[string]interface{}{
		"key": "value",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	ctx := context.Background()

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    createdBy,
		TemplateType: contracttemplatetype.FrameContract,
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	createHandler := command.Creator{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract template: %v", err)
	}

	updateStatement := `UPDATE contract_templates SET
        	state = $2
    	WHERE did = $1
`

	_, err = db.Exec(updateStatement, cmd.DID, state)
	if err != nil {
		t.Fatalf("Failed to update template state: %v", err)
	}
}

func createTestContractTemplateWithData(t *testing.T, db *sqlx.DB, repo *TestRepo, did *string, state contracttemplatestate.ContractTemplateState, createdBy string, name string, description string, templateData map[string]interface{}) {
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	ctx := context.Background()

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    createdBy,
		TemplateType: contracttemplatetype.FrameContract,
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	createHandler := command.Creator{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract template: %v", err)
	}

	updateStatement := `UPDATE contract_templates SET
        	state = $2
    	WHERE did = $1
`

	_, err = db.Exec(updateStatement, *did, state)
	if err != nil {
		t.Fatalf("Failed to update template state: %v", err)
	}
}

func createReviewTasks(t *testing.T, ctx context.Context, db *sqlx.DB, repo *TestRepo, did string, state reviewtaskstate.ReviewTaskState, submittedBy string, reviewers []string) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("failed to rollback transaction: %s", err)
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
		err := tx.Rollback()
		if err != nil {
			log.Printf("failed to rollback transaction: %s", err)
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
