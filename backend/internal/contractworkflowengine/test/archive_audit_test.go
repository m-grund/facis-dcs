package test

import (
	"context"
	"testing"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	basepq "digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/service"

	"github.com/stretchr/testify/assert"
)

func TestPACAudit_ArchiveScopeReturnsArchiveSummaries(t *testing.T) {
	db := setupTestDB(t)
	cleanupContractTable(t, db)

	_, err := db.Exec(`
		DELETE FROM audit_trail_log
		WHERE component = $1
	`, componenttype.ContractStorageArchive.String())
	if err != nil {
		t.Fatalf("Failed to clean archive audit trail log: %v", err)
	}

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"
	repo := NewTestRepo()
	createContract(t, db, repo, did, contractstate.Approved, creator)

	svc := service.NewProcessAuditAndCompliance(
		db,
		auth.JWTAuthenticator{},
		base.AuditTrailReader{ARepo: &basepq.PostgresAuditTrailRepository{}},
		nil,
		repo.CRepo,
	)

	result, err := svc.Audit(context.Background(), &processauditandcompliance.PACAuditRequest{
		Scope: "archive",
	})
	if err != nil {
		t.Fatalf("Failed to audit archive scope: %v", err)
	}

	if assert.Len(t, result, 1) {
		assert.Equal(t, componenttype.ContractStorageArchive.String(), result[0].Component)
		assert.Equal(t, *did, result[0].Did)
		if assert.Len(t, result[0].AuditTrail, 1) {
			entry := result[0].AuditTrail[0]
			assert.Equal(t, "ARCHIVE_ENTRY_AUDIT_SUMMARY", entry.EventType)
			data, ok := entry.EventData.(map[string]any)
			if assert.True(t, ok) {
				assert.Equal(t, *did, data["objectDid"])
				assert.Equal(t, "STORED", data["archiveStatus"])
				assert.Equal(t, true, data["snapshotPresent"])
				assert.Equal(t, "NOT_PERFORMED", data["signatureStatus"])
				assert.Equal(t, "PENDING", data["credentialHashStatus"])
				assert.NotEmpty(t, data["contentHash"])
			}
		}
	}
}
