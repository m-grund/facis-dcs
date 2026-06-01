package service

import (
	"context"

	externaltargetsystemapi "digital-contracting-service/gen/external_target_system_api"
	"digital-contracting-service/internal/auth"

	"goa.design/clue/log"
)

type externalTargetSystemAPIsrvc struct {
	auth.JWTAuthenticator
}

func NewExternalTargetSystemAPI(jwtAuth auth.JWTAuthenticator) externaltargetsystemapi.Service {
	return &externalTargetSystemAPIsrvc{JWTAuthenticator: jwtAuth}
}

func (s *externalTargetSystemAPIsrvc) Action(ctx context.Context, p *externaltargetsystemapi.ActionPayload) (res any, err error) {
	log.Printf(ctx, "externalTargetSystemAPI.action")
	return
}

func (s *externalTargetSystemAPIsrvc) Status(ctx context.Context, p *externaltargetsystemapi.StatusPayload) (res any, err error) {
	log.Printf(ctx, "externalTargetSystemAPI.status")
	return
}

func (s *externalTargetSystemAPIsrvc) Callback(ctx context.Context, p *externaltargetsystemapi.CallbackPayload) (res any, err error) {
	log.Printf(ctx, "externalTargetSystemAPI.callback")
	return
}
