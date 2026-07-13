package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/identity"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/middleware"
	pacquery "digital-contracting-service/internal/processauditandcompliance/query"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

// ContractStorageArchive service implementation.
type contractStorageArchivesrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	DIDDocument  identity.DIDDocument
	ATrailReader base.AuditTrailReader
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, didDocument identity.DIDDocument, auditTrailReader base.AuditTrailReader) contractstoragearchive.Service {
	return &contractStorageArchivesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
	}
}

func (s *contractStorageArchivesrvc) Retrieve(ctx context.Context, p *contractstoragearchive.ArchiveRetrieveRequest) (res *contractstoragearchive.ArchiveRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetArchivedContractsQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
	}

	queryHandler := contract.GetArchivedContractsHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractstoragearchive.ContractItem
	for _, item := range result.Contracts {

		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := *item.ExpPolicy
			expPolicy = &s
		}

		contracts = append(contracts, &contractstoragearchive.ContractItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State,
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
			Evidence:        archiveEvidenceValue(item.Evidence),
		})
	}

	return &contractstoragearchive.ArchiveRetrieveResponse{
		Contracts: contracts,
	}, nil
}

func (s *contractStorageArchivesrvc) Search(ctx context.Context, p *contractstoragearchive.ArchiveSearchRequest) (res []*contractstoragearchive.ContractItem, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contractstate.ContractState
	if p.State != nil {
		tState, err := contractstate.NewContractState(*p.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		state = &tState
	}

	qry := contract.SearchArchivedContractsQry{
		DID:             stringValue(p.Did),
		ContractVersion: intValue(p.ContractVersion),
		State:           state,
		RetrievedBy:     middleware.GetParticipantID(ctx),
		Name:            stringValue(p.Name),
		Description:     stringValue(p.Description),
		ContractData:    stringValue(p.ContractData),
	}
	queryHandler := contract.GetArchivedContractsHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Search(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractstoragearchive.ContractItem
	for _, item := range result.Contracts {
		contracts = append(contracts, toArchiveContractItem(item))
	}

	return contracts, nil
}

func (s *contractStorageArchivesrvc) Store(ctx context.Context, p *contractstoragearchive.StorePayload) (res string, err error) {
	log.Printf(ctx, "contractStorageArchive.store")
	return
}

// Delete soft-deletes every not-yet-deleted archive entry for the given DID
// (DCS-FR-CSA-17): the row is marked deleted_at/deleted_by/deletion_reason,
// never physically removed, and the operation is itself logged as an audit
// event under the ContractStorageArchive component so it shows up through
// Audit below.
func (s *contractStorageArchivesrvc) Delete(ctx context.Context, p *contractstoragearchive.DeletePayload) (res int, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	deletedBy := middleware.GetParticipantID(ctx)
	affected, err := s.CRepo.MarkArchiveEntryDeleted(ctx, tx, p.Did, deletedBy, p.Justification)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	if affected == 0 {
		return 0, contractstoragearchive.MakeBadRequest(
			fmt.Errorf("no archive entry found for DID %q (or it was already deleted)", p.Did))
	}

	evt := contractevents.DeleteArchivedEvent{
		DID:           p.Did,
		DeletedBy:     deletedBy,
		Justification: p.Justification,
		EntriesMarked: affected,
		OccurredAt:    time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	if err := tx.Commit(); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	return affected, nil
}

// Audit returns the archive component's audit log (DCS-IR-CSA-04,
// UC-07-03): every event recorded under componenttype.ContractStorageArchive
// (store/retrieve/search/delete), across every DID's chain plus the
// DID-less "*" chain used by component-wide operations (retrieve/search) —
// reusing the same cross-component reader process_audit_and_compliance's
// own audit method is built on (qry.Auditor / ReadAuditLogEntriesByComponent).
func (s *contractStorageArchivesrvc) Audit(ctx context.Context, p *contractstoragearchive.AuditPayload) (res []*contractstoragearchive.ContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := pacquery.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	chains, err := handler.Handle(ctx, pacquery.GetAuditLogQry{
		Scope:     componenttype.ContractStorageArchive,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}

	history := make([]*contractstoragearchive.ContractAuditResponse, 0)
	for _, chain := range chains {
		for _, entry := range chain {
			if !base.IsAuditVisibleEventType(entry.EventType) {
				continue
			}
			history = append(history, &contractstoragearchive.ContractAuditResponse{
				ID:               entry.ID,
				Component:        entry.Component,
				EventType:        entry.EventType,
				EventData:        entry.EventData,
				Did:              entry.DID,
				CreatedAt:        entry.CreatedAt.String(),
				GlobalLogPredCid: entry.GlobalLogPredCID,
				ResLogPredCid:    entry.ResLogPredCID,
			})
		}
	}

	return history, nil
}

func toArchiveContractItem(item db.ContractMetadata) *contractstoragearchive.ContractItem {
	var startDate *string
	if item.StartDate != nil {
		s := item.StartDate.Format(time.RFC3339)
		startDate = &s
	}

	var expDate *string
	if item.ExpDate != nil {
		s := item.ExpDate.Format(time.RFC3339)
		expDate = &s
	}

	var expPolicy *string
	if item.ExpPolicy != nil {
		s := *item.ExpPolicy
		expPolicy = &s
	}

	return &contractstoragearchive.ContractItem{
		Did:             item.DID,
		ContractVersion: item.ContractVersion,
		State:           item.State,
		Name:            item.Name,
		Description:     item.Description,
		CreatedBy:       item.CreatedBy,
		CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: item.ExpNoticePeriod,
		Responsible:     item.Responsible,
		Evidence:        archiveEvidenceValue(item.Evidence),
	}
}

// archiveEvidenceValue decodes a ContractMetadata.Evidence blob (populated
// only for archived-contract queries, joined from
// contract_archive_entries.evidence) into a plain any for the API response.
func archiveEvidenceValue(evidence *datatype.JSON) any {
	if evidence == nil || !evidence.IsNotNullValue() {
		return nil
	}
	var value any
	if err := json.Unmarshal(*evidence, &value); err != nil {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
