package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/builder"
	"digital-contracting-service/internal/pdfgeneration/verify"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	"digital-contracting-service/internal/signingmanagement/query"
)

type signatureManagementsrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	ATrailReader base.AuditTrailReader
	DSSClient    dss.Client
	IPFSClient   *ipfs.APIClient
	auth.JWTAuthenticator
}

func NewSignatureManagement(
	database *sqlx.DB,
	jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo,
	auditTrailReader base.AuditTrailReader,
	dssClient dss.Client,
	ipfsClient *ipfs.APIClient,
) signaturemanagement.Service {
	return &signatureManagementsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               database,
		CRepo:            cRepo,
		ATrailReader:     auditTrailReader,
		DSSClient:        dssClient,
		IPFSClient:       ipfsClient,
	}
}

func (s *signatureManagementsrvc) Retrieve(ctx context.Context, req *signaturemanagement.SMContractRetrieveRequest) (res *signaturemanagement.SMContractRetrieveResponse, err error) {

	qry := query.GetAllMetadataQry{
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := query.GetAllMetadataHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	var contracts []*signaturemanagement.SMContractListItem
	for _, item := range result.Contracts {
		contracts = append(contracts, &signaturemanagement.SMContractListItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		})
	}

	signingTasks, stErr := s.fetchSigningTasks(ctx)
	if stErr != nil {
		log.Printf(ctx, "signatureManagement.retrieve: fetch signing tasks: %v", stErr)
		signingTasks = []*signaturemanagement.SMContractSigningTaskItem{}
	}

	return &signaturemanagement.SMContractRetrieveResponse{
		Contracts:    contracts,
		SigningTasks: signingTasks,
	}, nil
}

func (s *signatureManagementsrvc) RetrieveByID(ctx context.Context, req *signaturemanagement.SMContractRetrieveByIDRequest) (res *signaturemanagement.SMContractRetrieveByIDResponse, err error) {

	qry := query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := query.GetByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	contractResult, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	contract := signaturemanagement.SMContractItem{
		Did:             contractResult.DID,
		ContractVersion: contractResult.ContractVersion,
		State:           contractResult.State.String(),
		Name:            contractResult.Name,
		Description:     contractResult.Description,
		CreatedAt:       contractResult.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       contractResult.UpdatedAt.Format(time.RFC3339),
	}

	// Fetch the latest non-revoked signature envelope from DB.
	envelope, err := s.fetchLatestEnvelope(ctx, req.Did)
	if err != nil {
		log.Printf(ctx, "signatureManagement.retrieveByID: fetch envelope: %v", err)
		envelope = &signaturemanagement.SMContractSignatureEnvelope{
			ContractDid:    req.Did,
			SignerDid:      "",
			CredentialType: "",
			Status:         "NONE",
		}
	}

	return &signaturemanagement.SMContractRetrieveByIDResponse{
		Contract:          &contract,
		SignatureEnvelope: envelope,
	}, nil
}

func (s *signatureManagementsrvc) Verify(ctx context.Context, req *signaturemanagement.SMContractVerifyRequest) (res *signaturemanagement.SMContractVerifyResponse, err error) {
	// Count active signatures.
	verifier := command.SignatureVerifier{DB: s.DB, CRepo: s.CRepo}
	vResult, err := verifier.Handle(ctx, command.VerifyCmd{DID: req.Did})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	// Fetch PDF bytes and run MR/HR hash check (DCS-FR-CWE-04).
	pdfBytes, fetchErr := s.fetchContractPDFBytes(ctx, req.Did)
	if fetchErr != nil || len(pdfBytes) == 0 {
		// No PDF yet — return match=false with sig count only.
		return &signaturemanagement.SMContractVerifyResponse{
			Did:      req.Did,
			Match:    false,
			SigCount: vResult.ActiveSigCount,
		}, nil
	}

	contractVerifier := &verify.ContractVerifier{
		BuildFn: func(jsonld []byte) ([]byte, error) {
			return s.rebuildContractPDFFromJSONLD(ctx, req.Did, jsonld)
		},
	}
	hashResult, err := contractVerifier.Verify(pdfBytes)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("verify PDF: %w", err))
	}

	jsonldHash := hashResult.JSONLDHash
	basePDFHash := hashResult.BasePDFHash
	return &signaturemanagement.SMContractVerifyResponse{
		Did:         req.Did,
		Match:       hashResult.Match,
		JsonldHash:  &jsonldHash,
		BasePdfHash: &basePDFHash,
		SigCount:    vResult.ActiveSigCount,
	}, nil
}

func (s *signatureManagementsrvc) Apply(ctx context.Context, req *signaturemanagement.SMContractApplyRequest) (res *signaturemanagement.SMContractApplyResponse, err error) {
	credType := "stub"
	if req.CredentialType != nil && *req.CredentialType != "" {
		credType = *req.CredentialType
	}

	applier := command.Applier{DB: s.DB, CRepo: s.CRepo}
	if err := applier.Handle(ctx, command.ApplyCmd{
		DID:            req.Did,
		SignerDID:      req.SignerDid,
		CredentialType: credType,
		AppliedBy:      middleware.GetUsername(ctx),
		DSSClient:      s.DSSClient,
	}); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	envelope, err := s.fetchLatestEnvelope(ctx, req.Did)
	if err != nil {
		envelope = &signaturemanagement.SMContractSignatureEnvelope{
			ContractDid:    req.Did,
			SignerDid:      req.SignerDid,
			CredentialType: credType,
			Status:         "SIGNED",
		}
	}

	return &signaturemanagement.SMContractApplyResponse{
		Did:               req.Did,
		SignatureEnvelope: envelope,
	}, nil
}

func (s *signatureManagementsrvc) Validate(ctx context.Context, req *signaturemanagement.SMContractValidateRequest) (res *signaturemanagement.SMContractValidateResponse, err error) {
	findings, err := s.collectValidationFindings(ctx, req.Did)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	handler := command.Validator{DB: s.DB, CRepo: s.CRepo}
	if err := handler.Handle(ctx, command.ValidateCmd{
		DID:         req.Did,
		ValidatedBy: middleware.GetUsername(ctx),
	}); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	return &signaturemanagement.SMContractValidateResponse{Did: req.Did, Findings: findings}, nil
}

func (s *signatureManagementsrvc) Revoke(ctx context.Context, req *signaturemanagement.SMContractRevokeRequest) (res *signaturemanagement.SMContractRevokeResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("begin tx: %w", err))
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx,
		`UPDATE contract_signatures
		    SET status = 'REVOKED', revoked_at = $1
		  WHERE contract_did = $2 AND signer_did = $3 AND status != 'REVOKED'`,
		now, req.Did, req.SignerDid,
	)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("revoke signature: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("commit: %w", err))
	}

	// Emit RevokeEvent via the existing command handler.
	revoker := command.Revoker{DB: s.DB, CRepo: s.CRepo}
	if err := revoker.Handle(ctx, command.RevokeCmd{
		DID:       req.Did,
		RevokedBy: middleware.GetUsername(ctx),
	}); err != nil {
		log.Printf(ctx, "signatureManagement.revoke: emit event: %v", err)
	}

	return &signaturemanagement.SMContractRevokeResponse{Did: req.Did}, nil
}

func (s *signatureManagementsrvc) Audit(ctx context.Context, req *signaturemanagement.SMContractAuditRequest) (res []*signaturemanagement.SMContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.GetAuditLogQry{
		DID:       req.Did,
		AuditedBy: middleware.GetUsername(ctx),
	}
	handler := query.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	auditLogHistory, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	history := make([]*signaturemanagement.SMContractAuditResponse, 0)
	for _, entry := range auditLogHistory {
		history = append(history, &signaturemanagement.SMContractAuditResponse{
			ID:               entry.ID,
			Component:        entry.Component,
			EventType:        entry.EventType,
			EventData:        entry.EventData,
			Did:              entry.DID,
			CreatedAt:        entry.CreatedAt.String(),
			GlobalLogPredCid: entry.GlobalLogPredCID,
			ResLogPredCid:    entry.ResLogPredCID,
		})
	}

	return history, nil
}

func (s *signatureManagementsrvc) Compliance(ctx context.Context, req *signaturemanagement.SMContractComplianceRequest) (res *signaturemanagement.SMContractComplianceResponse, err error) {
	findings, err := s.collectComplianceFindings(ctx, req.Did)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	handler := command.ComplianceValidator{DB: s.DB, CRepo: s.CRepo}
	if err := handler.Handle(ctx, command.ComplianceCmd{
		DID:         req.Did,
		ValidatedBy: middleware.GetUsername(ctx),
	}); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	return &signaturemanagement.SMContractComplianceResponse{Did: req.Did, Findings: findings}, nil
}

type signatureRecord struct {
	SignerDID      string     `db:"signer_did"`
	CredentialType string     `db:"credential_type"`
	Status         string     `db:"status"`
	SignedAt       *time.Time `db:"signed_at"`
	RevokedAt      *time.Time `db:"revoked_at"`
}

func (s *signatureManagementsrvc) fetchSigningTasks(ctx context.Context) ([]*signaturemanagement.SMContractSigningTaskItem, error) {
	type taskRow struct {
		ContractDID     string     `db:"contract_did"`
		ContractVersion *int       `db:"contract_version"`
		SignerDID       string     `db:"signer_did"`
		CreatedAt       *time.Time `db:"created_at"`
	}

	rows := make([]taskRow, 0)
	err := s.DB.SelectContext(ctx, &rows,
		`SELECT cs.contract_did, c.contract_version, cs.signer_did, cs.created_at
		   FROM contract_signatures cs
		   JOIN contracts c ON c.did = cs.contract_did
		  WHERE c.state = 'APPROVED' AND cs.status = 'PENDING'
		  ORDER BY cs.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}

	tasks := make([]*signaturemanagement.SMContractSigningTaskItem, 0, len(rows))
	for _, r := range rows {
		created := time.Now().UTC().Format(time.RFC3339)
		if r.CreatedAt != nil {
			created = r.CreatedAt.UTC().Format(time.RFC3339)
		}
		tasks = append(tasks, &signaturemanagement.SMContractSigningTaskItem{
			Did:             r.ContractDID,
			ContractVersion: r.ContractVersion,
			State:           "PENDING",
			Reviewer:        r.SignerDID,
			CreatedAt:       created,
		})
	}

	return tasks, nil
}

func (s *signatureManagementsrvc) loadSignatures(ctx context.Context, did string) ([]signatureRecord, error) {
	records := make([]signatureRecord, 0)
	err := s.DB.SelectContext(ctx, &records,
		`SELECT signer_did, credential_type, status, signed_at, revoked_at
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at ASC`, did,
	)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *signatureManagementsrvc) collectValidationFindings(ctx context.Context, did string) ([]string, error) {
	records, err := s.loadSignatures(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	findings := make([]string, 0)
	if len(records) == 0 {
		findings = append(findings, "No signatures found for the contract")
	}

	active := 0
	for _, rec := range records {
		status := strings.ToUpper(strings.TrimSpace(rec.Status))
		switch status {
		case "SIGNED":
			active++
		case "PENDING":
			findings = append(findings, "Pending signature detected")
		case "REVOKED":
			findings = append(findings, "Revoked signature detected")
		default:
			findings = append(findings, fmt.Sprintf("Unknown signature status: %s", rec.Status))
		}
	}
	if active == 0 {
		findings = append(findings, "No active signatures available for validation")
	}

	pdfBytes, fetchErr := s.fetchContractPDFBytes(ctx, did)
	if fetchErr != nil {
		findings = append(findings, fmt.Sprintf("Could not fetch contract PDF for integrity check: %v", fetchErr))
	} else if len(pdfBytes) == 0 {
		findings = append(findings, "No contract PDF available for MR/HR integrity check")
	} else {
		contractVerifier := &verify.ContractVerifier{
			BuildFn: func(jsonld []byte) ([]byte, error) {
				return s.rebuildContractPDFFromJSONLD(ctx, did, jsonld)
			},
		}
		hashResult, verifyErr := contractVerifier.Verify(pdfBytes)
		if verifyErr != nil {
			findings = append(findings, fmt.Sprintf("Integrity check failed: %v", verifyErr))
		} else if !hashResult.Match {
			findings = append(findings, "Document integrity check failed")
		} else {
			findings = append(findings, "Document integrity check passed")
		}
	}

	if len(findings) == 0 {
		findings = append(findings, "Validation passed")
	}

	return findings, nil
}

func (s *signatureManagementsrvc) collectComplianceFindings(ctx context.Context, did string) ([]string, error) {
	records, err := s.loadSignatures(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	findings := make([]string, 0)
	if len(records) == 0 {
		findings = append(findings, "No signatures found for compliance evaluation")
		return findings, nil
	}

	active := 0
	for _, rec := range records {
		status := strings.ToUpper(strings.TrimSpace(rec.Status))
		cred := strings.ToUpper(strings.TrimSpace(rec.CredentialType))

		if status == "REVOKED" {
			findings = append(findings, fmt.Sprintf("Signer %s has a revoked signature", rec.SignerDID))
			continue
		}
		if status != "SIGNED" {
			findings = append(findings, fmt.Sprintf("Signer %s signature not finalized (status=%s)", rec.SignerDID, rec.Status))
			continue
		}

		active++
		switch cred {
		case "SES", "AES", "QES":
			// Supported compliance levels.
		case "STUB", "":
			findings = append(findings, fmt.Sprintf("Signer %s uses non-production credential type '%s'", rec.SignerDID, rec.CredentialType))
		default:
			findings = append(findings, fmt.Sprintf("Signer %s uses unknown credential type '%s'", rec.SignerDID, rec.CredentialType))
		}
	}

	if active == 0 {
		findings = append(findings, "No active signed credentials satisfy compliance checks")
	}

	if len(findings) == 0 {
		findings = append(findings, "Compliance checks passed")
	}

	return findings, nil
}

// fetchLatestEnvelope retrieves the most recent non-revoked signature record for did.
func (s *signatureManagementsrvc) fetchLatestEnvelope(ctx context.Context, did string) (*signaturemanagement.SMContractSignatureEnvelope, error) {
	type sigRow struct {
		SignerDID      string     `db:"signer_did"`
		CredentialType string     `db:"credential_type"`
		Status         string     `db:"status"`
		SignedAt       *time.Time `db:"signed_at"`
		RevokedAt      *time.Time `db:"revoked_at"`
		IpfsCID        *string    `db:"ipfs_cid"`
	}
	var row sigRow
	err := s.DB.GetContext(ctx, &row,
		`SELECT signer_did, credential_type, status, signed_at, revoked_at, ipfs_cid
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at DESC
		  LIMIT 1`,
		did,
	)
	if err != nil {
		return nil, err
	}
	env := &signaturemanagement.SMContractSignatureEnvelope{
		ContractDid:    did,
		SignerDid:      row.SignerDID,
		CredentialType: row.CredentialType,
		Status:         row.Status,
		IpfsCid:        row.IpfsCID,
	}
	if row.SignedAt != nil {
		t := row.SignedAt.Format(time.RFC3339)
		env.SignedAt = &t
	}
	if row.RevokedAt != nil {
		t := row.RevokedAt.Format(time.RFC3339)
		env.RevokedAt = &t
	}
	return env, nil
}

// fetchContractPDFBytes fetches the stored PDF bytes for a contract from IPFS.
func (s *signatureManagementsrvc) fetchContractPDFBytes(ctx context.Context, did string) ([]byte, error) {
	if s.IPFSClient == nil {
		return nil, nil
	}
	var cidStr string
	_ = s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid, '') FROM contracts WHERE did = $1`, did,
	).Scan(&cidStr)
	if cidStr == "" {
		return nil, nil
	}
	result, err := s.IPFSClient.FetchFile(cidStr)
	if err != nil {
		return nil, err
	}
	return []byte(result.Data), nil
}

// rebuildContractPDFFromJSONLD re-generates the base PDF from embedded JSON-LD bytes,
// fetching the contract metadata required for rendering.
func (s *signatureManagementsrvc) rebuildContractPDFFromJSONLD(ctx context.Context, did string, jsonld []byte) ([]byte, error) {
	type contractMeta struct {
		DID             string     `db:"did"`
		State           string     `db:"state"`
		ContractVersion int        `db:"contract_version"`
		Name            *string    `db:"name"`
		Description     *string    `db:"description"`
		CreatedBy       string     `db:"created_by"`
		CreatedAt       time.Time  `db:"created_at"`
		UpdatedAt       time.Time  `db:"updated_at"`
	}
	var meta contractMeta
	if err := s.DB.GetContext(ctx, &meta,
		`SELECT did, state, COALESCE(contract_version, 1) AS contract_version,
		        name, description, created_by, created_at, updated_at
		   FROM contracts WHERE did = $1`, did,
	); err != nil {
		return nil, fmt.Errorf("fetch contract meta: %w", err)
	}

	var rawJSON json.RawMessage
	_ = s.DB.QueryRowContext(ctx,
		`SELECT contract_data FROM contracts WHERE did = $1`, did,
	).Scan(&rawJSON)

	name := ""
	if meta.Name != nil {
		name = *meta.Name
	}
	desc := ""
	if meta.Description != nil {
		desc = *meta.Description
	}
	contractData := jsonld
	if len(rawJSON) > 0 {
		contractData = []byte(rawJSON)
	}

	return builder.BuildContract(builder.ContractInput{
		DID:          meta.DID,
		State:        meta.State,
		Version:      meta.ContractVersion,
		Name:         name,
		Description:  desc,
		CreatedBy:    meta.CreatedBy,
		CreatedAt:    meta.CreatedAt,
		UpdatedAt:    meta.UpdatedAt,
		ContractData: contractData,
	})
}
