// Package remotesync holds the plain data types transferred between DCS
// peers during synchronization (mirroring the db package's row types, but
// decoupled from SQL so they can cross the wire), plus conversion helpers
// (ToReviewTaskData, etc.) used by remotesync/command's LocalPeerUpdater to
// persist an incoming PostSync snapshot. See dcstodcs for the HTTP transport
// and trust checks, and remoteaction for the outbound RPC call itself.
package remotesync

import (
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	negotiationdescision "digital-contracting-service/internal/contractworkflowengine/datatype/negotiationaction"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

type ContractData struct {
	DID             string
	Origin          string
	ContractVersion int
	State           contractstate.ContractState
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Name            *string
	Description     *string
	Responsible     *db.Responsible
	ContractData    *datatype.JSON
	TemplateDID     string
	TemplateVersion int
}

type NegotiationTaskData struct {
	ID         string
	DID        string
	State      negotiationtaskstate.NegotiationTaskState
	Negotiator string
	CreatedBy  string
	CreatedAt  time.Time
}

type NegotiationData struct {
	ID              string
	DID             string
	ContractVersion int
	ChangeRequest   *datatype.JSON
	CreatedBy       string
	CreatedAt       time.Time
}

type NegotiationDecisionData struct {
	ID              string
	NegotiationID   string
	Negotiator      string
	Decision        *negotiationdescision.NegotiationDecision
	RejectionReason *string
}

type ApprovalTaskData struct {
	ID        string
	DID       string
	State     string
	Approver  string
	CreatedBy string
	CreatedAt time.Time
}

type ReviewTaskData struct {
	ID        string
	DID       string
	State     string
	Reviewer  string
	CreatedBy string
	CreatedAt time.Time
}

func ToReviewTaskData(tasks []ReviewTaskData) []db.ReviewTaskData {
	var reviewTasks []db.ReviewTaskData
	for _, task := range tasks {
		reviewTasks = append(reviewTasks, db.ReviewTaskData{
			ID:        task.ID,
			DID:       task.DID,
			CreatedBy: task.CreatedBy,
			CreatedAt: task.CreatedAt,
			State:     task.State,
			Reviewer:  task.Reviewer,
		})
	}
	return reviewTasks
}

func ToApprovalTaskData(tasks []ApprovalTaskData) []db.ApprovalTaskData {
	var approvalTasks []db.ApprovalTaskData
	for _, task := range tasks {
		approvalTasks = append(approvalTasks, db.ApprovalTaskData{
			ID:        task.ID,
			DID:       task.DID,
			CreatedBy: task.CreatedBy,
			CreatedAt: task.CreatedAt,
			State:     task.State,
			Approver:  task.Approver,
		})
	}
	return approvalTasks
}

func ToNegotiationTaskData(tasks []NegotiationTaskData) []db.NegotiationTaskData {
	var negotiationTasks []db.NegotiationTaskData
	for _, task := range tasks {
		negotiationTasks = append(negotiationTasks, db.NegotiationTaskData{
			ID:         task.ID,
			DID:        task.DID,
			CreatedBy:  task.CreatedBy,
			CreatedAt:  task.CreatedAt,
			State:      task.State.String(),
			Negotiator: task.Negotiator,
		})
	}
	return negotiationTasks
}

func ToNegotiationData(tasks []NegotiationData) []db.NegotiationData {
	var negotiations []db.NegotiationData
	for _, task := range tasks {
		negotiations = append(negotiations, db.NegotiationData{
			ID:              task.ID,
			DID:             task.DID,
			CreatedBy:       task.CreatedBy,
			CreatedAt:       task.CreatedAt,
			ContractVersion: task.ContractVersion,
			ChangeRequest:   task.ChangeRequest,
		})
	}
	return negotiations
}

func ToNegotiationDecisionData(tasks []NegotiationDecisionData) []db.NegotiationDecisionData {
	var negotiationDecisions []db.NegotiationDecisionData
	for _, task := range tasks {

		var decision *string
		if task.Decision != nil {
			tmp := task.Decision.String()
			decision = &tmp
		}

		negotiationDecisions = append(negotiationDecisions, db.NegotiationDecisionData{
			ID:              task.ID,
			RejectionReason: task.RejectionReason,
			Decision:        decision,
			Negotiator:      task.Negotiator,
			NegotiationID:   task.NegotiationID,
		})
	}
	return negotiationDecisions
}
