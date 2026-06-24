package service

import (
	"context"
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/base/event"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/auth"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"goa.design/clue/log"
)

// ContractStorageArchive service implementation.
type contractStorageArchivesrvc struct {
	CESubClient *event.CloudEventSubClient
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(ctx context.Context, jwtAuth auth.JWTAuthenticator, ceSubClient *event.CloudEventSubClient) contractstoragearchive.Service {
	csa := &contractStorageArchivesrvc{JWTAuthenticator: jwtAuth, CESubClient: ceSubClient}

	csa.startEventHandler(ctx)

	return csa
}

func (s *contractStorageArchivesrvc) startEventHandler(ctx context.Context) {
	eventHandler := func(evt cloudevent.Event) {

		data, err := json.Marshal(evt)
		if err != nil {
			log.Printf(ctx, "Could not marshal event to JSON: %v", err)
		}

		fmt.Printf("received event: %s\n", string(data))
	}
	go func() {
		if err := s.CESubClient.Subscribe(eventHandler); err != nil {
			log.Errorf(ctx, err, "could not start event printer")
		}
	}()
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
