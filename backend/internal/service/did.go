package service

import (
	"context"

	didservice "digital-contracting-service/gen/did_service"
	"digital-contracting-service/internal/base"
)

type DIDSrv struct {
	DIDocument base.DIDDocument
}

func NewDIDService(didDocument base.DIDDocument) (didservice.Service, error) {
	return &DIDSrv{
		DIDocument: didDocument,
	}, nil
}

func (s DIDSrv) GetServiceDID(ctx context.Context) (res any, err error) {
	return s.DIDocument, nil
}
