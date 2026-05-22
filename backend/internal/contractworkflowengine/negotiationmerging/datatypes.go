package negotiationmerging

import "encoding/json"

type ChangeRequest struct {
	Name            *string       `json:"name"`
	Description     *string       `json:"description"`
	ContractData    *ContractData `json:"contract_data"`
	StartDate       *string       `json:"start_date"`
	ExpDate         *string       `json:"exp_date,omitempty"`
	ExpNoticePeriod *int          `json:"exp_notice_period,omitempty"`
	ExpPolicy       *string       `json:"exp_policy,omitempty"`
}

type ContractData struct {
	DocumentBlocks          []DocumentBlock          `json:"documentBlocks"`
	DocumentOutline         []DocumentOutlineNode    `json:"documentOutline"`
	SemanticConditions      []SemanticCondition      `json:"semanticConditions"`
	TemplateDataVersion     int                      `json:"templateDataVersion"`
	SubTemplateSnapshots    []SubTemplateSnapshot    `json:"subTemplateSnapshots"`
	SemanticConditionValues []SemanticConditionValue `json:"semanticConditionValues"`
}

type DocumentBlock struct {
	Text           string   `json:"text"`
	Type           string   `json:"type"`
	BlockID        string   `json:"blockId"`
	Title          string   `json:"title,omitempty"`
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
}

type ConditionParameter struct {
	Type          string   `json:"type"`
	Operators     []string `json:"operators"`
	IsRequired    bool     `json:"isRequired"`
	ParameterName string   `json:"parameterName"`
}

type SemanticConditionValue struct {
	BlockID        string          `json:"blockId"`
	ConditionID    string          `json:"conditionId"`
	ParameterName  string          `json:"parameterName"`
	ParameterValue json.RawMessage `json:"parameterValue"`
}

type SubTemplateSnapshot struct {
	DID          string       `json:"did"`
	Name         string       `json:"name"`
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
