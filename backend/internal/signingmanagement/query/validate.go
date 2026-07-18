package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
	"digital-contracting-service/internal/signingmanagement/poaverify"
)

type ValidateQry struct {
	DID         string
	ValidatedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type ValidationResult struct {
	Findings []string
}

type Validator struct {
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
}

func (h *Validator) Handle(ctx context.Context, cmd ValidateQry) (*ValidationResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	findings, err := h.CRepo.CollectValidationFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect validation findings: %w", err)
	}

	findings = append(findings, h.crossCheckEmbeddedPoA(ctx, tx, cmd.DID)...)
	findings = append(findings, h.crossCheckSHACLDrift(ctx, tx, cmd.DID)...)

	dssFindings, err := h.validateWithDSS(ctx, tx, cmd.DID)
	if err != nil {
		// A CONFIGURED DSS is a required validator: its unavailability is an
		// error the caller sees, never a silently thinner findings list.
		return nil, err
	}
	findings = append(findings, dssFindings...)

	evt := signingmanagementevents.ValidateEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ValidatedBy:     cmd.ValidatedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &ValidationResult{
		Findings: findings,
	}, nil
}

// validateWithDSS submits the stored signed PDF to the configured EU DSS
// instance (DCS-FR-SM-18, DCS-IR-SI-10, DCS-IR-CI-08) and reports its ETSI
// EN 319 102-1 indication as a finding. No DSS_URL means no DSS leg (the
// internal PKCS#11-based checks stand alone); a configured-but-failing DSS
// is an error. An unsigned contract (no stored PDF) yields no DSS finding.
func (h *Validator) validateWithDSS(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	dssURL := dss.URL()
	if dssURL == "" {
		return nil, nil
	}
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return nil, nil
	}
	report, err := dss.New(dssURL).ValidatePDF(ctx, pdfBytes, did+".pdf")
	if err != nil {
		return nil, fmt.Errorf("EU DSS validation of %s failed: %w", did, err)
	}
	finding := fmt.Sprintf("EU DSS validation report: indication=%s", report.Indication)
	if report.SubIndication != "" {
		finding += fmt.Sprintf(" (subIndication=%s)", report.SubIndication)
	}
	return []string{finding}, nil
}

// crossCheckEmbeddedPoA re-verifies the embedded PoA presentation against the
// signature record (UC-04-03): it extracts the signing evidence from the
// stored signed PDF, re-verifies the SD-JWT VC + KB-JWT, and confirms the
// resolved signer DID matches the signature row. Absence of evidence (an
// unsigned contract) yields no findings; any mismatch or verification
// failure is reported as a finding so validate surfaces it.
func (h *Validator) crossCheckEmbeddedPoA(ctx context.Context, tx *sqlx.Tx, did string) []string {
	if h.PDFCore == nil {
		return nil
	}

	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return nil
	}

	evidence, found, err := h.PDFCore.ExtractEvidence(ctx, pdfBytes)
	if err != nil {
		return []string{fmt.Sprintf("Could not extract embedded PoA evidence: %v", err)}
	}
	if !found || len(evidence) == 0 {
		return nil
	}

	// The evidence attachment is a single ContractSigningSummaryCredential
	// for single-signature contracts, or a JSON ARRAY of them for
	// multi-signer contracts (one per declared field, all embedded before
	// the first signature — DCS-FR-SM-07/-17).
	documents := []json.RawMessage{evidence}
	var bundle []json.RawMessage
	if err := json.Unmarshal(evidence, &bundle); err == nil && len(bundle) > 0 {
		documents = bundle
	}

	verifiedSigners := map[string]bool{}
	for _, doc := range documents {
		presentation, subject := signingSummaryPoAFields(doc)
		if presentation == "" {
			return []string{"Embedded signing evidence is missing the PoA presentation"}
		}
		signerDID, _, err := poaverify.Verify(presentation)
		if err != nil {
			return []string{fmt.Sprintf("PoA verification failed: %v", err)}
		}
		if subject != "" && subject != signerDID {
			return []string{"Evidence mismatch: embedded PoA subject does not match the credential subject"}
		}
		verifiedSigners[signerDID] = true
	}

	records, err := h.CRepo.LoadSignatures(ctx, tx, did)
	if err == nil {
		for _, rec := range records {
			if strings.EqualFold(strings.TrimSpace(rec.Status), "REVOKED") {
				continue
			}
			if rec.SignerDID != "" && !verifiedSigners[rec.SignerDID] {
				return []string{"Evidence mismatch: re-verified PoA signer does not match the signature record"}
			}
		}
	}

	return []string{"Embedded PoA presentation re-verified and cross-checked against the signature record"}
}

// crossCheckSHACLDrift (Phase 4, ADR-9) re-runs the Semantic Hub SHACL
// validation the contract was signed under and compares the resulting
// finding hash against the one embedded in the signing-summary credential
// at signing time (validation.SHACLEvidence). A mismatch means the
// contract's stored data has changed since it was signed — a real
// modification, not just a hub schema version bump (evidence is pinned to
// the version active at signing time, ADR-8, so rolling the hub forward
// never causes a false drift finding). Absence of embedded evidence (an
// unsigned contract) yields no finding.
func (h *Validator) crossCheckSHACLDrift(ctx context.Context, tx *sqlx.Tx, did string) []string {
	if h.PDFCore == nil {
		return nil
	}
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return nil
	}
	evidence, found, err := h.PDFCore.ExtractEvidence(ctx, pdfBytes)
	if err != nil || !found || len(evidence) == 0 {
		return nil
	}

	documents := []json.RawMessage{evidence}
	var bundle []json.RawMessage
	if err := json.Unmarshal(evidence, &bundle); err == nil && len(bundle) > 0 {
		documents = bundle
	}

	embeddedHash := ""
	for _, doc := range documents {
		if hash := signingSummarySHACLHash(doc); hash != "" {
			embeddedHash = hash
			break
		}
	}
	if embeddedHash == "" {
		return nil
	}

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil || contract == nil || contract.ContractData == nil {
		return []string{"Could not re-run SHACL validation for drift comparison: contract data unavailable"}
	}
	_, currentHash, err := validation.SHACLEvidence(ctx, *contract.ContractData)
	if err != nil {
		return []string{fmt.Sprintf("Could not re-run SHACL validation for drift comparison: %v", err)}
	}
	if currentHash != embeddedHash {
		return []string{"SHACL drift detected: the contract's stored data no longer matches the validation report embedded at signing time"}
	}
	return []string{"SHACL validation report re-verified against the pinned hub schema version — no drift"}
}

// signingSummarySHACLHash extracts the validation_report_hash field from a
// ContractSigningSummaryCredential evidence document (empty for documents
// signed before Phase 4, or where evidence enrichment was best-effort
// skipped).
func signingSummarySHACLHash(evidence []byte) string {
	var vc struct {
		CredentialSubject struct {
			ValidationReportHash string `json:"validation_report_hash"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil {
		return ""
	}
	return vc.CredentialSubject.ValidationReportHash
}

// signingSummaryPoAFields extracts the verbatim PoA presentation and credential
// subject from a ContractSigningSummaryCredential evidence document.
func signingSummaryPoAFields(evidence []byte) (presentation, subject string) {
	var vc struct {
		CredentialSubject struct {
			ID              string `json:"id"`
			PoAPresentation string `json:"poa_presentation"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil {
		return "", ""
	}
	return vc.CredentialSubject.PoAPresentation, vc.CredentialSubject.ID
}
