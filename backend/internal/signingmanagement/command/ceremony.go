package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/poaverify"
)

// ceremonyTTL is how long a started ceremony stays valid for a wallet to
// present the PoA before it must be restarted.
const ceremonyTTL = 15 * time.Minute

// ceremonyAudience is the fixed OID4VP audience/client_id bound into the
// KB-JWT of a PoA presentation for a signing ceremony.
const ceremonyAudience = poaverify.Audience

// WebhookSecret returns the shared secret that authenticates the EUDIPLO
// OID4VP webhook (NFR-SEC-18). It is read from EUDIPLO_WEBHOOK_SECRET.
func WebhookSecret() string {
	if v := strings.TrimSpace(os.Getenv("EUDIPLO_WEBHOOK_SECRET")); v != "" {
		return v
	}
	return "bdd-eudiplo-webhook-secret"
}

// StartCeremonyCmd carries the inputs for starting a signing ceremony.
type StartCeremonyCmd struct {
	ContractDID string
	FieldName   string
	RequestedBy string
	BaseURL     string
}

// StartCeremonyHandler creates a pending signing ceremony (FR-SM-14).
type StartCeremonyHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func (h *StartCeremonyHandler) Handle(ctx context.Context, cmd StartCeremonyCmd) (*db.SignatureCeremony, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer rollback(tx)

	now := time.Now().UTC()
	id := uuid.NewString()
	nonce := uuid.NewString()
	walletURI := fmt.Sprintf("openid4vp://?client_id=%s&request_uri=%s/signature/request/%s&nonce=%s",
		ceremonyAudience, strings.TrimRight(cmd.BaseURL, "/"), id, nonce)
	expiresAt := now.Add(ceremonyTTL)

	ceremony := db.SignatureCeremony{
		ID:          id,
		ContractDID: cmd.ContractDID,
		FieldName:   cmd.FieldName,
		RequestedBy: cmd.RequestedBy,
		Status:      db.CeremonyPending,
		WalletURI:   &walletURI,
		Nonce:       nonce,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}
	if err := h.CeremonyRepo.CreateCeremony(ctx, tx, ceremony); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit start ceremony: %w", err)
	}
	return &ceremony, nil
}

// ErrWebhookUnauthorized is returned when the webhook shared secret is missing
// or incorrect (NFR-SEC-18).
var ErrWebhookUnauthorized = errors.New("incorrect webhook shared secret")

// ErrCeremonyNotFound is returned when a webhook references an unknown ceremony.
var ErrCeremonyNotFound = errors.New("ceremony not found")

// WebhookCmd carries a completed PoA presentation from EUDIPLO.
type WebhookCmd struct {
	Secret     string
	CeremonyID string
	VpToken    string
	PoaClaims  any
}

// WebhookHandler validates a PoA presentation and marks the ceremony verified.
type WebhookHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func (h *WebhookHandler) Handle(ctx context.Context, cmd WebhookCmd) (*db.SignatureCeremony, error) {
	if strings.TrimSpace(cmd.Secret) == "" || cmd.Secret != WebhookSecret() {
		return nil, ErrWebhookUnauthorized
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer rollback(tx)

	ceremony, err := h.CeremonyRepo.GetCeremonyByID(ctx, tx, cmd.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, ErrCeremonyNotFound
	}

	signerDID, sdHash, err := poaverify.Verify(cmd.VpToken)
	if err != nil {
		return nil, fmt.Errorf("PoA presentation verification failed: %w", err)
	}

	var poaBytes []byte
	if cmd.PoaClaims != nil {
		if b, mErr := json.Marshal(cmd.PoaClaims); mErr == nil {
			poaBytes = b
		}
	}

	if err := h.CeremonyRepo.MarkCeremonyVerified(ctx, tx, cmd.CeremonyID, signerDID, cmd.VpToken, poaBytes, sdHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit webhook: %w", err)
	}

	ceremony.Status = db.CeremonyVerified
	ceremony.SignerDID = &signerDID
	return ceremony, nil
}

func rollback(tx *sqlx.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Printf("could not rollback transaction: %v", err)
	}
}
