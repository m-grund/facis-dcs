package db

import (
	"context"
	"encoding/json"
	"time"
)

type PresentationStatus string

const (
	PresentationPending  PresentationStatus = "pending"
	PresentationComplete PresentationStatus = "complete"
	PresentationFailed   PresentationStatus = "failed"
	PresentationExpired  PresentationStatus = "expired"
)

// PresentationAttempt tracks one OpenID4VP login attempt.
type PresentationAttempt struct {
	PresentationState   string             `db:"presentation_state"`
	Status              PresentationStatus `db:"status"`
	Nonce               string             `db:"nonce"`
	ExpiresAt           time.Time          `db:"expires_at"`
	VerifiedClaims      *json.RawMessage   `db:"verified_claims"`
	HydraLoginChallenge *string            `db:"hydra_login_challenge"`
	RedirectURI         *string            `db:"redirect_uri"`
	ErrorMessage        *string            `db:"error_message"`
	SubjectDID          *string            `db:"subject_did"`
	OrganizationID      *string            `db:"organization_id"`
	Roles               *json.RawMessage   `db:"roles"`
	CreatedAt           time.Time          `db:"created_at"`
	UpdatedAt           time.Time          `db:"updated_at"`
}

type PresentationAttemptRepo interface {
	CreatePending(ctx context.Context, attempt PresentationAttempt) error
	FindByPresentationState(ctx context.Context, presentationState string) (*PresentationAttempt, error)
	SetHydraLoginChallenge(ctx context.Context, presentationState, challenge string) error
	MarkComplete(ctx context.Context, presentationState string, claims json.RawMessage, subjectDID, organizationID string, roles json.RawMessage, redirectURI string) error
	MarkFailed(ctx context.Context, presentationState string, errorMessage string) error
	MarkExpired(ctx context.Context, presentationState string) error
	RenewPending(ctx context.Context, presentationState string, expiresAt time.Time) error
}
