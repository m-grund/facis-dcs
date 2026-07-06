package service

import (
	"context"
	"time"

	"digital-contracting-service/internal/base/identity"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/middleware"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

// ContractStorageArchive service implementation.
type contractStorageArchivesrvc struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	DIDDocument identity.DIDDocument
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, didDocument identity.DIDDocument) contractstoragearchive.Service {
	return &contractStorageArchivesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		DIDDocument:      didDocument,
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

func (s *contractStorageArchivesrvc) Delete(ctx context.Context, p *contractstoragearchive.DeletePayload) (res int, err error) {
	log.Printf(ctx, "contractStorageArchive.delete")
	return
}

func (s *contractStorageArchivesrvc) Audit(ctx context.Context, p *contractstoragearchive.AuditPayload) (res []string, err error) {
	log.Printf(ctx, "contractStorageArchive.audit")
	return
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
	}
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
