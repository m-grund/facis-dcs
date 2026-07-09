package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/hsm"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/db"
	event2 "digital-contracting-service/internal/signingmanagement/event"
	"digital-contracting-service/internal/signingmanagement/signer"

	"github.com/jmoiron/sqlx"
)

// ErrCeremonyRequired is the typed precondition failure returned when a
// signature is applied for a signer/contract that has no completed PID
// presentation ceremony (DCS-FR-SM-16, FR-SM-25, UC-04-02).
var ErrCeremonyRequired = errors.New("a completed PID presentation ceremony is required before signing")

// ApplyCmd carries the inputs for applying a digital signature.
type ApplyCmd struct {
	DID            string
	SignerDID      string
	CredentialType string
	AppliedBy      string
	HolderDID      string
	UserRoles      userrole.UserRoles
}

// Applier handles the ApplyCmd command.
type Applier struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	CeremonyRepo db.CeremonyRepo
	Signer       signer.ContractSigner
	PDFCore      *pdfcore.Client
	IPFSClient   *ipfs.APIClient
	VCSigner     provenance.VCSigner
	IssuerDID    string
}

// Handle applies a PAdES digital signature to a contract (DCS-FR-SM-16,
// DCS-IR-SI-10). It first enforces the PID-presentation ceremony precondition
// (orthogonal to, and evaluated before, the APPROVED -> SIGNED state gate),
// then embeds the presentation and a ContractSigningSummaryCredential into the
// PDF and signs it (embed-first-sign-second), stores the signed artefact in
// IPFS, and binds both the signed-PDF hash and the JSON-LD content hash.
func (h *Applier) Handle(ctx context.Context, cmd ApplyCmd) error {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}
	if data.ContractData == nil {
		return fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}

	// Ceremony precondition (DCS-FR-SM-16): a completed (verified) PID
	// presentation for this signer and contract must exist. Evaluated before
	// the state-machine transition so a missing ceremony is reported as its own
	// typed error rather than a state error.
	ceremony, err := h.CeremonyRepo.FindVerifiedCeremony(ctx, tx, cmd.DID, cmd.SignerDID)
	if err != nil {
		return fmt.Errorf("could not resolve signing ceremony: %w", err)
	}
	if ceremony == nil {
		return ErrCeremonyRequired
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventSign); err != nil {
		return err
	}

	if err := validation.ValidateContractPolicySatisfaction(
		*data.ContractData,
		validation.ContractContentAuditMetadata{
			ContractDID:     cmd.DID,
			ContractVersion: fmt.Sprint(data.ContractVersion),
			AuditedBy:       cmd.AppliedBy,
			HolderDID:       cmd.HolderDID,
		},
	); err != nil {
		return err
	}

	// Load (or generate) the base PDF to be signed.
	basePDF, err := h.loadBasePDF(ctx, tx, cmd.DID, *data.ContractData)
	if err != nil {
		return err
	}

	contentSum := sha256.Sum256(*data.ContractData)
	contentHash := hex.EncodeToString(contentSum[:])
	basePDFSum := sha256.Sum256(basePDF)
	basePDFHash := hex.EncodeToString(basePDFSum[:])

	// Issue the signing-summary credential carrying the verbatim PID
	// presentation, then embed it and sign (embed-first-sign-second).
	vpToken := ""
	if ceremony.VpToken != nil {
		vpToken = *ceremony.VpToken
	}
	kbSDHash := ""
	if ceremony.KbSdHash != nil {
		kbSDHash = *ceremony.KbSdHash
	}
	signedAt := time.Now().UTC()
	evidence, _, err := provenance.IssueSigningSummaryVC(ctx, h.VCSigner, h.IssuerDID, provenance.SigningSummary{
		ContractID:      cmd.DID,
		SignerDID:       cmd.SignerDID,
		CeremonyID:      ceremony.ID,
		FieldName:       ceremony.FieldName,
		ContentHash:     contentHash,
		PDFHash:         basePDFHash,
		CredentialType:  cmd.CredentialType,
		KBSDHash:        kbSDHash,
		PIDPresentation: vpToken,
		SignedAt:        signedAt,
	})
	if err != nil {
		return fmt.Errorf("issue signing-summary VC: %w", err)
	}

	signedPDF, err := h.Signer.SignPDF(ctx, basePDF, ceremony.FieldName, ceremony.FieldName, evidence)
	if err != nil {
		return fmt.Errorf("pades sign: %w", err)
	}

	signedPDFSum := sha256.Sum256(signedPDF)
	signedPDFHash := hex.EncodeToString(signedPDFSum[:])

	ipfsRes, err := h.IPFSClient.CreateFile(ctx, signedPDF)
	if err != nil {
		return fmt.Errorf("store signed PDF in IPFS: %w", err)
	}
	cid := ipfsRes.Identifier.Value
	if err := h.CRepo.SetSignedPDF(ctx, tx, cmd.DID, cid, pdfcore.RendererVersion, "active"); err != nil {
		return err
	}

	keyVersion, err := h.CRepo.ActiveKeyVersion(ctx, tx, hsm.KeyLabelPADES())
	if err != nil {
		return fmt.Errorf("could not resolve active key version: %w", err)
	}

	ceremonyID := ceremony.ID
	signature := db.ContractSignature{
		ContractDID:    cmd.DID,
		Status:         "SIGNED",
		SignatureBytes: signedPDFSum[:],
		SignerDID:      cmd.SignerDID,
		CredentialType: cmd.CredentialType,
		KeyVersion:     keyVersion,
		IpfsCID:        &cid,
		CeremonyID:     &ceremonyID,
		PDFHash:        &signedPDFHash,
		ContentHash:    &contentHash,
	}
	if err := h.CRepo.CreateSignature(ctx, tx, signature); err != nil {
		return fmt.Errorf("could not create signature: %w", err)
	}

	if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Signed.String()); err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	evt := event2.ApplyEvent{
		DID:             cmd.DID,
		ContractVersion: data.ContractVersion,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		CredentialType:  cmd.CredentialType,
		AppliedBy:       cmd.AppliedBy,
		OccurredAt:      signedAt,
	}
	if err := event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// loadBasePDF returns the current PDF for the contract, generating a fresh base
// render from the JSON-LD when none is cached yet. IPFS may return the artefact
// base64-encoded (the C2PA lifecycle subscriber stores it that way); such bytes
// are decoded back to raw PDF here.
func (h *Applier) loadBasePDF(ctx context.Context, tx *sqlx.Tx, did string, jsonld []byte) ([]byte, error) {
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("fetch contract PDF: %w", err)
	}
	pdfBytes = decodePDFBytes(pdfBytes)
	if len(pdfBytes) == 0 {
		pdfBytes, _, err = h.PDFCore.Download(ctx, jsonld)
		if err != nil {
			return nil, fmt.Errorf("render base PDF: %w", err)
		}
	}
	return pdfBytes, nil
}

// decodePDFBytes returns raw PDF bytes, base64-decoding the input when it is not
// already a PDF (some IPFS write paths store the artefact base64-encoded).
func decodePDFBytes(b []byte) []byte {
	if len(b) == 0 || bytes.HasPrefix(b, []byte("%PDF")) {
		return b
	}
	if decoded, err := base64.StdEncoding.DecodeString(string(b)); err == nil && bytes.HasPrefix(decoded, []byte("%PDF")) {
		return decoded
	}
	return b
}
