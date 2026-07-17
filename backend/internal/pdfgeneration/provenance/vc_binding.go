package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// VCSigner signs unsigned VCs via the Crypto Provider Service.
type VCSigner interface {
	CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error)
}

// VCBinding is the W3C VC issued to bind a lifecycle event to contract_id + file_hash
// (DCS-OR-C2PA-004). It is returned by IssueLifecycleVC and stored as vc_id in the
// LifecycleAssertion once issued. Uses W3C VC Data Model 2.0 field names.
type VCBinding struct {
	Context           []interface{}          `json:"@context"`
	Type              []string               `json:"type"`
	ID                string                 `json:"id"`
	Issuer            string                 `json:"issuer"`
	ValidFrom         string                 `json:"validFrom"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
	// CredentialStatus links this VC to the XFSC status list entry so
	// verifiers can check revocation (DCS-OR-C2PA-004, DCS-OR-C2PA-005).
	CredentialStatus map[string]interface{} `json:"credentialStatus,omitempty"`
}

// IssueLifecycleVC builds and signs a W3C VC recording the lifecycle event.
// statusListURI is the URL of the XFSC status list entry for this contract;
// it is embedded as credentialStatus.id so verifiers can check revocation
// (DCS-OR-C2PA-004, DCS-OR-C2PA-005).
// The signed VC bytes are returned; the VC id is derived from the SHA-256 of
// its content so it can be stored in LifecycleAssertion.VCId.
func IssueLifecycleVC(ctx context.Context, signer VCSigner, issuerDID, statusListURI string, assertion LifecycleAssertion) (json.RawMessage, string, error) {
	subjectID := normalizeSubjectID(assertion.ContractID)
	securityCtx := vcSecuritySuiteContext()

	unsignedVC := VCBinding{
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			securityCtx,
			map[string]interface{}{
				"dcs":                         "https://w3id.org/facis/dcs/ontology/v1#",
				"ContractLifecycleCredential": "dcs:ContractLifecycleCredential",
				"contract_id":                 "dcs:contractId",
				"file_hash":                   "dcs:fileHash",
				"status":                      "dcs:status",
				"reason":                      "dcs:reason",
				"effective_at": map[string]interface{}{
					"@id":   "dcs:effectiveAt",
					"@type": "http://www.w3.org/2001/XMLSchema#dateTime",
				},
			},
		},
		Type:      []string{"VerifiableCredential", "ContractLifecycleCredential"},
		ID:        "", // filled after hash
		Issuer:    issuerDID,
		ValidFrom: assertion.EffectiveAt.UTC().Format(time.RFC3339),
		CredentialSubject: map[string]interface{}{
			"id":           subjectID,
			"contract_id":  assertion.ContractID,
			"file_hash":    assertion.FileHash,
			"status":       assertion.Status,
			"reason":       assertion.Reason,
			"effective_at": assertion.EffectiveAt.UTC().Format(time.RFC3339),
		},
		CredentialStatus: buildCredentialStatus(statusListURI, assertion.ContractID),
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

// buildCredentialStatus constructs the W3C StatusList2021 credentialStatus
// object that links this VC to the XFSC bitstring status list (DCS-OR-C2PA-005).
// Returns nil when statusListURI is empty so the field is omitted from the VC.
func buildCredentialStatus(statusListURI, contractID string) map[string]interface{} {
	if statusListURI == "" {
		return nil
	}
	return map[string]interface{}{
		"id":                   fmt.Sprintf("%s#%d", statusListURI, StatusListIndex(contractID)),
		"type":                 "BitstringStatusListEntry",
		"statusPurpose":        "revocation",
		"statusListIndex":      fmt.Sprintf("%d", StatusListIndex(contractID)),
		"statusListCredential": statusListURI,
	}
}

func vcSecuritySuiteContext() string {
	return dataIntegrityContext
}

// normalizeSubjectID returns a URI to satisfy strict VC signer validation.
// If the input is already an absolute URI (including did:... and urn:...), it
// is used as-is. Otherwise, a deterministic URN is generated from the raw value.
func normalizeSubjectID(raw string) string {
	s := strings.TrimSpace(raw)
	if s != "" {
		u, err := url.Parse(s)
		if err == nil && u.IsAbs() && u.Scheme != "" {
			return s
		}
	}
	h := sha256.Sum256([]byte(s))
	return "urn:dcs:subject:" + hex.EncodeToString(h[:])
}
