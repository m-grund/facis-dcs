package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"
)

type CreateCmd struct {
	DID         string             `json:"did"`
	TemplateDID string             `json:"template_did"`
	CreatedBy   string             `json:"created_by"`
	HolderDID   string             `json:"holder_did"`
	Reviewers   []string           `json:"reviewers"`
	Approvers   []string           `json:"approvers"`
	Negotiators []string           `json:"negotiators"`
	Parties     []ContractParty    `json:"parties"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

type ContractParty struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type Creator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	CTRepo      db.ContractTemplateRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	DIDDocument identity.DIDDocument
}

func createTasks(ctx context.Context, tx *sqlx.Tx, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo, ntRepo db.NegotiationTaskRepo, cmd CreateCmd) error {
	for _, reviewer := range cmd.Reviewers {
		reviewTask := db.ReviewTaskData{
			DID:       cmd.DID,
			Reviewer:  reviewer,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: cmd.CreatedBy,
		}
		_, err := rtRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			return fmt.Errorf("could not create review task: %w", err)
		}
	}

	for _, negotiator := range cmd.Negotiators {
		negotiationTask := db.NegotiationTaskData{
			DID:        cmd.DID,
			Negotiator: negotiator,
			State:      reviewtaskstate.Open.String(),
			CreatedBy:  cmd.CreatedBy,
		}
		_, err := ntRepo.Create(ctx, tx, negotiationTask)
		if err != nil {
			return fmt.Errorf("could not create negotiation task: %w", err)
		}
	}

	for _, approver := range cmd.Approvers {
		data := db.ApprovalTaskData{
			DID:       cmd.DID,
			CreatedBy: cmd.CreatedBy,
			Approver:  approver,
			State:     reviewtaskstate.Open.String(),
		}
		_, err := atRepo.Create(ctx, tx, data)
		if err != nil {
			return fmt.Errorf("could not create approval task: %w", err)
		}
	}

	return nil
}

// Handle has no entry in contractstate.Transitions: creation establishes the
// initial DRAFT state, it is not a transition from a prior state.
func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {

	if len(cmd.Reviewers) == 0 {
		return errors.New("no reviewers provided")
	}

	if len(cmd.Negotiators) == 0 {
		return errors.New("no negotiators provided")
	}

	if len(cmd.Approvers) == 0 {
		return errors.New("no approvers provided")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contractTemplate, err := h.CTRepo.ReadContractTemplateDataByID(ctx, tx, cmd.TemplateDID)
	if err != nil {
		return fmt.Errorf("could not read contract template data: %w", err)
	}

	contractDocument, err := contracttemplate.ConvertTemplateDataToContractData(contractTemplate.TemplateData, cmd.TemplateDID, contractTemplate.TemplateVersion)
	if err != nil {
		return fmt.Errorf("could not derive contract data from template: %w", err)
	}
	normalizedContractData, err := validation.NormalizeContractDataForPersistence(contractDocument, cmd.DID, false)
	if err != nil {
		return fmt.Errorf("contract data validation failed: %w", err)
	}
	// Parties are attached after normalization for the same reason renewal's
	// dcs:renewsContract is (see attachRenewsContractReference): the rebase
	// pass must not touch them. They gate party read-scoping in
	// query/contract/querybyid.go.
	if len(cmd.Parties) > 0 {
		normalizedContractData, err = attachContractParties(normalizedContractData, cmd.Parties)
		if err != nil {
			return fmt.Errorf("could not attach contract parties: %w", err)
		}
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return fmt.Errorf("could not get DID: %w", err)
	}

	// Reviewers/Approvers/Negotiators are peer DIDs (other DCS instances), not
	// individual users — task ownership is peer-scoped. Origin below marks
	// this node as the single writer for this contract (see package doc).
	resp := db.Responsible{
		Creator:     localPeer,
		Reviewers:   cmd.Reviewers,
		Approvers:   cmd.Approvers,
		Negotiators: cmd.Negotiators,
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          localPeer,
		CreatedBy:       cmd.CreatedBy,
		State:           contractstate.Draft.String(),
		ContractData:    normalizedContractData,
		TemplateDID:     cmd.TemplateDID,
		TemplateVersion: contractTemplate.TemplateVersion,
		Name:            contractTemplate.Name,
		Description:     contractTemplate.Description,
		Responsible:     &resp,
	}
	err = h.CRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract: %w", err)
	}

	err = createTasks(ctx, tx, h.RTRepo, h.ATRepo, h.NTRepo, cmd)
	if err != nil {
		return err
	}

	evt := contractevents.CreateEvent{
		DID:          cmd.DID,
		TemplateDID:  cmd.TemplateDID,
		CreatedBy:    cmd.CreatedBy,
		Name:         contractTemplate.Name,
		Description:  contractTemplate.Description,
		ContractData: normalizedContractData,
		OccurredAt:   data.CreatedAt,
		HolderDID:    cmd.HolderDID,
		UserRoles:    cmd.UserRoles,
		Responsible:  &resp,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// attachContractParties records the organizations that are parties to this
// contract as typed dcs:CompanyParty nodes under "dcs:parties". The legal
// name (the same value the OID4VP organization claim discloses) gates read
// access in query/contract/querybyid.go. A party carrying a role is bound
// to the role-derived node the contract's ODRL rules reference as
// odrl:assigner/odrl:assignee, so rule parties resolve to the named
// organization.
func attachContractParties(raw *datatype.JSON, parties []ContractParty) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	nodes, _ := doc["dcs:parties"].([]any)
	for index, party := range parties {
		if party.Role != "" {
			if node := partyNodeByRoleFragment(nodes, party.Role); node != nil {
				node["dcs:legalName"] = party.Name
				continue
			}
		}
		fragment := fmt.Sprintf("party-%d", index)
		if party.Role != "" {
			fragment = "party-" + party.Role
		}
		node := map[string]any{
			"@id":           fmt.Sprintf("%s#%s", doc["@id"], fragment),
			"@type":         "dcs:CompanyParty",
			"dcs:legalName": party.Name,
		}
		if party.Role != "" {
			node["dcs:role"] = party.Role
		}
		nodes = append(nodes, node)
	}
	doc["dcs:parties"] = nodes
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

func partyNodeByRoleFragment(nodes []any, role string) map[string]any {
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if iri, _ := node["@id"].(string); strings.HasSuffix(iri, "#party-"+role) {
			return node
		}
	}
	return nil
}
