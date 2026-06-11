package service

import (
	"context"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/ipfs"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	"digital-contracting-service/internal/signingmanagement/query"

	"github.com/jmoiron/sqlx"
)

type signatureManagementsrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	ATrailReader base.AuditTrailReader
	DSSClient    dss.Client
	IPFSClient   *ipfs.APIClient
	auth.JWTAuthenticator
}

func NewSignatureManagement(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo,
	auditTrailReader base.AuditTrailReader, dssClient dss.Client, ipfsClient *ipfs.APIClient) signaturemanagement.Service {

	return &signatureManagementsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		ATrailReader:     auditTrailReader,
		DSSClient:        dssClient,
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

	var startDate *string
	if result.Contract.StartDate != nil {
		s := result.Contract.StartDate.Format(time.RFC3339)
		startDate = &s
	}

	var expDate *string
	if result.Contract.ExpDate != nil {
		s := result.Contract.ExpDate.Format(time.RFC3339)
		expDate = &s
	}

	var expPolicy *string
	if result.Contract.ExpPolicy != nil {
		s := result.Contract.ExpPolicy.String()
		expPolicy = &s
	}

	contract := signaturemanagement.SMContractItem{
		Did:             result.Contract.DID,
		ContractVersion: result.Contract.ContractVersion,
		State:           result.Contract.State.String(),
		Name:            result.Contract.Name,
		Description:     result.Contract.Description,
		CreatedAt:       result.Contract.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       result.Contract.UpdatedAt.Format(time.RFC3339),
		Responsible:     result.Contract.Responsible,
		ContractData:    result.Contract.ContractData,
		ExpNoticePeriod: result.Contract.ExpNoticePeriod,
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		CreatedBy:       result.Contract.CreatedBy,
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

	return &signaturemanagement.SMContractRetrieveByIDResponse{
		Contract:          &contract,
		SignatureEnvelope: signatureEnvelop,
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
		DB:    s.DB,
		CRepo: s.CRepo,
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

	cmd := command.ApplyCmd{
		DID:       req.Did,
		AppliedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	handler := command.Applier{
		DB:       s.DB,
		CRepo:    s.CRepo,
		DSClient: s.DSSClient,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
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
		DB:    s.DB,
		CRepo: s.CRepo,
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
