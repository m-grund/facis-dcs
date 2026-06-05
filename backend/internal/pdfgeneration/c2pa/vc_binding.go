package c2pa

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

// VCBinding is the W3C VC (Data Model 2.0) issued to bind a lifecycle event to
// contract_id + file_hash (DCS-OR-C2PA-004).
// Fields follow W3C VC Data Model 2.0: validFrom/validUntil replace issuanceDate.
type VCBinding struct {
	Context           []interface{}          `json:"@context"`
	Type              []string               `json:"type"`
	ID                string                 `json:"id"`
	Issuer            string                 `json:"issuer"`
	ValidFrom         time.Time              `json:"validFrom"`
	ValidUntil        *time.Time             `json:"validUntil,omitempty"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
	// CredentialStatus links this VC to the XFSC status list entry so
	// verifiers can check revocation (DCS-OR-C2PA-004, DCS-OR-C2PA-005).
	CredentialStatus map[string]interface{} `json:"credentialStatus,omitempty"`
}

// IssueLifecycleVC builds and signs a W3C VC (Data Model 2.0) recording the
// lifecycle event. statusListURI is the URL of the XFSC status list entry for
// this contract; it is embedded as credentialStatus.id so verifiers can check
// revocation (DCS-OR-C2PA-004, DCS-OR-C2PA-005).
func IssueLifecycleVC(ctx context.Context, signer VCSigner, issuerDID, statusListURI string, assertion LifecycleAssertion) (json.RawMessage, string, error) {
	subjectID := normalizeSubjectID(assertion.ContractID)

	unsignedVC := VCBinding{
		// W3C VC Data Model 2.0: first context element MUST be
		// https://www.w3.org/ns/credentials/v2.
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			vcSecuritySuiteContext(),
			map[string]interface{}{
				"dcs":                         "https://w3id.org/facis/dcs#",
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
		ValidFrom: assertion.EffectiveAt.UTC(),
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

// buildCredentialStatus constructs the W3C BitstringStatusListEntry
// credentialStatus object that links this VC to the XFSC bitstring status list
// (DCS-OR-C2PA-005, W3C VC Status List 2021/BitstringStatusList).
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

// vcSecuritySuiteContext returns the JSON-LD context URL for the Ed25519Signature2020
// proof suite used by the XFSC Crypto Provider Service signer.
// The proof type is determined by the external CRYPTO_PROVIDER_VC_SIGNATURE_TYPE
// env var (ed25519signature2020), not by this service.
func vcSecuritySuiteContext() string {
	return "https://w3id.org/security/suites/ed25519-2020/v1"
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
