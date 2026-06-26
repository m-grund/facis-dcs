package service

import (
	"context"
	"errors"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"digital-contracting-service/internal/base"

	"digital-contracting-service/internal/contractworkflowengine/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"

	"github.com/jmoiron/sqlx"
)

type dcsToDcssrvc struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	CTRepo      db.ContractTemplateRepo
	DIDDocument base.DIDDocument
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo,
	didDocument base.DIDDocument) dcstodcs.Service {
	return &dcsToDcssrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		CTRepo:           ctRepo,
		DIDDocument:      didDocument,
	}
}

func (s *dcsToDcssrvc) Create(ctx context.Context, req *dcstodcs.DCSToDCSContractCreateRequest) (res *dcstodcs.DCSToDCSContractCreateResponse, err error) {

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contract := req.Contract

	if contract.Origin == origin {
		return nil, errors.New("could not create contract on same peer")
	}

	createAt, err := time.Parse(time.RFC3339, contract.CreatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	updatedAt, err := time.Parse(time.RFC3339, contract.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contractData, err := datatype.NewJSON(contract.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var startDate *time.Time
	if contract.StartDate != nil {
		startD, err := time.Parse(time.RFC3339, *contract.StartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		startDate = &startD
	}

	var expDate *time.Time
	if contract.ExpDate != nil {
		expD, err := time.Parse(time.RFC3339, *contract.ExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		expDate = &expD
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if contract.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*contract.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	responsible, err := db.ToResponsible(contract.Responsible)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	state, err := contractstate.NewContractState(contract.State)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	remoteContractData := command.RemoteContractData{
		DID:             contract.Did,
		ContractData:    &contractData,
		Origin:          contract.Origin,
		Responsible:     responsible,
		TemplateDID:     contract.Did,
		CreatedBy:       contract.CreatedBy,
		CreatedAt:       createAt,
		TemplateVersion: contract.ContractVersion,
		State:           state,
		ContractVersion: contract.ContractVersion,
		ExpPolicy:       expPolicy,
		ExpDate:         expDate,
		ExpNoticePeriod: contract.ExpNoticePeriod,
		StartDate:       startDate,
		Name:            contract.Name,
		Description:     contract.Description,
		UpdatedAt:       updatedAt,
	}

	cmd := command.RemoteCreateCmd{
		Contract: remoteContractData,
	}
	handler := command.RemoteCreator{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractCreateResponse{
		Did: req.Contract.Did,
	}, nil
}

func (s *dcsToDcssrvc) Update(ctx context.Context, req *dcstodcs.DCSToDCSContractUpdateRequest) (res *dcstodcs.DCSToDCSContractUpdateResponse, err error) {

	origin, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contract := req.Contract

	if contract.Origin == origin {
		return nil, errors.New("could not update contract on same peer")
	}

	createAt, err := time.Parse(time.RFC3339, contract.CreatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	updatedAt, err := time.Parse(time.RFC3339, contract.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contractData, err := datatype.NewJSON(contract.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var startDate *time.Time
	if contract.StartDate != nil {
		startD, err := time.Parse(time.RFC3339, *contract.StartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		startDate = &startD
	}

	var expDate *time.Time
	if contract.ExpDate != nil {
		expD, err := time.Parse(time.RFC3339, *contract.ExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		expDate = &expD
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if contract.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*contract.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	responsible, err := db.ToResponsible(contract.Responsible)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	state, err := contractstate.NewContractState(contract.State)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.RemoteUpdateCmd{
		DID:             contract.Did,
		ContractData:    &contractData,
		Origin:          contract.Origin,
		Responsible:     responsible,
		TemplateDID:     contract.Did,
		CreatedAt:       createAt,
		TemplateVersion: contract.ContractVersion,
		State:           state,
		ContractVersion: contract.ContractVersion,
		ExpPolicy:       expPolicy,
		ExpDate:         expDate,
		ExpNoticePeriod: contract.ExpNoticePeriod,
		StartDate:       startDate,
		Name:            contract.Name,
		Description:     contract.Description,
		UpdatedAt:       updatedAt,
		CreatedBy:       contract.CreatedBy,
	}
	handler := command.RemoteUpdater{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractUpdateResponse{
		Did: req.Contract.Did,
	}, nil

}

func (s *dcsToDcssrvc) Status(ctx context.Context, req *dcstodcs.DCSToDCSContractStatusRequest) (res *dcstodcs.DCSToDCSContractStatusResponse, err error) {
	return &dcstodcs.DCSToDCSContractStatusResponse{}, nil
}
