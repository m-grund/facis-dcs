package service

import (
	"context"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/query"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type processAuditAndCompliancesrvc struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
	auth.JWTAuthenticator
}

func NewProcessAuditAndCompliance(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, auditTrailReader base.AuditTrailReader) processauditandcompliance.Service {
	return &processAuditAndCompliancesrvc{DB: db, JWTAuthenticator: jwtAuth, ATrailReader: auditTrailReader}
}

func (s *processAuditAndCompliancesrvc) Audit(ctx context.Context, req *processauditandcompliance.PACAuditRequest) (res []*processauditandcompliance.PACAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	scope, err := componenttype.NewComponentType(req.Scope)
	if err != nil {
		return nil, err
	}

	qry := query.GetAuditLogQry{
		Scope:     scope,
		AuditedBy: middleware.GetUsername(ctx),
	}
	handler := query.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	resLogHistories, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	result := make([]*processauditandcompliance.PACAuditResponse, 0)
	for _, resLog := range resLogHistories {

		var did string
		history := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0)
		for _, entry := range resLog {

			if entry.DID != nil {
				did = *entry.DID
			}

			history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
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

		result = append(result, &processauditandcompliance.PACAuditResponse{
			Component: req.Scope,
			Did:       did,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}

	return result, nil
}

func (s *processAuditAndCompliancesrvc) AuditReport(ctx context.Context, p *processauditandcompliance.AuditReportPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.audit_report")
	return
}

func (s *processAuditAndCompliancesrvc) Monitor(ctx context.Context, p *processauditandcompliance.MonitorPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.monitor")
	return
}

func (s *processAuditAndCompliancesrvc) IncidentReport(ctx context.Context, p *processauditandcompliance.IncidentReportPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.incident_report")
	return
}
