package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/ipfs"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/query"
	"digital-contracting-service/internal/signingmanagement/signer"

	"github.com/jmoiron/sqlx"
)

// mapSignatureCommandError classifies a signing command error for the HTTP
// layer, mirroring service.mapContractCommandError: a contractstate.
// ErrInvalidTransition (e.g. attempting to sign a contract that isn't
// APPROVED) is a client error (400), everything else stays an internal
// error (500).
func mapSignatureCommandError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, command.ErrCeremonyRequired) {
		return signaturemanagement.MakeCeremonyRequired(err)
	}
	if errors.Is(err, contractstate.ErrInvalidTransition) {
		return signaturemanagement.MakeBadRequest(err)
	}
	return signaturemanagement.MakeInternalError(err)
}

type signatureManagementsrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	CeremonyRepo db.CeremonyRepo
	PDFCore      *pdfcore.Client
	ATrailReader base.AuditTrailReader
	Signer       signer.ContractSigner
	VCSigner     provenance.VCSigner
	IssuerDID    string
	IPFSClient   *ipfs.APIClient
	DIDDocument  identity.DIDDocument
	auth.JWTAuthenticator
}

func NewSignatureManagement(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, ceremonyRepo db.CeremonyRepo,
	auditTrailReader base.AuditTrailReader, contractSigner signer.ContractSigner, vcSigner provenance.VCSigner, issuerDID string,
	ipfsClient *ipfs.APIClient, pdfCore *pdfcore.Client) signaturemanagement.Service {

	return &signatureManagementsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		CeremonyRepo:     ceremonyRepo,
		PDFCore:          pdfCore,
		ATrailReader:     auditTrailReader,
		Signer:           contractSigner,
		VCSigner:         vcSigner,
		IssuerDID:        issuerDID,
		IPFSClient:       ipfsClient,
	}
}

func (s *signatureManagementsrvc) Retrieve(ctx context.Context, req *signaturemanagement.SMContractRetrieveRequest) (res *signaturemanagement.SMContractRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	pagination := datatype.Pagination{
		Offset: 0, // DerefInt(req.Offset),
		Limit:  0, // DerefInt(req.Limit),
	}

	qry := query.GetAllMetadataQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		Pagination:  pagination,
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

		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &signaturemanagement.SMContractListItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
		})
	}

	var signingTasks []*signaturemanagement.SMContractSigningTaskItem
	for _, item := range result.SigningTasks {
		signingTasks = append(signingTasks, &signaturemanagement.SMContractSigningTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Signer:          item.SignerDID,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	return &signaturemanagement.SMContractRetrieveResponse{
		Contracts:    contracts,
		SigningTasks: signingTasks,
	}, nil
}

func (s *signatureManagementsrvc) RetrieveByID(ctx context.Context, req *signaturemanagement.SMContractRetrieveByIDRequest) (res *signaturemanagement.SMContractRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.GetByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	contract := signaturemanagement.SMContractItem{
		Did:             result.Contract.DID,
		ContractVersion: result.Contract.ContractVersion,
		State:           result.Contract.State.String(),
		Name:            result.Contract.Name,
		Description:     result.Contract.Description,
		CreatedAt:       result.Contract.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       result.Contract.UpdatedAt.Format(time.RFC3339),
	}

	signatureEnvelop := &signaturemanagement.SMContractSignatureEnvelope{
		ContractDid:    result.SignatureEnvelope.ContractDID,
		CredentialType: result.SignatureEnvelope.CredentialType,
		IpfsCid:        result.SignatureEnvelope.IpfsCID,
		RevokedAt:      result.SignatureEnvelope.RevokedAt,
		SignedAt:       result.SignatureEnvelope.SignedAt,
		SignerDid:      result.SignatureEnvelope.SignerDID,
		Status:         result.SignatureEnvelope.Status.String(),
	}

	keyVersion := result.SignatureEnvelope.KeyVersion

	return &signaturemanagement.SMContractRetrieveByIDResponse{
		Contract:          &contract,
		SignatureEnvelope: signatureEnvelop,
		KeyVersion:        &keyVersion,
	}, nil
}

func (s *signatureManagementsrvc) Verify(ctx context.Context, req *signaturemanagement.SMContractVerifyRequest) (res *signaturemanagement.SMContractVerifyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.SignatureVerifyQry{
		DID:        req.Did,
		VerifiedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := query.SignatureVerifier{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}
	_, err = handler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	return &signaturemanagement.SMContractVerifyResponse{}, nil
}

func (s *signatureManagementsrvc) Apply(ctx context.Context, req *signaturemanagement.SMContractApplyRequest) (res *signaturemanagement.SMContractApplyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	credentialType := req.CredentialType
	if credentialType == nil || *credentialType == "" {
		aes := "AES"
		credentialType = &aes
	}
	cmd := command.ApplyCmd{
		DID:            req.Did,
		SignerDID:      req.SignerDid,
		CredentialType: *credentialType,
		AppliedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
	}
	handler := command.Applier{
		DB:           s.DB,
		CRepo:        s.CRepo,
		CeremonyRepo: s.CeremonyRepo,
		Signer:       s.Signer,
		PDFCore:      s.PDFCore,
		IPFSClient:   s.IPFSClient,
		VCSigner:     s.VCSigner,
		IssuerDID:    s.IssuerDID,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}

	qry := query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.GetByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	signatureEnvelop := &signaturemanagement.SMContractSignatureEnvelope{
		ContractDid:    result.SignatureEnvelope.ContractDID,
		CredentialType: result.SignatureEnvelope.CredentialType,
		IpfsCid:        result.SignatureEnvelope.IpfsCID,
		RevokedAt:      result.SignatureEnvelope.RevokedAt,
		SignedAt:       result.SignatureEnvelope.SignedAt,
		SignerDid:      result.SignatureEnvelope.SignerDID,
		Status:         result.SignatureEnvelope.Status.String(),
	}

	return &signaturemanagement.SMContractApplyResponse{
		Did:               req.Did,
		SignatureEnvelope: signatureEnvelop,
	}, nil
}

func (s *signatureManagementsrvc) Validate(ctx context.Context, req *signaturemanagement.SMContractValidateRequest) (res *signaturemanagement.SMContractValidateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.ValidateQry{
		DID:         req.Did,
		ValidatedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.Validator{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractValidateResponse{
		Did:      req.Did,
		Findings: result.Findings,
	}, nil
}

func (s *signatureManagementsrvc) Revoke(ctx context.Context, req *signaturemanagement.SMContractRevokeRequest) (res *signaturemanagement.SMContractRevokeResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := command.RevokeCmd{
		DID:       req.Did,
		RevokedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	queryHandler := command.Revoker{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	err = queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractRevokeResponse{}, nil
}

func (s *signatureManagementsrvc) Audit(ctx context.Context, req *signaturemanagement.SMContractAuditRequest) (res []*signaturemanagement.SMContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.GetAuditLogQry{
		DID:       req.Did,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
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

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := command.ComplianceCmd{
		DID:       req.Did,
		CheckedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	queryHandler := command.ComplianceValidator{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	err = queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractComplianceResponse{}, nil
}

func (s *signatureManagementsrvc) StartCeremony(ctx context.Context, req *signaturemanagement.SMSignatureRequestStartRequest) (res *signaturemanagement.SMSignatureRequestStartResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := command.StartCeremonyHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, command.StartCeremonyCmd{
		ContractDID: req.ContractDid,
		FieldName:   req.FieldName,
		RequestedBy: middleware.GetParticipantID(ctx),
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	walletURI := ""
	if ceremony.WalletURI != nil {
		walletURI = *ceremony.WalletURI
	}
	return &signaturemanagement.SMSignatureRequestStartResponse{
		CeremonyID: ceremony.ID,
		WalletURI:  walletURI,
		ExpiresAt:  ceremony.ExpiresAt.Format(time.RFC3339),
		Status:     ceremony.Status,
	}, nil
}

func (s *signatureManagementsrvc) CeremonyStatus(ctx context.Context, req *signaturemanagement.SMSignatureRequestStatusRequest) (res *signaturemanagement.SMSignatureRequestStatusResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := query.CeremonyStatusHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, query.CeremonyStatusQry{CeremonyID: req.CeremonyID})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", req.CeremonyID))
	}

	res = &signaturemanagement.SMSignatureRequestStatusResponse{
		CeremonyID: ceremony.ID,
		Status:     ceremony.Status,
	}
	res.ContractDid = &ceremony.ContractDID
	res.FieldName = &ceremony.FieldName
	res.SignerDid = ceremony.SignerDID
	expiresAt := ceremony.ExpiresAt.Format(time.RFC3339)
	res.ExpiresAt = &expiresAt
	return res, nil
}

func (s *signatureManagementsrvc) CeremonyWebhook(ctx context.Context, req *signaturemanagement.SMSignatureWebhookRequest) (res *signaturemanagement.SMSignatureWebhookResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	secret := ""
	if req.WebhookSecret != nil {
		secret = *req.WebhookSecret
	}
	handler := command.WebhookHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, command.WebhookCmd{
		Secret:     secret,
		CeremonyID: req.CeremonyID,
		VpToken:    req.VpToken,
		PidClaims:  req.PidClaims,
	})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrWebhookUnauthorized):
			return nil, signaturemanagement.MakeUnauthorized(err)
		case errors.Is(err, command.ErrCeremonyNotFound):
			return nil, signaturemanagement.MakeNotFound(err)
		default:
			return nil, signaturemanagement.MakeInternalError(err)
		}
	}

	return &signaturemanagement.SMSignatureWebhookResponse{
		CeremonyID: ceremony.ID,
		Status:     ceremony.Status,
	}, nil
}
