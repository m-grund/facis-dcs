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
	"digital-contracting-service/internal/base/validation"
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
	cleanArchiveEntriesStatement := `
	TRUNCATE contract_archive_entry_events;
	TRUNCATE contract_archive_entries;
`
	_, err := db.Exec(cleanArchiveEntriesStatement)
	if err != nil {
		t.Fatalf("Failed to clean table: %v", err)
	}

	cleanApprovalTasksStatement := `
	-- noinspection SqlWithoutWhere
	DELETE FROM contract_approval_task;
`
	_, err = db.Exec(cleanApprovalTasksStatement)
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

	jsonContractData, err := datatype.NewJSON(validTestContractData())
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

	if state == contractstate.Approved {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		approvedContract, err := repo.CRepo.ReadDataByID(ctx, tx, cmd.DID)
		if err != nil {
			t.Fatalf("Failed to read approved contract: %v", err)
		}
		archiveEntry, err := command.BuildArchiveEntry(approvedContract, createdBy)
		if err != nil {
			t.Fatalf("Failed to build archive entry: %v", err)
		}
		archiveEntry.SnapshotCID = "bafy-test-contract-snapshot"
		err = repo.CRepo.StoreArchiveEntry(ctx, tx, archiveEntry)
		if err != nil {
			t.Fatalf("Failed to store archive entry: %v", err)
		}

		err = tx.Commit()
		if err != nil {
			t.Fatalf("Failed to commit archive entry: %v", err)
		}
	}
}

func validTestContractData() map[string]any {
	return map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-main"}},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId":      "clause-main",
				"type":         "CLAUSE",
				"text":         "Provider {{provider.legalName}} from {{provider.country}}. Customer {{customer.legalName}} from {{customer.country}}. Payment {{payment.amount}} {{payment.currency}} due {{payment.dueDate}}. Availability {{sla.availability}}.",
				"conditionIds": []any{"provider", "customer", "payment", "sla"},
			},
		},
		"semanticConditions": []any{
			partyTestCondition("provider", "Provider"),
			partyTestCondition("customer", "Customer"),
			map[string]any{
				"conditionId":   "payment",
				"conditionName": "Payment",
				"schemaVersion": "v1",
				"parameters": []any{
					testSemanticParam("amount", "decimal", validation.SchemaContractV1, "contract.payment.amount"),
					testSemanticParam("currency", "string", validation.SchemaContractV1, "contract.payment.currency"),
					testSemanticParam("dueDate", "date", validation.SchemaContractV1, "contract.payment.dueDate"),
				},
			},
			map[string]any{
				"conditionId":   "sla",
				"conditionName": "SLA Availability",
				"schemaVersion": "v1",
				"parameters": []any{
					testSemanticParam("availability", "decimal", validation.SchemaServiceV1, "service.sla.availability"),
				},
			},
		},
		"semanticConditionValues": []any{
			testSemanticValue("clause-main", "provider", "legalName", "Provider GmbH"),
			testSemanticValue("clause-main", "provider", "country", "DEU"),
			testSemanticValue("clause-main", "customer", "legalName", "Customer GmbH"),
			testSemanticValue("clause-main", "customer", "country", "DEU"),
			testSemanticValue("clause-main", "payment", "amount", 1000.0),
			testSemanticValue("clause-main", "payment", "currency", "EUR"),
			testSemanticValue("clause-main", "payment", "dueDate", "2026-06-19"),
			testSemanticValue("clause-main", "sla", "availability", 99.9),
		},
		"customMetaData": []any{},
	}
}

func partyTestCondition(id string, name string) map[string]any {
	return map[string]any{
		"conditionId":   id,
		"conditionName": name,
		"schemaVersion": "v1",
		"entityType":    "CompanyParty",
		"entityRole":    id,
		"parameters": []any{
			testSemanticParam("legalName", "string", validation.SchemaPartyV1, "company.legalName"),
			testSemanticParam("country", "string", validation.SchemaPartyV1, "company.location.country"),
		},
	}
}

func testSemanticParam(name string, paramType string, schemaRef string, semanticPath string) map[string]any {
	return map[string]any{
		"parameterName": name,
		"type":          paramType,
		"schemaRef":     schemaRef,
		"semanticPath":  semanticPath,
		"isRequired":    true,
		"operators":     []any{},
	}
}

func testSemanticValue(blockID string, conditionID string, parameterName string, value any) map[string]any {
	return map[string]any{
		"blockId":        blockID,
		"conditionId":    conditionID,
		"parameterName":  parameterName,
		"parameterValue": value,
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
