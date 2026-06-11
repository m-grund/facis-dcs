package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/pdfgeneration/builder"
	"digital-contracting-service/internal/pdfgeneration/c2pa"
	"digital-contracting-service/internal/pdfgeneration/verify"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/signingmanagement/db"
)

type PostgresContractRepo struct {
	IPFSClient *ipfs.APIClient
}

func (r *PostgresContractRepo) ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.Contract, error) {
	query := `
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date, exp_date, exp_policy, exp_notice_period, responsible
        FROM contracts
        WHERE did = $1
         AND state IN ('APPROVED', 'SIGNED')
    `
	var ct db.Contract
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible
        FROM contracts
        WHERE state IN ('APPROVED', 'SIGNED')
    `

	var params []any
	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		query += ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, state, updated_at, created_by, contract_version
        FROM contracts
        WHERE did = $1
         AND  state = 'APPROVED'
    `
	var processData db.ContractProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("could not found contract with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contracts SET state = $2
        WHERE did = $1
         AND  state = 'APPROVED'
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

// ---------------------------------------------------------------------------------------------------------------------

func (r *PostgresContractRepo) CreateSignature(ctx context.Context, tx *sqlx.Tx, signature db.ContractSignature) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO contract_signatures
			(contract_did, signer_did, credential_type, signature_bytes, status)
		VALUES ($1, $2, $3, $4, $5)`,
		signature.ContractDID, signature.SignerDID, signature.CredentialType, signature.SignatureBytes, signature.Status,
	)
	if err != nil {
		return fmt.Errorf("could not create contract signature: %w", err)
	}

	return nil
}

func (r *PostgresContractRepo) RevokeSignature(ctx context.Context, tx *sqlx.Tx, did string, signerDID string) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx,
		`UPDATE contract_signatures
		    SET status = 'REVOKED', revoked_at = $1
		  WHERE contract_did = $2 AND signer_did = $3 AND status != 'REVOKED'`,
		now, did, signerDID,
	)
	if err != nil {
		return err
	}
	return nil
}

// ReadLatestEnvelopeByContractDID retrieves the most recent non-revoked signature record for did.
func (r *PostgresContractRepo) ReadLatestEnvelopeByContractDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractSignatureEnvelope, error) {
	var signature db.ContractSignature
	err := tx.GetContext(ctx, &signature,
		`SELECT contract_did, signer_did, credential_type, status, signed_at, revoked_at, ipfs_cid
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at DESC
		  LIMIT 1`,
		did,
	)
	if err != nil {
		return nil, err
	}
	env := &db.ContractSignatureEnvelope{
		ContractDID:    did,
		SignerDID:      signature.SignerDID,
		CredentialType: signature.CredentialType,
		Status:         signature.Status,
		IpfsCID:        signature.IpfsCID,
	}
	if signature.SignedAt != nil {
		t := signature.SignedAt.Format(time.RFC3339)
		env.SignedAt = &t
	}
	if signature.RevokedAt != nil {
		t := signature.RevokedAt.Format(time.RFC3339)
		env.RevokedAt = &t
	}
	return env, nil
}

func (r *PostgresContractRepo) ReadAllSigningTasks(ctx context.Context, tx *sqlx.Tx) ([]db.ContractSigningTask, error) {

	var tasks []db.ContractSigningTask
	err := tx.SelectContext(ctx, &tasks,
		`SELECT cs.contract_did, c.contract_version, cs.signer_did, cs.created_at
		   FROM contract_signatures cs
		   JOIN contracts c ON c.did = cs.contract_did
		  WHERE c.state = 'APPROVED' AND cs.status = 'PENDING'
		  ORDER BY cs.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresContractRepo) CountSignatureForContractDID(ctx context.Context, tx *sqlx.Tx, did string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contract_signatures WHERE contract_did=$1 AND status != 'REVOKED'`,
		did,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count signatures: %w", err)
	}
	return count, nil
}

// FetchContractPDFBytes fetches the stored PDF bytes for a contract from IPFS.
func (r *PostgresContractRepo) FetchContractPDFBytes(ctx context.Context, tx *sqlx.Tx, did string) ([]byte, error) {
	var cidStr string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid, '') FROM contracts WHERE did = $1`, did,
	).Scan(&cidStr)
	if cidStr == "" {
		return nil, nil
	}
	result, err := r.IPFSClient.FetchFile(cidStr)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// RebuildContractPDFFromJSONLD re-generates the base PDF from embedded JSON-LD bytes,
// fetching the contract metadata required for rendering.
func (r *PostgresContractRepo) RebuildContractPDFFromJSONLD(ctx context.Context, tx *sqlx.Tx, did string, jsonld []byte) ([]byte, error) {

	type contractMeta struct {
		DID             string    `db:"did"`
		State           string    `db:"state"`
		ContractVersion int       `db:"contract_version"`
		Name            *string   `db:"name"`
		Description     *string   `db:"description"`
		CreatedBy       string    `db:"created_by"`
		CreatedAt       time.Time `db:"created_at"`
		UpdatedAt       time.Time `db:"updated_at"`
	}
	var meta contractMeta
	if err := tx.GetContext(ctx, &meta,
		`SELECT did, state, COALESCE(contract_version, 1) AS contract_version,
		        name, description, created_by, created_at, updated_at
		   FROM contracts WHERE did = $1`, did,
	); err != nil {
		return nil, fmt.Errorf("fetch contract meta: %w", err)
	}

	var rawJSON json.RawMessage
	_ = tx.QueryRowContext(ctx,
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
		contractData = rawJSON
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

// ContractIPFSFetchFn returns a verifier FetchFn that retrieves the canonical
// contract PDF from IPFS.
func (r *PostgresContractRepo) ContractIPFSFetchFn(ctx context.Context, tx *sqlx.Tx, did string) func() ([]byte, error) {
	return func() ([]byte, error) {
		return r.FetchContractPDFBytes(ctx, tx, did)
	}
}

// ContractManifestIPFSFetchFn returns a verifier FetchManifestFn that retrieves
// the standalone remote manifest bytes for the given contract.
func (r *PostgresContractRepo) ContractManifestIPFSFetchFn(ctx context.Context, tx *sqlx.Tx, did string) func() ([]byte, error) {
	return func() ([]byte, error) {
		var cidStr string
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(pdf_manifest_ipfs_cid,'') FROM contracts WHERE did = $1`, did,
		).Scan(&cidStr); err != nil || cidStr == "" {
			return nil, nil
		}
		result, err := r.IPFSClient.FetchFile(cidStr)
		if err != nil {
			return nil, err
		}
		return []byte(result.Data), nil
	}
}

func (r *PostgresContractRepo) StatusListCheckFn(ctx context.Context, tx *sqlx.Tx) func(string, uint32) (string, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return func(statusListCredential string, index uint32) (string, error) {
		return c2pa.QueryStatusListStatus(ctx, httpClient, statusListCredential, index)
	}
}

func (r *PostgresContractRepo) CollectValidationFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	records, err := r.LoadSignatures(ctx, tx, did)
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

	pdfBytes, fetchErr := r.FetchContractPDFBytes(ctx, tx, did)
	if fetchErr != nil {
		findings = append(findings, fmt.Sprintf("Could not fetch contract PDF for integrity check: %v", fetchErr))
	} else if len(pdfBytes) == 0 {
		findings = append(findings, "No contract PDF available for MR/HR integrity check")
	} else {
		contractVerifier := &verify.ContractVerifier{
			BuildFn: func(jsonld []byte) ([]byte, error) {
				return r.RebuildContractPDFFromJSONLD(ctx, tx, did, jsonld)
			},
			FetchFn:         r.ContractIPFSFetchFn(ctx, tx, did),
			FetchManifestFn: r.ContractManifestIPFSFetchFn(ctx, tx, did),
			CheckStatusFn:   r.StatusListCheckFn(ctx, tx),
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

func (r *PostgresContractRepo) LoadSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]db.SignatureRecord, error) {
	var records []db.SignatureRecord
	err := tx.SelectContext(ctx, &records,
		`SELECT signer_did, credential_type, status, signed_at, revoked_at
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at`, did,
	)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PostgresContractRepo) CollectComplianceFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	records, err := r.LoadSignatures(ctx, tx, did)
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
