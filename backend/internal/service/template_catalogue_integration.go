package service

import (
	"context"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
)

type templateCatalogueIntegrationsrvc struct {
	auth.JWTAuthenticator
	fcClient *fcclient.FederatedCatalogueClient
}

func NewTemplateCatalogueIntegration(jwtAuth auth.JWTAuthenticator, fcClient *fcclient.FederatedCatalogueClient) templatecatalogueintegration.Service {
	return &templateCatalogueIntegrationsrvc{JWTAuthenticator: jwtAuth, fcClient: fcClient}
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	queryHandler := templatequery.GetAllMetadataHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(templatequery.GetAllMetadataQry{
		Offset: req.Offset,
		Limit:  req.Limit,
	})

	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplateByID(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveByIDRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, err error) {
	queryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(templatequery.GetByIDQry{
		DID:     req.Did,
		Version: req.Version,
	})

	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	if result == nil {
		return nil, nil
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) SearchTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueSearchRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	queryHandler := templatequery.SearchHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	qry := templatequery.SearchQry{
		DID:            base.DerefString(req.Did),
		DocumentNumber: base.DerefString(req.DocumentNumber),
		Version:        base.DerefInt(req.Version),
		Name:           base.DerefString(req.Name),
		Description:    base.DerefString(req.Description),
		Offset:         req.Offset,
		Limit:          req.Limit,
	}

	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	return result, nil
}
