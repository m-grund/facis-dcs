package service

import (
	"context"

	"digital-contracting-service/internal/base/identity"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/auth"

	"goa.design/clue/log"
)

// ContractStorageArchive service implementation.
type contractStorageArchivesrvc struct {
	DIDDocument identity.DIDDocument
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(jwtAuth auth.JWTAuthenticator, didDocument identity.DIDDocument) contractstoragearchive.Service {
	return &contractStorageArchivesrvc{JWTAuthenticator: jwtAuth, DIDDocument: didDocument}
}

func (s *contractStorageArchivesrvc) Retrieve(ctx context.Context, p *contractstoragearchive.RetrievePayload) (res any, err error) {
	log.Printf(ctx, "contractStorageArchive.retrieve")
	return
}

func (s *contractStorageArchivesrvc) Search(ctx context.Context, p *contractstoragearchive.SearchPayload) (res []any, err error) {
	log.Printf(ctx, "contractStorageArchive.search")
	return
}

func (s *contractStorageArchivesrvc) Store(ctx context.Context, p *contractstoragearchive.StorePayload) (res string, err error) {
	log.Printf(ctx, "contractStorageArchive.store")
	return
}

func (s *contractStorageArchivesrvc) Terminate(ctx context.Context, p *contractstoragearchive.TerminatePayload) (res int, err error) {
	log.Printf(ctx, "contractStorageArchive.terminate")
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
