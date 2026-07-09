package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// SigningSummary carries the fields bound into a ContractSigningSummaryCredential
// (DCS-FR-SM-08). It records who signed, through which ceremony, and the hashes
// the PAdES signature covers, plus the verbatim PID presentation the signer's
// wallet produced so a verifier can re-verify it from the PDF alone.
type SigningSummary struct {
	ContractID      string
	SignerDID       string
	CeremonyID      string
	FieldName       string
	ContentHash     string // SHA-256 hex of the JSON-LD contract source
	PDFHash         string // SHA-256 hex of the base PDF bytes bound by the signature
	CredentialType  string // e.g. "AES"
	KBSDHash        string // KB-JWT sd_hash of the presented PID
	PIDPresentation string // verbatim SD-JWT VC + KB-JWT compact presentation
	SignedAt        time.Time
}

// IssueSigningSummaryVC builds and signs a ContractSigningSummaryCredential over
// the given summary, using the same VC signer as the lifecycle credentials. The
// PID presentation is embedded verbatim as a credentialSubject field so that it
// survives into the signed PDF unchanged (DCS-FR-SM-08, UC-04-03).
func IssueSigningSummaryVC(ctx context.Context, signer VCSigner, issuerDID string, s SigningSummary) (json.RawMessage, string, error) {
	unsignedVC := VCBinding{
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			dataIntegrityContext,
			map[string]interface{}{
				"dcs":                             "https://w3id.org/facis/dcs#",
				"ContractSigningSummaryCredential": "dcs:ContractSigningSummaryCredential",
				"contract_id":                      "dcs:contractId",
				"ceremony_id":                      "dcs:ceremonyId",
				"field_name":                       "dcs:fieldName",
				"content_hash":                     "dcs:contentHash",
				"pdf_hash":                         "dcs:pdfHash",
				"credential_type":                  "dcs:credentialType",
				"kb_sd_hash":                       "dcs:kbSdHash",
				"pid_presentation":                 "dcs:pidPresentation",
				"signed_at": map[string]interface{}{
					"@id":   "dcs:signedAt",
					"@type": "http://www.w3.org/2001/XMLSchema#dateTime",
				},
			},
		},
		Type:      []string{"VerifiableCredential", "ContractSigningSummaryCredential"},
		Issuer:    issuerDID,
		ValidFrom: s.SignedAt.UTC().Format(time.RFC3339),
		CredentialSubject: map[string]interface{}{
			"id":               normalizeSubjectID(s.SignerDID),
			"contract_id":      s.ContractID,
			"ceremony_id":      s.CeremonyID,
			"field_name":       s.FieldName,
			"content_hash":     s.ContentHash,
			"pdf_hash":         s.PDFHash,
			"credential_type":  s.CredentialType,
			"kb_sd_hash":       s.KBSDHash,
			"pid_presentation": s.PIDPresentation,
			"signed_at":        s.SignedAt.UTC().Format(time.RFC3339),
		},
	}

	raw, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned signing-summary VC: %w", err)
	}
	h := sha256.Sum256(raw)
	vcID := "urn:dcs:vc:" + hex.EncodeToString(h[:])
	unsignedVC.ID = vcID

	rawWithID, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned signing-summary VC with ID: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, "", fmt.Errorf("sign signing-summary VC: %w", err)
	}
	return signed, vcID, nil
}
