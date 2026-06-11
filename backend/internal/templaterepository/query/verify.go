package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"digital-contracting-service/internal/base/validation"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"digital-contracting-service/internal/templaterepository/selfdescription"

	"github.com/jmoiron/sqlx"
)

type VerifyQry struct {
	DID           string
	VerifiedBy    string
	ParticipantID string
	Token         string
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type VerifyResult struct {
	Findings []string
}

type Verifier struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Verifier) Handle(ctx context.Context, cmd VerifyQry) (*VerifyResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	fullTemplate, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read template data: %w", err)
	}
	if _, err := validation.NormalizeTemplateData(fullTemplate.TemplateData); err != nil {
		return nil, fmt.Errorf("template data validation failed: %w", err)
	}

	if h.FCClient != nil {
		findings, err := h.verifyTemplateResourceSelfDescription(ctx, cmd, processData, fullTemplate)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{
			Findings: findings,
		}, nil
	}

	hasTask, err := h.RTRepo.TaskExistsInState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Open.String())
	if err != nil {
		return nil, err
	}

	if hasTask {
		err := h.RTRepo.UpdateState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Verified.String())
		if err != nil {
			return nil, err
		}
	}

	evt := templateevents.VerifyEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		VerifiedBy:     cmd.VerifiedBy,
		OccurredAt:     time.Now().UTC(),
		HolderDID:      cmd.HolderDID,
		UserRoles:      cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &VerifyResult{}, nil
}

func (h *Verifier) verifyTemplateResourceSelfDescription(ctx context.Context, cmd VerifyQry, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) ([]string, error) {
	if h.FCClient == nil {
		return nil, fcclient.ErrFederatedCatalogueNotConfigured
	}

	if processData == nil {
		return nil, fmt.Errorf("process data is nil")
	}
	if fullTemplate == nil {
		return nil, fmt.Errorf("full template is nil")
	}

	findings := []string{}
	if cmd.ParticipantID == "" {
		findings = append(findings, "participantID is empty")
	}

	documentNumber := ""
	if processData.DocumentNumber != nil {
		documentNumber = *processData.DocumentNumber
		if documentNumber == "" {
			findings = append(findings, "documentNumber is empty")
		}
	}

	templateType := fullTemplate.TemplateType
	name := ""
	description := ""
	if fullTemplate.Name != nil {
		name = *fullTemplate.Name
		if name == "" {
			findings = append(findings, "name is empty")
		}
	}
	if fullTemplate.Description != nil {
		description = *fullTemplate.Description
		if description == "" {
			findings = append(findings, "description is empty")
		}
	}

	sd := selfdescription.BuildTemplateResourceSelfDescription(selfdescription.TemplateResourceInput{
		ParticipantID:  cmd.ParticipantID,
		DID:            cmd.DID,
		DocumentNumber: documentNumber,
		Version:        processData.Version,
		TemplateType:   templateType,
		Name:           name,
		Description:    description,
		CreatedAt:      fullTemplate.CreatedAt,
		UpdatedAt:      fullTemplate.UpdatedAt,
		TemplateData:   fullTemplate.TemplateData,
	})

	body, err := json.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("marshal template resource self-description failed: %w", err)
	}

	query := url.Values{}
	query.Set("verifySemantics", "true")
	query.Set("verifySchema", "true")
	query.Set("verifySignatures", "true")
	query.Set("verifyVPSignature", "false")
	query.Set("verifyVCSignature", "false")

	resp, err := h.FCClient.Post(ctx, fcclient.VerificationEndpointPath, query, body)
	if err != nil {
		return nil, fmt.Errorf("verify template resource self-description failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if message := h.FCClient.ExtractErrorMessage(resp.Body); message != "" {
			msg := fmt.Errorf("verify template resource self-description failed: %s", message)
			findings = append(findings, msg.Error())
		} else {
			msg := fmt.Errorf("verify template resource self-description failed with status %d", resp.StatusCode)
			findings = append(findings, msg.Error())
		}
	}

	return findings, nil
}
