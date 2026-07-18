package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/hsm"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/base/validation"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cweevent "digital-contracting-service/internal/contractworkflowengine/event"
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

// ErrCeremoniesIncomplete is returned by the multi-signer flow's
// all-ceremonies-before-first-signature gate (DCS-FR-SM-07/-17): every
// declared signature field needs a verified ceremony before the FIRST
// signature is applied, because every signer's evidence must be embedded
// into the PDF before any PAdES signature freezes it (embedding an
// attachment after a signature trips standards-compliant diff analysis).
var ErrCeremoniesIncomplete = errors.New("all declared signature fields need a completed PID presentation ceremony before the first signature")

// ErrUnknownSignatureField rejects a ceremony/signature for a field the
// contract document does not declare.
var ErrUnknownSignatureField = errors.New("signature field is not declared by the contract document")

// ErrFieldAlreadySigned rejects re-signing an already-signed field.
var ErrFieldAlreadySigned = errors.New("signature field is already signed")

// ApplyCmd carries the inputs for applying a digital signature.
type ApplyCmd struct {
	DID       string
	SignerDID string
	// FieldName selects which declared signature field this signer covers
	// on a multi-signer contract (DCS-FR-SM-07/-17). Empty = single-signer
	// flow (resolve the signer's most recent verified ceremony).
	FieldName      string
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
	// VCIssuer issues the C2PA lifecycle-assertion VC stamped into the base
	// PDF before signing (DCS-OR-C2PA-004) — see stampActiveLifecycle below.
	VCIssuer  provenance.VCIssuer
	IssuerDID string
	// DIDDocument is the instance's HSM-backed signing identity (x5c chain);
	// used to produce the JAdES signature over the machine-readable JSON-LD
	// alongside the visible PAdES on the PDF (DCS-FR-SM-02, DCS-FR-SM-11).
	DIDDocument identity.DIDDocument
	// ArchiveRepo, IPFSStorer, ArchiveNotary, and ArchiveTSA back the
	// archive-entry creation that now happens on reaching SIGNED (DCS-FR-
	// CWE-20), not on APPROVED. ArchiveRepo is the contractworkflowengine
	// repo (same contracts/contract_archive_entries tables as CRepo above,
	// a different package's repo interface) reused purely for its
	// StoreArchiveEntry/ReadDataByDID methods.
	ArchiveRepo   cwedb.ContractRepo
	IPFSStorer    cwecommand.ArchiveSnapshotStorer
	ArchiveNotary cwecommand.ArchiveNotary
	ArchiveTSA    cwecommand.ArchiveTimestampIssuer
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

	prepared, err := h.prepare(ctx, tx, cmd)
	if err != nil {
		return err
	}

	// Transitional DCS-side signing — superseded by the wallet-driven ceremony
	// (ADR-12), kept until the ceremony endpoints replace this entrypoint. The
	// signatory's wallet/QTSP produces these artefacts from prepared.basePDF
	// (evidence embedded, AcroForm field placed) and prepared.jadesPayload; the
	// DCS then validates and finalizes, holding no signing key of its own.
	signedPDF, err := h.Signer.SignPDF(ctx, prepared.basePDF, prepared.ceremony.FieldName, prepared.ceremony.FieldName, prepared.evidence)
	if err != nil {
		return fmt.Errorf("pades sign: %w", err)
	}
	jadesSignature, err := jades.Sign(&h.DIDDocument, prepared.jadesPayload)
	if err != nil {
		return fmt.Errorf("JAdES sign: %w", err)
	}

	if err := h.finalize(ctx, tx, cmd, finalizeInput{
		ceremony:        prepared.ceremony,
		signedPDF:       signedPDF,
		jadesSignature:  jadesSignature,
		contentHash:     prepared.contentHash,
		rendererVersion: prepared.rendererVersion,
		signedCount:     prepared.signedCount,
		vpToken:         prepared.vpToken,
		kbSDHash:        prepared.kbSDHash,
		signedAt:        prepared.signedAt,
		contractVersion: prepared.contractVersion,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// preparedSignature is the to-be-signed material the prepare phase yields: the
// base PDF (AcroForm signature field placed, lifecycle-stamped, NOT yet
// evidence-embedded or signed), the signing-summary evidence to embed, and the
// canonical JAdES payload — plus the ceremony and hashes finalize binds. In the
// wallet-driven ceremony (ADR-12) the base PDF is evidence-embedded and handed
// to the signatory's wallet/QTSP to sign; the DCS applies no signature here.
type preparedSignature struct {
	ceremony        *db.SignatureCeremony
	basePDF         []byte
	basePDFHash     string
	evidence        []byte
	jadesPayload    []byte
	contentHash     string
	signedCount     int
	rendererVersion string
	vpToken         string
	kbSDHash        string
	signedAt        time.Time
	contractVersion int
}

// prepare runs every step up to (but not including) the signature: it enforces
// the ceremony precondition and multi-signer gating, seals the offer into the
// odrl:Agreement on the first signature, runs the policy/closedness/conformance
// and SHACL gates, loads and lifecycle-stamps the base PDF, and issues the
// signing-summary credential(s). It mutates within tx (the sealed agreement is
// persisted) but applies no signature and stores no artefact — the caller
// either signs (transitional Handle) or embeds the evidence and hands the PDF
// to the signatory's wallet (the ceremony download).
func (h *Applier) prepare(ctx context.Context, tx *sqlx.Tx, cmd ApplyCmd) (*preparedSignature, error) {
	// Serialize against the background PDF regenerator on the same per-contract
	// key it uses (pdfgeneration/event). Without this, a genesis/lifecycle
	// regeneration already in flight — holding this lock across its slow
	// pdf-core render — commits its UpdatePDFState *after* SetSignedPDF and
	// overwrites the signed CID with an unsigned re-render, stripping the PAdES
	// signature. Blocking here lets the regenerator finish first; the signed
	// state we then write is frozen, so its later events short-circuit.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", cmd.DID); err != nil {
		return nil, fmt.Errorf("acquire per-contract PDF regeneration lock for %s: %w", cmd.DID, err)
	}

	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}
	if data.ContractData == nil {
		return nil, fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}

	// Ceremony precondition (DCS-FR-SM-16): a completed (verified) PID
	// presentation for this signer and contract must exist. Evaluated before
	// the state-machine transition so a missing ceremony is reported as its own
	// typed error rather than a state error.
	// Resolve the ceremony this signature applies to. On a multi-signer
	// contract several fields may share one signer identity (e.g. one person
	// signing two roles), so resolving by signer alone is ambiguous —
	// FieldName disambiguates when provided; otherwise fall back to the
	// signer's most recent verified ceremony (single-signer flow).
	var ceremony *db.SignatureCeremony
	if cmd.FieldName != "" {
		ceremony, err = h.CeremonyRepo.FindVerifiedCeremonyByField(ctx, tx, cmd.DID, cmd.FieldName)
	} else {
		ceremony, err = h.CeremonyRepo.FindVerifiedCeremony(ctx, tx, cmd.DID, cmd.SignerDID)
	}
	if err != nil {
		return nil, fmt.Errorf("could not resolve signing ceremony: %w", err)
	}
	if ceremony == nil {
		return nil, ErrCeremonyRequired
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventSign); err != nil {
		return nil, err
	}

	// Multi-signer workflow (DCS-FR-SM-07/-17): contracts that declare
	// signature fields require one ceremony+signature per field, applied
	// SEQUENTIALLY (parallel signing is incompatible with PDF/A-3
	// incremental updates — see the change request), with every ceremony
	// completed BEFORE the first signature so all signers' evidence is
	// embedded ahead of the signature that freezes the document.
	requiredFields := validation.RequiredSignatureFields(*data.ContractData)
	existingRecords, err := h.CRepo.LoadSignatures(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not load existing signatures: %w", err)
	}
	signedCount := 0
	for _, rec := range existingRecords {
		if rec.Status != "SIGNED" {
			continue
		}
		signedCount++
		if rec.FieldName != nil && *rec.FieldName == ceremony.FieldName {
			return nil, fmt.Errorf("%w: %s", ErrFieldAlreadySigned, ceremony.FieldName)
		}
	}
	fieldCeremonies := map[string]*db.SignatureCeremony{ceremony.FieldName: ceremony}
	if len(requiredFields) > 0 {
		declared := false
		for _, f := range requiredFields {
			if f == ceremony.FieldName {
				declared = true
				break
			}
		}
		if !declared {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSignatureField, ceremony.FieldName)
		}
		if signedCount == 0 {
			var missing []string
			for _, f := range requiredFields {
				c, err := h.CeremonyRepo.FindVerifiedCeremonyByField(ctx, tx, cmd.DID, f)
				if err != nil {
					return nil, fmt.Errorf("could not resolve ceremony for field %q: %w", f, err)
				}
				if c == nil {
					missing = append(missing, f)
					continue
				}
				fieldCeremonies[f] = c
			}
			if _, ok := fieldCeremonies[ceremony.FieldName]; !ok {
				fieldCeremonies[ceremony.FieldName] = ceremony
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("%w: missing ceremonies for %v", ErrCeremoniesIncomplete, missing)
			}
		}
	}

	// The first signature is the acceptance act: the offered policy set
	// becomes the odrl:Agreement the signatures bind, sealed into the
	// contract document BEFORE the content hash and PDF are computed so the
	// signed artefact and the machine-readable document are the same bytes.
	if signedCount == 0 {
		sealed, err := sealAgreementForSigning(*data.ContractData, data.Responsible, cmd.SignerDID)
		if err != nil {
			return nil, fmt.Errorf("seal agreement for signing: %w", err)
		}
		if err := h.CRepo.UpdateContractData(ctx, tx, cmd.DID, sealed); err != nil {
			return nil, fmt.Errorf("persist sealed agreement: %w", err)
		}
		data.ContractData = &sealed
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
		return nil, err
	}

	// Signatures are the point of no return: a contract must be closed — no
	// unresolved placeholders — before it is sealed into an odrl:Agreement and
	// signed. A template's open policy is only ever a contract once every
	// placeholder is materialized.
	if err := validation.ValidateContractClosed(*data.ContractData); err != nil {
		return nil, fmt.Errorf("signature application blocked: %w", err)
	}

	// A non-conformant contract must never be signed (DCS-FR-PACM-03) —
	// submission already gates this, but signatures are the point of no
	// return, so the invariant is re-checked here.
	if err := validation.RequireHubConformance(ctx, *data.ContractData); err != nil {
		return nil, fmt.Errorf("signature application blocked: %w", err)
	}

	// SHACL evidence (Phase 4, ADR-9): the hub schema version this contract
	// validates against and a stable hash of the resulting findings, bound
	// into the signing-summary credential below — an external verifier
	// resolves sh:shapesGraph to fetch those exact pinned shapes, re-runs
	// validation, and compares hashes to detect drift.
	schemaVersion, validationReportHash, err := validation.SHACLEvidence(ctx, *data.ContractData)
	if err != nil {
		return nil, fmt.Errorf("SHACL evidence for signing-summary credential: %w", err)
	}

	// Load (or generate) the base PDF to be signed.
	basePDF, err := h.loadBasePDF(ctx, tx, cmd.DID, *data.ContractData)
	if err != nil {
		return nil, err
	}

	// Stamp the "active" C2PA lifecycle assertion into the base PDF BEFORE
	// signing it (update-then-sign), not after. The signed artefact must never
	// be mutated again once it carries a PAdES signature: any subsequent
	// incremental update to a referenced embedded-file object (the C2PA
	// manifest attachment) — however carefully byte-range-preserving — is
	// flagged as an unexplained/illegal modification by standards-compliant
	// PAdES validators (Adobe Reader, pyHanko's diff-analysis), even though the
	// CMS signature itself stays cryptographically valid. Stamping here means
	// the signature commits to the PDF's FINAL lifecycle-bearing content, so
	// exportcontract.go/verifycontract.go never need to touch it again for the
	// SIGNED/ACTIVE C2PA state (DCS-OR-C2PA-004, DCS-FR-SM-16).
	rendererVersion := ""
	if signedCount == 0 {
		stampedPDF, rv, err := stampLifecycleForSigning(ctx, cmd.DID, *data.ContractData, basePDF, h.PDFCore, h.VCIssuer, h.IssuerDID)
		if err != nil {
			return nil, fmt.Errorf("stamp active lifecycle assertion before signing: %w", err)
		}
		basePDF = stampedPDF
		rendererVersion = rv
	}
	// A PDF that already carries a PAdES signature is never stamped again —
	// it was stamped before the FIRST signature, and any later mutation
	// besides an incremental signature is an illegal modification.

	contentSum := sha256.Sum256(*data.ContractData)
	contentHash := hex.EncodeToString(contentSum[:])
	basePDFSum := sha256.Sum256(basePDF)
	basePDFHash := hex.EncodeToString(basePDFSum[:])

	// Issue the signing-summary credential carrying the verbatim PID
	// presentation, to be embedded before signing (embed-first-sign-second).
	vpToken := ""
	if ceremony.VpToken != nil {
		vpToken = *ceremony.VpToken
	}
	kbSDHash := ""
	if ceremony.KbSdHash != nil {
		kbSDHash = *ceremony.KbSdHash
	}
	signedAt := time.Now().UTC()
	var evidence []byte
	switch {
	case len(requiredFields) == 0:
		// Single-signature contract: one summary VC, the established shape.
		evidence, _, err = provenance.IssueSigningSummaryVC(ctx, h.VCSigner, h.IssuerDID, provenance.SigningSummary{
			ContractID:           cmd.DID,
			SignerDID:            cmd.SignerDID,
			CeremonyID:           ceremony.ID,
			FieldName:            ceremony.FieldName,
			ContentHash:          contentHash,
			PDFHash:              basePDFHash,
			CredentialType:       cmd.CredentialType,
			KBSDHash:             kbSDHash,
			SignedAt:             signedAt,
			SchemaVersion:        schemaVersion,
			ValidationReportHash: validationReportHash,
		})
		if err != nil {
			return nil, fmt.Errorf("issue signing-summary VC: %w", err)
		}
	case signedCount == 0:
		// First signature on a multi-signer contract: embed EVERY declared
		// field's summary VC as a JSON array, so no later signer needs a
		// post-signature attachment (all-ceremonies-before-first-signature).
		summaries := make([]json.RawMessage, 0, len(requiredFields))
		for _, f := range requiredFields {
			c := fieldCeremonies[f]
			fieldKB := ""
			if c.KbSdHash != nil {
				fieldKB = *c.KbSdHash
			}
			fieldSigner := ""
			if c.SignerDID != nil {
				fieldSigner = *c.SignerDID
			}
			credentialType := cmd.CredentialType
			if f != ceremony.FieldName {
				// The other signers' signature level is recorded when THEY
				// apply; their embedded ceremony evidence carries the
				// required default level (QES is out of scope per SRS).
				credentialType = "AES"
			}
			vc, _, err := provenance.IssueSigningSummaryVC(ctx, h.VCSigner, h.IssuerDID, provenance.SigningSummary{
				ContractID:           cmd.DID,
				SignerDID:            fieldSigner,
				CeremonyID:           c.ID,
				FieldName:            f,
				ContentHash:          contentHash,
				PDFHash:              basePDFHash,
				CredentialType:       credentialType,
				KBSDHash:             fieldKB,
				SignedAt:             signedAt,
				SchemaVersion:        schemaVersion,
				ValidationReportHash: validationReportHash,
			})
			if err != nil {
				return nil, fmt.Errorf("issue signing-summary VC for field %q: %w", f, err)
			}
			summaries = append(summaries, vc)
		}
		evidence, err = json.Marshal(summaries)
		if err != nil {
			return nil, fmt.Errorf("encode signing-summary evidence bundle: %w", err)
		}
	default:
		// Later signature on a multi-signer contract: its evidence is
		// already embedded (see above); the signed document must not be
		// mutated beyond the incremental signature itself.
		evidence = nil
	}

	// The JAdES payload over the machine-readable JSON-LD, the counterpart to
	// the visible PAdES on the PDF: one signature event covers both
	// representations (DCS-FR-SM-02, DCS-FR-SM-11), so an external verifier can
	// validate the contract's terms from the canonical JSON-LD without the PDF.
	jadesPayload, err := jades.BuildContractPayload(cmd.DID, data.ContractVersion, *data.ContractData)
	if err != nil {
		return nil, fmt.Errorf("build JAdES payload: %w", err)
	}

	return &preparedSignature{
		ceremony:        ceremony,
		basePDF:         basePDF,
		basePDFHash:     basePDFHash,
		evidence:        evidence,
		jadesPayload:    jadesPayload,
		contentHash:     contentHash,
		signedCount:     signedCount,
		rendererVersion: rendererVersion,
		vpToken:         vpToken,
		kbSDHash:        kbSDHash,
		signedAt:        signedAt,
		contractVersion: data.ContractVersion,
	}, nil
}

// finalizeInput carries the post-signature state the Finalizer persists: the
// wallet-signed PDF, the JAdES over the machine-readable JSON-LD, and the
// hashes/ceremony metadata bound into the signature record and archive entry.
type finalizeInput struct {
	ceremony        *db.SignatureCeremony
	signedPDF       []byte
	jadesSignature  string
	contentHash     string
	rendererVersion string
	signedCount     int
	vpToken         string
	kbSDHash        string
	signedAt        time.Time
	contractVersion int
}

// finalize persists a completed signature: it stores the signed PDF in IPFS,
// points the contract at it, records the signature (PAdES hash + JAdES),
// transitions to SIGNED, and — on the first signature — archives the contract.
// In the wallet-driven ceremony the signedPDF and jadesSignature originate from
// the signatory's wallet/QTSP (the DCS holds no signing key); this is the
// receive-and-record half the ceremony callback invokes after validating the
// returned signature.
func (h *Applier) finalize(ctx context.Context, tx *sqlx.Tx, cmd ApplyCmd, in finalizeInput) error {
	signedPDFSum := sha256.Sum256(in.signedPDF)
	signedPDFHash := hex.EncodeToString(signedPDFSum[:])

	ipfsRes, err := h.IPFSClient.CreateFile(ctx, in.signedPDF)
	if err != nil {
		return fmt.Errorf("store signed PDF in IPFS: %w", err)
	}
	cid := ipfsRes.Identifier.Value

	// Confirm the artefact resolves through the read path before persisting its
	// CID. The tenant store is eventually consistent, so a CID CreateFile has
	// just returned is not always immediately retrievable; persisting it early
	// would let a later export/verify fetch the contract's PDF and fail
	// (DCS-FR-SM-16). FetchFile retries the transient not-yet-resolvable window.
	readback, err := h.IPFSClient.FetchFile(cid)
	if err != nil || readback == nil || len(readback.Data) == 0 {
		return fmt.Errorf("signed PDF CID %s not resolvable after store: %w", cid, err)
	}

	// contentHash (computed from *data.ContractData) is the same payload hash
	// exportcontract.go/verifycontract.go compare against, so recording it here
	// means the first export/verify after signing sees a matching hash and
	// serves the frozen signed PDF as-is instead of appending a post-signature
	// revision.
	if err := h.CRepo.SetSignedPDF(ctx, tx, cmd.DID, cid, in.rendererVersion, "active", in.contentHash); err != nil {
		return err
	}

	keyVersion, err := h.CRepo.ActiveKeyVersion(ctx, tx, hsm.KeyLabelPADES())
	if err != nil {
		return fmt.Errorf("could not resolve active key version: %w", err)
	}

	ceremonyID := in.ceremony.ID
	fieldName := in.ceremony.FieldName
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
		ContentHash:    &in.contentHash,
		FieldName:      &fieldName,
		JAdESSignature: &in.jadesSignature,
	}
	if err := h.CRepo.CreateSignature(ctx, tx, signature); err != nil {
		return fmt.Errorf("could not create signature: %w", err)
	}

	if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Signed.String()); err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	// The archive entry is created when the contract REACHES SIGNED (first
	// signature); later multi-signer signatures update the stored artefact
	// pointer above but never insert a second entry for the same version.
	if in.signedCount == 0 {
		credentialHashes := map[string]string{}
		if in.vpToken != "" {
			sum := sha256.Sum256([]byte(in.vpToken))
			credentialHashes["presentation"] = "sha256:" + hex.EncodeToString(sum[:])
		}
		if in.kbSDHash != "" {
			credentialHashes["key_binding"] = "sha256:" + strings.TrimPrefix(in.kbSDHash, "sha256:")
		}
		if err := h.archiveSignedContract(ctx, tx, cmd.DID, cmd.AppliedBy, cwecommand.ArchiveSigningEvidence{Signer: cmd.SignerDID, CredentialType: cmd.CredentialType, CeremonyID: in.ceremony.ID, Field: in.ceremony.FieldName, SignedAt: in.signedAt, PDFCID: cid, PDFHash: signedPDFHash, CredentialHashes: credentialHashes}); err != nil {
			return err
		}
	}

	evt := event2.ApplyEvent{
		DID:             cmd.DID,
		ContractVersion: in.contractVersion,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		CredentialType:  cmd.CredentialType,
		AppliedBy:       cmd.AppliedBy,
		OccurredAt:      in.signedAt,
	}
	if err := event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return nil
}

// archiveSignedContract creates the archive entry for a contract that just
// reached SIGNED (DCS-FR-CWE-20: the archive-entry trigger is gated to
// SIGNED, not APPROVED), notarizing and RFC-3161-TSA-timestamping it exactly
// as the former APPROVED-time trigger did.
func (h *Applier) archiveSignedContract(ctx context.Context, tx *sqlx.Tx, did string, appliedBy string, signingEvidence cwecommand.ArchiveSigningEvidence) error {
	signedContract, err := h.ArchiveRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read signed contract for archive storage: %w", err)
	}

	archiveEntry, err := cwecommand.BuildArchiveEntry(signedContract, appliedBy, signingEvidence)
	if err != nil {
		return fmt.Errorf("could not build archive entry: %w", err)
	}
	if h.IPFSStorer == nil {
		return errors.New("archive snapshot IPFS storer is required")
	}
	snapshotResult, err := h.IPFSStorer.CreateFile(ctx, archiveEntry.ContractSnapshot)
	if err != nil {
		return fmt.Errorf("could not store archive snapshot in IPFS: %w", err)
	}
	if snapshotResult == nil || snapshotResult.Identifier.Value == "" {
		return errors.New("archive snapshot IPFS storer returned empty CID")
	}
	archiveEntry.SnapshotCID = snapshotResult.Identifier.Value

	archiveEntryID := fmt.Sprintf("%s#%d", did, signedContract.ContractVersion)
	notaryPayload := cwecommand.ArchiveNotaryPayload{
		EventType:       "ARCHIVE_STORED",
		ArchiveEntryID:  archiveEntryID,
		DID:             did,
		ContractVersion: signedContract.ContractVersion,
		ContentHash:     archiveEntry.ContentHash,
		SnapshotCID:     archiveEntry.SnapshotCID,
		StoredBy:        appliedBy,
		StoredAt:        archiveEntry.StoredAt,
	}
	var notaryReceipt *cwecommand.ArchiveNotaryReceipt
	if h.ArchiveNotary != nil {
		notaryReceipt, err = h.ArchiveNotary.NotarizeArchiveEntry(ctx, notaryPayload)
		if err != nil {
			return fmt.Errorf("could not notarize archive entry: %w", err)
		}
	}

	var tsaReceipt *cweevent.ArchiveTSAReceipt
	if h.ArchiveTSA != nil && h.ArchiveTSA.Enabled() && notaryReceipt != nil {
		evidence, err := cwecommand.BuildArchiveTimestampEvidence(notaryPayload, notaryReceipt)
		if err != nil {
			return fmt.Errorf("could not build archive TSA evidence: %w", err)
		}
		evidenceBytes, err := cwecommand.CanonicalArchiveTimestampEvidence(evidence)
		if err != nil {
			return err
		}
		rawReceipt, err := h.ArchiveTSA.TimestampBytes(ctx, evidenceBytes)
		if err != nil {
			return fmt.Errorf("could not timestamp archive entry: %w", err)
		}
		tsaReceipt = &cweevent.ArchiveTSAReceipt{
			ReceiptType:    "ARCHIVE_TSA_RECEIPT",
			Token:          rawReceipt.Token,
			TokenEncoding:  rawReceipt.TokenEncoding,
			HashAlgorithm:  rawReceipt.HashAlgorithm,
			MessageImprint: rawReceipt.MessageImprint,
			GeneratedAt:    rawReceipt.GeneratedAt,
			Policy:         rawReceipt.Policy,
			SerialNumber:   rawReceipt.SerialNumber,
		}
		tsaReceiptJSON, err := datatype.NewJSON(tsaReceipt)
		if err != nil {
			return fmt.Errorf("could not encode archive TSA receipt: %w", err)
		}
		archiveEntry.TSAReceipt = &tsaReceiptJSON
	}

	if err := h.ArchiveRepo.StoreArchiveEntry(ctx, tx, archiveEntry); err != nil {
		return fmt.Errorf("could not store contract in archive: %w", err)
	}

	var notaryEventReceipt *cweevent.ArchiveNotaryReceipt
	if notaryReceipt != nil {
		notaryEventReceipt = &cweevent.ArchiveNotaryReceipt{
			ReceiptType:    notaryReceipt.ReceiptType,
			ArchiveEntryID: notaryReceipt.ArchiveEntryID,
			EventHash:      notaryReceipt.EventHash,
			PreviousHash:   notaryReceipt.PreviousHash,
			ReceivedAt:     notaryReceipt.ReceivedAt,
		}
	}
	archiveEvt := cweevent.StoreArchivedEvent{
		DID:             did,
		ContractVersion: signedContract.ContractVersion,
		StoredBy:        appliedBy,
		ContentHash:     archiveEntry.ContentHash,
		SnapshotCID:     archiveEntry.SnapshotCID,
		ArchiveStatus:   "STORED",
		NotaryReceipt:   notaryEventReceipt,
		TSAReceipt:      tsaReceipt,
		EvidenceSummary: cweevent.ArchiveEvidenceSummary{
			SnapshotHashAlgorithm: "SHA-256",
			SignatureStatus:       "SIGNED",
			CredentialHashStatus:  "HASHED",
		},
		OccurredAt: time.Now().UTC(),
	}
	if err := event.Create(ctx, tx, archiveEvt, componenttype.ContractStorageArchive); err != nil {
		return fmt.Errorf("could not create archive store event: %w", err)
	}

	return nil
}

// loadBasePDF returns the current PDF for the contract, generating a fresh base
// render from the JSON-LD when none is cached yet.
func (h *Applier) loadBasePDF(ctx context.Context, tx *sqlx.Tx, did string, jsonld []byte) ([]byte, error) {
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("fetch contract PDF: %w", err)
	}
	if len(pdfBytes) == 0 {
		pdfBytes, _, err = h.PDFCore.Download(ctx, jsonld)
		if err != nil {
			return nil, fmt.Errorf("render base PDF: %w", err)
		}
	}
	return pdfBytes, nil
}

// stampLifecycleForSigning embeds the "active" C2PA lifecycle assertion
// (DCS-OR-C2PA-004) into pdfBytes and returns the updated PDF plus the
// renderer version pdf-core reports. It is the update-then-sign counterpart of
// pdfgeneration/query.stampLifecycle: called BEFORE PAdES-signing so the
// signature commits to the PDF's final lifecycle-bearing content, and the
// signed artefact never needs a post-signature revision for the SIGNED/ACTIVE
// transition (see the Applier.VCIssuer field doc comment).
func stampLifecycleForSigning(
	ctx context.Context,
	did string,
	jsonldBytes, pdfBytes []byte,
	pdfCore *pdfcore.Client,
	vcIssuer provenance.VCIssuer,
	issuerDID string,
) ([]byte, string, error) {
	const c2paState = "active"
	const reason = "Contract activated for execution"

	h := sha256.Sum256(pdfBytes)
	assetHash := hex.EncodeToString(h[:])

	_, vcBytes, err := vcIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, issuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, "", fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := pdfCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, provenance.RemoteManifestURL(did))
	if err != nil {
		return pdfBytes, "", fmt.Errorf("pdf-core update for %s: %w", did, err)
	}
	return updatedPDF, rendererVersion, nil
}

// sealAgreementForSigning turns the offered policy set into the
// odrl:Agreement the signatures bind: the enclosing policy node retypes,
// and a still-open role-derived party placeholder is rewritten to the
// accepting counterparty's identity — the one workflow peer distinct from
// the originator when there is exactly one, otherwise the signer's
// verified DID — with the signing identity recorded as dcs:hasSignatory.
// Binding only happens while exactly one placeholder remains open, so an
// undeclared originator role never gets mislabeled as the counterparty.
func sealAgreementForSigning(raw datatype.JSON, responsible *db.Responsible, signerDID string) (datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode contract data: %w", err)
	}

	if policies, ok := doc["dcs:policies"].(map[string]any); ok {
		policies["@type"] = "odrl:Agreement"
	}

	// The offeror is the contracting party (ODRL §4.3.7 — "the Party who is
	// offering the contract"); the accepting counterparty is the contracted
	// party (§4.3.8). Both are signatories.
	if responsible != nil && responsible.Creator != "" {
		if node := partyNodeByID(doc, responsible.Creator); node != nil {
			node["odrl:function"] = map[string]any{"@id": "odrl:contractingParty"}
		}
	}

	if placeholder := singleOpenPartyPlaceholder(doc); placeholder != "" {
		counterparty := counterpartyIdentity(responsible, signerDID)
		replaceNodeIRI(doc, placeholder, counterparty)
		if node := partyNodeByID(doc, counterparty); node != nil {
			node["dcs:hasSignatory"] = map[string]any{"@id": signerDID}
			node["odrl:function"] = map[string]any{"@id": "odrl:contractedParty"}
		}
	}

	return datatype.NewJSON(doc)
}

// counterpartyIdentity resolves who accepted the offer: the single workflow
// peer that is not the originating instance, or the verified signer when
// the workflow ran on one instance.
func counterpartyIdentity(responsible *db.Responsible, signerDID string) string {
	if responsible == nil {
		return signerDID
	}
	peers := map[string]bool{}
	for _, group := range [][]string{responsible.Reviewers, responsible.Approvers, responsible.Negotiators} {
		for _, peer := range group {
			if peer != "" && peer != responsible.Creator {
				peers[peer] = true
			}
		}
	}
	if len(peers) == 1 {
		for peer := range peers {
			return peer
		}
	}
	return signerDID
}

// singleOpenPartyPlaceholder returns the IRI of the only dcs:parties node
// still carrying a role-derived #party-<role> placeholder ("" when none or
// several remain).
func singleOpenPartyPlaceholder(doc map[string]any) string {
	nodes, _ := doc["dcs:parties"].([]any)
	open := []string{}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		iri, _ := node["@id"].(string)
		if _, role, found := strings.Cut(iri, "#party-"); found {
			if _, isIndexed := strconvAtoiOK(role); !isIndexed {
				open = append(open, iri)
			}
		}
	}
	if len(open) == 1 {
		return open[0]
	}
	return ""
}

// strconvAtoiOK reports whether s is a plain index (an attachContractParties
// read-authorization node, never a role placeholder).
func strconvAtoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func partyNodeByID(doc map[string]any, id string) map[string]any {
	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		if node, ok := rawNode.(map[string]any); ok {
			if iri, _ := node["@id"].(string); iri == id {
				return node
			}
		}
	}
	return nil
}

// replaceNodeIRI rewrites every "@id" equal to old with new, recursively.
func replaceNodeIRI(current any, old, new string) {
	switch value := current.(type) {
	case map[string]any:
		if iri, _ := value["@id"].(string); iri == old {
			value["@id"] = new
		}
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	case []any:
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	}
}
