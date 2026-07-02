package service

import (
	"context"

	"digital-contracting-service/internal/base/identity"

	didservice "digital-contracting-service/gen/did_service"
)

type DIDSrv struct {
	DIDocument identity.DIDDocument
}

func NewDIDService(didDocument identity.DIDDocument) (didservice.Service, error) {
	return &DIDSrv{
		DIDocument: didDocument,
	}, nil
}

func (s DIDSrv) GetServiceDID(ctx context.Context) (res any, err error) {
	return s.DIDocument.GetDIDContent(), nil
}
