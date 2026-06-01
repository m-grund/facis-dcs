package service

import (
	"context"

	orchestrationwebhooks "digital-contracting-service/gen/orchestration_webhooks"
	"digital-contracting-service/internal/auth"

	"goa.design/clue/log"
)

type orchestrationWebhookssrvc struct {
	auth.JWTAuthenticator
}

func NewOrchestrationWebhooks(jwtAuth auth.JWTAuthenticator) orchestrationwebhooks.Service {
	return &orchestrationWebhookssrvc{JWTAuthenticator: jwtAuth}
}

func (s *orchestrationWebhookssrvc) NodeRedWebhook(ctx context.Context, p *orchestrationwebhooks.NodeRedWebhookPayload) (res any, err error) {
	log.Printf(ctx, "orchestrationWebhooks.node_red_webhook")
	return
}
