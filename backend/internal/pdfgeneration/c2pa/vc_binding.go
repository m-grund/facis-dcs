package c2pa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// VCSigner signs unsigned VCs via the Crypto Provider Service.
type VCSigner interface {
	CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error)
}

// VCBinding is the W3C VC issued to bind a lifecycle event to contract_id + file_hash
// (DCS-OR-C2PA-004). It is returned by IssueLifecycleVC and stored as vc_id in the
// LifecycleAssertion once issued.
type VCBinding struct {
	Context           []string               `json:"@context"`
	Type              []string               `json:"type"`
	ID                string                 `json:"id"`
	Issuer            string                 `json:"issuer"`
	IssuanceDate      time.Time              `json:"issuanceDate"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
}

// IssueLifecycleVC builds and signs a W3C VC recording the lifecycle event.
// The signed VC bytes are returned; the VC id is derived from the SHA-256 of
// its content so it can be stored in LifecycleAssertion.VCId.
func IssueLifecycleVC(ctx context.Context, signer VCSigner, issuerDID string, assertion LifecycleAssertion) (json.RawMessage, string, error) {
	unsignedVC := VCBinding{
		Context: []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		Type:         []string{"VerifiableCredential", "ContractLifecycleCredential"},
		ID:           "", // filled after hash
		Issuer:       issuerDID,
		IssuanceDate: assertion.EffectiveAt,
		CredentialSubject: map[string]interface{}{
			"contract_id": assertion.ContractID,
			"file_hash":   assertion.FileHash,
			"status":      assertion.Status,
			"reason":      assertion.Reason,
			"effective_at": assertion.EffectiveAt.UTC().Format(time.RFC3339),
		},
	}

	raw, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned VC: %w", err)
	}

	h := sha256.Sum256(raw)
	vcID := "urn:dcs:vc:" + hex.EncodeToString(h[:])
	unsignedVC.ID = vcID

	rawWithID, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned VC with ID: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, "", fmt.Errorf("sign lifecycle VC: %w", err)
	}

	return signed, vcID, nil
}
