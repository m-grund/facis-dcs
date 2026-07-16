// Package negotiationmerging folds the accepted change requests of a
// negotiation round into a new contract version (see MergeChangeRequests),
// triggered from command.Submitter once every negotiation task is closed.
// Conflicting changes to the same field/parameter are resolved by
// last-write-wins in persistence order — there is no explicit conflict
// detection between contradictory requests from different negotiators.
package negotiationmerging

import "encoding/json"

type ChangeRequest struct {
	Name            *string          `json:"name"`
	Description     *string          `json:"description"`
	ContractData    *json.RawMessage `json:"contract_data"`
	StartDate       *string          `json:"start_date"`
	ExpDate         *string          `json:"exp_date,omitempty"`
	ExpNoticePeriod *int             `json:"exp_notice_period,omitempty"`
	ExpPolicy       *string          `json:"exp_policy,omitempty"`
}

type ContractDataChange struct {
	SemanticConditionValues []SemanticConditionValue `json:"semanticConditionValues"`
}

type ContractData struct {
	DocumentBlocks          []DocumentBlock          `json:"documentBlocks"`
	DocumentOutline         []DocumentOutlineNode    `json:"documentOutline"`
	SemanticConditions      []SemanticCondition      `json:"semanticConditions"`
	SemanticRules           []SemanticRule           `json:"semanticRules"`
	ContractStatements      *ContractStatementSet    `json:"contractStatements,omitempty"`
	TemplateDataVersion     int                      `json:"templateDataVersion"`
	SubTemplateSnapshots    []SubTemplateSnapshot    `json:"subTemplateSnapshots"`
	SemanticConditionValues []SemanticConditionValue `json:"semanticConditionValues"`
}

type DocumentBlock struct {
	Text           string   `json:"text"`
	Type           string   `json:"type"`
	BlockID        string   `json:"blockId"`
	Title          *string  `json:"title,omitempty"`
	ConditionIDs   []string `json:"conditionIds,omitempty"`
	Version        int      `json:"version,omitempty"`
	TemplateID     string   `json:"templateId,omitempty"`
	DocumentNumber string   `json:"document_number,omitempty"`
}

type DocumentOutlineNode struct {
	IsRoot   bool     `json:"isRoot"`
	BlockID  string   `json:"blockId"`
	Children []string `json:"children"`
}

type SemanticCondition struct {
	Parameters    []ConditionParameter `json:"parameters"`
	ConditionID   string               `json:"conditionId"`
	ConditionName string               `json:"conditionName"`
	SchemaVersion string               `json:"schemaVersion"`
	EntityType    string               `json:"entityType,omitempty"`
	EntityRole    string               `json:"entityRole,omitempty"`
}

type ConditionParameter struct {
	Type          string          `json:"type"`
	Operators     []string        `json:"operators"`
	IsRequired    bool            `json:"isRequired"`
	ParameterName string          `json:"parameterName"`
	SchemaRef     string          `json:"schemaRef,omitempty"`
	SemanticPath  string          `json:"semanticPath,omitempty"`
	FixedValue    json.RawMessage `json:"fixedValue,omitempty"`
}

type SemanticConditionValue struct {
	ForField       string          `json:"forField"`
	BlockID        string          `json:"blockId"`
	ParameterValue json.RawMessage `json:"parameterValue"`
}

type SemanticRule struct {
	Type         string          `json:"@type,omitempty"`
	RuleID       string          `json:"ruleId"`
	LeftOperand  string          `json:"leftOperand"`
	Operator     string          `json:"operator"`
	RightOperand json.RawMessage `json:"rightOperand,omitempty"`
	Severity     string          `json:"severity,omitempty"`
	Source       string          `json:"source,omitempty"`
	Message      string          `json:"message,omitempty"`
}

type ContractStatementSet struct {
	Type       string              `json:"@type"`
	Statements []ContractStatement `json:"statements"`
}

type ContractStatement map[string]any

type SubTemplateSnapshot struct {
	DID          string       `json:"did"`
	Name         string       `json:"name"`
	Version      *int         `json:"version,omitempty"`
	Description  string       `json:"description"`
	TemplateData TemplateData `json:"template_data"`
}

type TemplateData struct {
	CustomMetaData     []CustomMetaData      `json:"customMetaData"`
	DocumentBlocks     []DocumentBlock       `json:"documentBlocks"`
	DocumentOutline    []DocumentOutlineNode `json:"documentOutline"`
	SemanticConditions []SemanticCondition   `json:"semanticConditions"`
}

type CustomMetaData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
