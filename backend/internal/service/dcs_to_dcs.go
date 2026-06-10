package service

import (
	"context"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"

	"goa.design/clue/log"
)

type dcsToDcssrvc struct {
	auth.JWTAuthenticator
}

func NewDcsToDcs(jwtAuth auth.JWTAuthenticator) dcstodcs.Service {
	return &dcsToDcssrvc{JWTAuthenticator: jwtAuth}
}

func (s *dcsToDcssrvc) Retrieve(ctx context.Context, p *dcstodcs.RetrievePayload) (res any, err error) {
	log.Printf(ctx, "dcsToDcs.retrieve")
	return
}
