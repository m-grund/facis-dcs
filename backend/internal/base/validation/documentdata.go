package validation

import (
	"digital-contracting-service/internal/base/datatype"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	SchemaDocumentStructureV1 = "facis.dcs.document-structure.v1"
	SchemaTemplateDataV1      = "facis.dcs.template-data.v1"
	SchemaContractDataV1      = "facis.dcs.contract-data.v1"
	SchemaSemanticConditionV1 = "facis.dcs.semantic-condition.v1"
	SchemaPartyV1             = "facis.dcs.party.v1"
	SchemaContractV1          = "facis.dcs.contract.v1"
	SchemaServiceV1           = "facis.dcs.service.v1"
	SchemaSignatureV1         = "facis.dcs.signature.v1"
	SchemaJSONLDContextV1     = "https://w3id.org/facis/dcs/context/v1"
	SchemaOntologyV1          = "https://w3id.org/facis/dcs/ontology/v1"
	SchemaSHACLShapesV1       = "https://w3id.org/facis/dcs/shapes/v1"
	SemanticProfileName       = "FACIS DCS Semantic Contract Profile"
	SemanticProfileVersionV1  = "v1"

	PolicyTemplateStructureV1          = "facis.dcs.template.structure"
	PolicyTemplateSemanticConditionsV1 = "facis.dcs.template.semantic-conditions"
	PolicyContractStructureV1          = "facis.dcs.contract.structure"
	PolicyContractSemanticValuesV1     = "facis.dcs.contract.semantic-values"

	semanticRuleOperatorProperty        = "operator"
	semanticRuleRightOperandProperty    = "rightOperand"
	semanticRuleAppliesToClauseProperty = "appliesToClause"

	contractStatementSetType       = ontologyDCSBase + "ContractStatementSet"
	contractStatementPartyType     = ontologyDCSBase + "Party"
	contractStatementPaymentType   = ontologyDCSBase + "PaymentTerm"
	contractStatementSLOType       = ontologyDCSBase + "ServiceLevelObjective"
	contractStatementObligation    = ontologyDCSBase + "Obligation"
	contractStatementPermission    = ontologyDCSBase + "Permission"
	contractStatementProhibition   = ontologyDCSBase + "Prohibition"
	contractStatementConstraint    = ontologyDCSBase + "Constraint"
	contractStatementProviderRole  = ontologyDCSTBase + "role-provider"
	contractStatementCustomerRole  = ontologyDCSTBase + "role-customer"
	contractStatementPayAction     = ontologyDCSBase + "Pay"
	contractStatementSLAAction     = ontologyDCSBase + "MaintainAvailability"
	contractStatementAvailability  = ontologyDCSBase + "Availability"
	contractStatementPercentUnit   = ontologyDCSTBase + "unit-percent"
	semanticRuleSourceContract     = "contractSemantics"
	semanticRuleSourceCondition    = "semanticCondition"
	contractStatementsPropertyName = "contractStatements"
)

var (
	templatePolicyRefs = []map[string]any{
		{"policyId": PolicyTemplateStructureV1, "version": "v1", "enforcementPoint": "template:create"},
		{"policyId": PolicyTemplateSemanticConditionsV1, "version": "v1", "enforcementPoint": "template:verify"},
	}
	contractPolicyRefs = []map[string]any{
		{"policyId": PolicyContractStructureV1, "version": "v1", "enforcementPoint": "contract:create"},
		{"policyId": PolicyContractSemanticValuesV1, "version": "v1", "enforcementPoint": "contract:update"},
	}
)

type domainField struct {
	SchemaRef      string
	Type           string
	DomainPath     string
	OntologyTerm   string
	StatementField string
	Constraint     *valueConstraint
}

type valueConstraint struct {
	Format           string
	Pattern          string
	AllowedValues    []string
	AllowedValuesRef string
	Min              *float64
	Max              *float64
	Description      string
}

type blockDefinition struct {
	SchemaRef    string
	SemanticPath string
}

func (constraint *valueConstraint) asMap() map[string]any {
	result := map[string]any{}
	if constraint.Format != "" {
		result["format"] = constraint.Format
	}
	if constraint.Pattern != "" {
		result["pattern"] = constraint.Pattern
	}
	if len(constraint.AllowedValues) > 0 {
		values := make([]any, len(constraint.AllowedValues))
		for i, value := range constraint.AllowedValues {
			values[i] = value
		}
		result["allowedValues"] = values
	}
	if constraint.AllowedValuesRef != "" {
		result["allowedValuesRef"] = constraint.AllowedValuesRef
	}
	if constraint.Min != nil {
		result["min"] = *constraint.Min
	}
	if constraint.Max != nil {
		result["max"] = *constraint.Max
	}
	if constraint.Description != "" {
		result["description"] = constraint.Description
	}
	return result
}

var blockCatalogue = map[string]blockDefinition{
	"facis.block.document.section":            {SchemaRef: SchemaDocumentStructureV1, SemanticPath: "document.section"},
	"facis.block.text.free":                   {SchemaRef: SchemaDocumentStructureV1, SemanticPath: "document.freeText"},
	"facis.block.clause.custom":               {SchemaRef: SchemaDocumentStructureV1, SemanticPath: "document.clause"},
	"facis.block.party.company":               {SchemaRef: SchemaPartyV1, SemanticPath: "company"},
	"facis.block.party.company.location":      {SchemaRef: SchemaPartyV1, SemanticPath: "company.location"},
	"facis.block.party.representative":        {SchemaRef: SchemaPartyV1, SemanticPath: "company.representative"},
	"facis.block.party.contact":               {SchemaRef: SchemaPartyV1, SemanticPath: "company.contact"},
	"facis.block.contract.basics":             {SchemaRef: SchemaContractV1, SemanticPath: "contract"},
	"facis.block.contract.jurisdiction":       {SchemaRef: SchemaContractV1, SemanticPath: "contract.jurisdiction"},
	"facis.block.sla.availability":            {SchemaRef: SchemaServiceV1, SemanticPath: "service.sla.availability"},
	"facis.block.sla.response-time":           {SchemaRef: SchemaServiceV1, SemanticPath: "service.sla.responseTime"},
	"facis.block.sla.resolution-time":         {SchemaRef: SchemaServiceV1, SemanticPath: "service.sla.resolutionTime"},
	"facis.block.sla.support":                 {SchemaRef: SchemaServiceV1, SemanticPath: "service.sla.supportHours"},
	"facis.block.contract.validity":           {SchemaRef: SchemaContractV1, SemanticPath: "contract.validity"},
	"facis.block.contract.renewal":            {SchemaRef: SchemaContractV1, SemanticPath: "contract.renewal"},
	"facis.block.contract.termination":        {SchemaRef: SchemaContractV1, SemanticPath: "contract.termination"},
	"facis.block.contract.payment":            {SchemaRef: SchemaContractV1, SemanticPath: "contract.payment"},
	"facis.block.contract.liability":          {SchemaRef: SchemaContractV1, SemanticPath: "contract.liability"},
	"facis.block.contract.insurance":          {SchemaRef: SchemaContractV1, SemanticPath: "contract.insurance"},
	"facis.block.contract.confidentiality":    {SchemaRef: SchemaContractV1, SemanticPath: "contract.confidentiality"},
	"facis.block.contract.data-protection":    {SchemaRef: SchemaContractV1, SemanticPath: "contract.dataProtection"},
	"facis.block.contract.audit-rights":       {SchemaRef: SchemaContractV1, SemanticPath: "contract.auditRights"},
	"facis.block.contract.ip-rights":          {SchemaRef: SchemaContractV1, SemanticPath: "contract.ipRights"},
	"facis.block.contract.force-majeure":      {SchemaRef: SchemaContractV1, SemanticPath: "contract.forceMajeure"},
	"facis.block.contract.dispute-resolution": {SchemaRef: SchemaContractV1, SemanticPath: "contract.disputeResolution"},
	"facis.block.service.description":         {SchemaRef: SchemaServiceV1, SemanticPath: "service.description"},
	"facis.block.service.scope":               {SchemaRef: SchemaServiceV1, SemanticPath: "service.scope"},
	"facis.block.service.deliverable":         {SchemaRef: SchemaServiceV1, SemanticPath: "service.deliverable"},
	"facis.block.signature.requirement":       {SchemaRef: SchemaSignatureV1, SemanticPath: "signature.requiredLevel"},
	"facis.block.signature.signer-role":       {SchemaRef: SchemaSignatureV1, SemanticPath: "signature.requiredSignerRole"},
	"facis.block.signature.deadline":          {SchemaRef: SchemaSignatureV1, SemanticPath: "signature.deadline"},
	"facis.block.template.approved-embed":     {SchemaRef: SchemaTemplateDataV1, SemanticPath: "template.approvedEmbed"},
	"facis.block.template.merged-approved":    {SchemaRef: SchemaTemplateDataV1, SemanticPath: "template.mergedApproved"},
}

type documentData map[string]any

// NormalizeTemplateData adds FACIS schema and policy references and validates the
// structural shape expected by the template builder.
func NormalizeTemplateData(raw *datatype.JSON) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	normalizeTemplateMetadata(data)
	if err := validateCommonStructure(data); err != nil {
		return nil, err
	}
	if err := validateSchemaRefs(data, true); err != nil {
		return nil, err
	}
	if err := validatePolicyRefs(data, true); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeTemplateDataForPersistence keeps stored template JSON-LD
// self-identifying when it is read outside the relational row envelope.
func NormalizeTemplateDataForPersistence(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	normalized, err := NormalizeTemplateData(raw)
	if err != nil {
		return nil, err
	}
	return addDocumentIdentity(normalized, did)
}

// NormalizeContractData adds FACIS contract schema and policy references and
// validates structure plus semantic values. When requireSemanticValues is false,
// required semantic values may still be empty so a draft contract can be created
// from a template before the creator has filled all parameters.
func NormalizeContractData(raw *datatype.JSON, requireSemanticValues bool) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	normalizeContractMetadata(data)
	if err := validateCommonStructure(data); err != nil {
		return nil, err
	}
	if err := validateSchemaRefs(data, false); err != nil {
		return nil, err
	}
	if err := validatePolicyRefs(data, false); err != nil {
		return nil, err
	}
	if err := validateSemanticValues(data, requireSemanticValues); err != nil {
		return nil, err
	}
	normalizeContractSemanticRuntime(data)
	if err := validateContractSemanticsData(data, requireSemanticValues); err != nil {
		return nil, err
	}
	if err := validateContractParties(data); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeContractDataForPersistence keeps stored contract JSON-LD
// self-identifying when it is read outside the relational row envelope.
func NormalizeContractDataForPersistence(raw *datatype.JSON, did string, requireSemanticValues bool) (*datatype.JSON, error) {
	normalized, err := NormalizeContractData(raw, requireSemanticValues)
	if err != nil {
		return nil, err
	}
	return addDocumentIdentity(normalized, did)
}

// BuildContractStatements derives machine-readable contract statements from
// semantic condition values. The input may use canonical ontology URIs or legacy
// dot-path semanticPath aliases.
func BuildContractStatements(raw *datatype.JSON) ([]map[string]any, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if err := validateCommonStructure(data); err != nil {
		return nil, err
	}
	return buildContractStatements(data)
}

// ValidateContractSemantics validates placeholders, bindings, semantic values,
// derived contractStatements, and generated contract semantic rules.
func ValidateContractSemantics(raw *datatype.JSON) error {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return err
	}
	if err := validateCommonStructure(data); err != nil {
		return err
	}
	if _, ok := data["semanticConditionValues"]; !ok {
		data["semanticConditionValues"] = []any{}
	}
	if err := validateSemanticValues(data, true); err != nil {
		return err
	}
	normalizeContractSemanticRuntime(data)
	return validateContractSemanticsData(data, true)
}

func addDocumentIdentity(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(did) != "" {
		data["@id"] = did
		data["did"] = did
	}
	return encodeDocumentData(data)
}

func decodeDocumentData(raw *datatype.JSON) (documentData, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return documentData{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(*raw, &data); err != nil {
		return nil, fmt.Errorf("document data is not valid JSON: %w", err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func encodeDocumentData(data documentData) (*datatype.JSON, error) {
	normalized, err := datatype.NewJSON(map[string]any(data))
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func normalizeTemplateMetadata(data documentData) {
	data["@context"] = SchemaJSONLDContextV1
	data["@type"] = "ContractTemplate"
	data["schemaRefs"] = map[string]any{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"templateData":      SchemaTemplateDataV1,
		"jsonLdContext":     SchemaJSONLDContextV1,
		"ontology":          SchemaOntologyV1,
		"shaclShapes":       SchemaSHACLShapesV1,
	}
	data["policyRefs"] = templatePolicyRefs
	data["validation"] = map[string]any{
		"schemaVersion":     "v1",
		"profile":           "FACIS_DCS_TEMPLATE_V1",
		"requiredPolicies":  []string{PolicyTemplateStructureV1, PolicyTemplateSemanticConditionsV1},
		"validatedBySchema": true,
	}
	normalizeSemanticProfile(data)
	normalizeSemanticRuntimeMetadata(data)
}

func normalizeContractMetadata(data documentData) {
	data["@context"] = SchemaJSONLDContextV1
	data["@type"] = "Contract"
	data["schemaRefs"] = map[string]any{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"contractData":      SchemaContractDataV1,
		"jsonLdContext":     SchemaJSONLDContextV1,
		"ontology":          SchemaOntologyV1,
		"shaclShapes":       SchemaSHACLShapesV1,
	}
	data["policyRefs"] = contractPolicyRefs
	data["validation"] = map[string]any{
		"schemaVersion":     "v1",
		"profile":           "FACIS_DCS_CONTRACT_V1",
		"requiredPolicies":  []string{PolicyContractStructureV1, PolicyContractSemanticValuesV1},
		"validatedBySchema": true,
	}
	if _, ok := data["semanticConditionValues"]; !ok {
		data["semanticConditionValues"] = []any{}
	}
	normalizeSemanticProfile(data)
	normalizeSemanticRuntimeMetadata(data)
}

func normalizeSemanticProfile(data documentData) {
	data["semanticProfile"] = map[string]any{
		"name":     SemanticProfileName,
		"version":  SemanticProfileVersionV1,
		"context":  SchemaJSONLDContextV1,
		"ontology": SchemaOntologyV1,
		"shapes":   SchemaSHACLShapesV1,
	}
}

func normalizeSemanticRuntimeMetadata(data documentData) {
	data["placeholderBindings"] = buildPlaceholderBindings(data)
	data["semanticRules"] = mergeSemanticRules(data["semanticRules"], buildSemanticRules(data))
}

func normalizeContractSemanticRuntime(data documentData) {
	statements, err := buildContractStatements(data)
	if err == nil {
		data[contractStatementsPropertyName] = map[string]any{
			"@type":      contractStatementSetType,
			"statements": statementsToAny(statements),
		}
	}
	generated := buildSemanticRules(data)
	data["semanticRules"] = mergeSemanticRules(data["semanticRules"], generated)
}

func validateSchemaRefs(data documentData, template bool) error {
	refs, ok := data["schemaRefs"].(map[string]any)
	if !ok {
		return errors.New("schemaRefs must be an object")
	}
	required := map[string]string{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
	}
	if template {
		required["templateData"] = SchemaTemplateDataV1
	} else {
		required["contractData"] = SchemaContractDataV1
	}
	for key, expected := range required {
		if actual, _ := refs[key].(string); actual != expected {
			return fmt.Errorf("schemaRefs.%s must be %q", key, expected)
		}
	}
	return nil
}

func validatePolicyRefs(data documentData, template bool) error {
	policies, ok := asArray(data["policyRefs"])
	if !ok {
		return errors.New("policyRefs must be an array")
	}
	required := []string{PolicyContractStructureV1, PolicyContractSemanticValuesV1}
	if template {
		required = []string{PolicyTemplateStructureV1, PolicyTemplateSemanticConditionsV1}
	}
	seen := map[string]bool{}
	for _, item := range policies {
		policy, ok := item.(map[string]any)
		if !ok {
			return errors.New("policyRefs entries must be objects")
		}
		policyID, _ := policy["policyId"].(string)
		version, _ := policy["version"].(string)
		if strings.TrimSpace(policyID) == "" || strings.TrimSpace(version) == "" {
			return errors.New("policyRefs entries require policyId and version")
		}
		seen[policyID] = true
	}
	for _, policyID := range required {
		if !seen[policyID] {
			return fmt.Errorf("required policy %q is missing", policyID)
		}
	}
	return nil
}

func buildPlaceholderBindings(data documentData) []map[string]any {
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := semanticConditionIndex(data)

	placeholderPattern := regexp.MustCompile(`\{\{([^}.]+)\.([^}]+)\}\}`)
	seen := map[string]bool{}
	bindings := []map[string]any{}
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		text, _ := block["text"].(string)
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			if len(match) != 3 {
				continue
			}
			conditionID := match[1]
			parameterName := match[2]
			condition := conditions.conditionForBlock(blockID, conditionID)
			if condition == nil {
				continue
			}
			if _, found := findParameter(condition, parameterName); !found {
				continue
			}
			key := blockID + "\x00" + conditionID + "\x00" + parameterName
			if seen[key] {
				continue
			}
			seen[key] = true
			bindings = append(bindings, map[string]any{
				"@type":            "PlaceholderBinding",
				"placeholder":      "{{" + conditionID + "." + parameterName + "}}",
				"boundToCondition": conditionID,
				"boundToParameter": parameterName,
				"blockId":          blockID,
				"source":           "clause-placeholder",
			})
		}
	}
	return bindings
}

func buildSemanticRules(data documentData) []map[string]any {
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := asArray(data["semanticConditions"])
	blockIDsByCondition := map[string][]string{}
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		refs, _ := asArray(block["conditionIds"])
		for _, rawConditionID := range refs {
			conditionID, ok := rawConditionID.(string)
			if !ok {
				continue
			}
			blockIDsByCondition[conditionID] = append(blockIDsByCondition[conditionID], blockID)
		}
	}

	rules := []map[string]any{}
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := condition["conditionId"].(string)
		conditionName, _ := condition["conditionName"].(string)
		if conditionName == "" {
			conditionName = conditionID
		}
		parameters, _ := asArray(condition["parameters"])
		for _, rawParam := range parameters {
			param, ok := rawParam.(map[string]any)
			if !ok {
				continue
			}
			parameterName, _ := param["parameterName"].(string)
			parameterType, _ := param["type"].(string)
			operators, _ := asArray(param["operators"])
			for _, rawOperator := range operators {
				operate, targets := parseSemanticOperator(rawOperator)
				operator := normalizeSemanticOperator(operate)
				if operator == "" {
					continue
				}
				ruleType := "SemanticRule"
				if parameterType == "date" {
					ruleType = "DateConstraintRule"
				} else if parameterType == "decimal" || parameterType == "integer" {
					ruleType = "ThresholdRule"
				}
				var rightOperand any = targets
				if len(targets) == 1 {
					rightOperand = targets[0]
				}
				rules = append(rules, map[string]any{
					"@type":                             ruleType,
					"ruleId":                            "rule-" + slugify(conditionID) + "-" + slugify(parameterName) + "-" + slugify(operator),
					"conditionId":                       conditionID,
					"parameterName":                     parameterName,
					semanticRuleAppliesToClauseProperty: stringSliceToAny(blockIDsByCondition[conditionID]),
					"leftOperand":                       "{{" + conditionID + "." + parameterName + "}}",
					semanticRuleOperatorProperty:        operator,
					semanticRuleRightOperandProperty:    rightOperand,
					"valueType":                         parameterType,
					"severity":                          semanticRuleSeverity(param),
					"source":                            semanticRuleSourceCondition,
					"message":                           fmt.Sprintf("%s.%s must satisfy %s.", conditionName, parameterName, operator),
				})
			}
		}
	}
	return rules
}

func mergeSemanticRules(rawExisting any, generated []map[string]any) []any {
	result := []any{}
	seen := map[string]bool{}
	if existing, ok := asArray(rawExisting); ok {
		for _, item := range existing {
			rule, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if source, _ := rule["source"].(string); source == semanticRuleSourceCondition || source == semanticRuleSourceContract {
				continue
			}
			canonicalizeSemanticRule(rule)
			ruleID, _ := rule["ruleId"].(string)
			if strings.TrimSpace(ruleID) == "" || seen[ruleID] {
				continue
			}
			seen[ruleID] = true
			result = append(result, rule)
		}
	}
	for _, rule := range generated {
		ruleID, _ := rule["ruleId"].(string)
		if strings.TrimSpace(ruleID) == "" || seen[ruleID] {
			continue
		}
		seen[ruleID] = true
		result = append(result, rule)
	}
	return result
}

// Keep generated and client-provided rules on the JSON-LD v1 ontology terms.
func canonicalizeSemanticRule(rule map[string]any) {
	if rawOperator, ok := rule[semanticRuleOperatorProperty].(string); ok {
		if operator := normalizeSemanticOperator(rawOperator); operator != "" {
			rule[semanticRuleOperatorProperty] = operator
		}
	} else if rawOperate, ok := rule["operate"].(string); ok {
		if operator := normalizeSemanticOperator(rawOperate); operator != "" {
			rule[semanticRuleOperatorProperty] = operator
		}
	}

	if _, exists := rule[semanticRuleRightOperandProperty]; !exists {
		if targets, ok := asArray(rule["targets"]); ok {
			if len(targets) == 1 {
				rule[semanticRuleRightOperandProperty] = targets[0]
			} else {
				rule[semanticRuleRightOperandProperty] = targets
			}
		}
	}

	if _, exists := rule[semanticRuleAppliesToClauseProperty]; !exists {
		if blockIDs, ok := asArray(rule["blockIds"]); ok {
			rule[semanticRuleAppliesToClauseProperty] = blockIDs
		}
	}

	delete(rule, "operate")
	delete(rule, "targets")
	delete(rule, "blockIds")
}

func parseSemanticOperator(raw any) (string, []string) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case map[string]any:
		operate, _ := value["operate"].(string)
		if strings.TrimSpace(operate) == "" {
			operate, _ = value[semanticRuleOperatorProperty].(string)
		}
		targets := []string{}
		if rawTargets, ok := asArray(value["targets"]); ok {
			for _, rawTarget := range rawTargets {
				target, ok := rawTarget.(string)
				if ok {
					targets = append(targets, target)
				}
			}
		} else if rawTarget, ok := value[semanticRuleRightOperandProperty].(string); ok {
			targets = append(targets, rawTarget)
		}
		return operate, targets
	default:
		return "", nil
	}
}

func normalizeSemanticOperator(value string) string {
	switch value {
	case "Equals", "NotEquals", "GreaterThan", "GreaterThanOrEqual", "LessThan", "LessThanOrEqual", "Between", "Contains", "MatchesRegex":
		return value
	case "equal":
		return "Equals"
	case "notEqual":
		return "NotEquals"
	case "greaterThan":
		return "GreaterThan"
	case "greaterThanOrEqual":
		return "GreaterThanOrEqual"
	case "lessThan":
		return "LessThan"
	case "lessThanOrEqual":
		return "LessThanOrEqual"
	case "between":
		return "Between"
	case "contains":
		return "Contains"
	case "matchesRegex":
		return "MatchesRegex"
	default:
		return ""
	}
}

func semanticRuleSeverity(param map[string]any) string {
	if isTrue(param["isRequired"]) {
		return "blocking"
	}
	return "error"
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func slugify(value string) string {
	value = regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(value, "${1}-${2}")
	value = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return strings.ToLower(value)
}

func validateCommonStructure(data documentData) error {
	outline, ok := asArray(data["documentOutline"])
	if !ok {
		return errors.New("documentOutline must be an array")
	}
	blocks, ok := asArray(data["documentBlocks"])
	if !ok {
		return errors.New("documentBlocks must be an array")
	}
	conditions, ok := asArray(data["semanticConditions"])
	if !ok {
		return errors.New("semanticConditions must be an array")
	}
	if _, ok := asArray(data["customMetaData"]); !ok {
		if _, isContract := data["contractData"]; !isContract {
			data["customMetaData"] = []any{}
		}
	}

	blockTypes := map[string]string{}
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			return errors.New("documentBlocks entries must be objects")
		}
		id, _ := block["blockId"].(string)
		blockType, _ := block["type"].(string)
		if strings.TrimSpace(id) == "" {
			return errors.New("documentBlocks entries require blockId")
		}
		if blockTypes[id] != "" {
			return fmt.Errorf("duplicate document block id %q", id)
		}
		if !validBlockType(blockType) {
			return fmt.Errorf("block %q has invalid type %q", id, blockType)
		}
		normalizeBlockCatalogue(block)
		if err := validateBlockCatalogue(block); err != nil {
			return fmt.Errorf("block %q catalogue validation failed: %w", id, err)
		}
		blockTypes[id] = blockType
	}

	outlineIDs := map[string]bool{}
	rootCount := 0
	for _, item := range outline {
		node, ok := item.(map[string]any)
		if !ok {
			return errors.New("documentOutline entries must be objects")
		}
		id, _ := node["blockId"].(string)
		if strings.TrimSpace(id) == "" {
			return errors.New("documentOutline entries require blockId")
		}
		if outlineIDs[id] {
			return fmt.Errorf("duplicate outline block id %q", id)
		}
		outlineIDs[id] = true
		if isTrue(node["isRoot"]) {
			rootCount++
		} else if _, ok := blockTypes[id]; !ok {
			return fmt.Errorf("outline block %q has no matching document block", id)
		}
		children, ok := asArray(node["children"])
		if !ok {
			return fmt.Errorf("outline block %q children must be an array", id)
		}
		for _, rawChildID := range children {
			childID, ok := rawChildID.(string)
			if !ok || strings.TrimSpace(childID) == "" {
				return fmt.Errorf("outline block %q has invalid child reference", id)
			}
			if _, ok := blockTypes[childID]; !ok {
				return fmt.Errorf("outline block %q references unknown child block %q", id, childID)
			}
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("documentOutline must contain exactly one root block, got %d", rootCount)
	}

	conditionIDs := map[string]bool{}
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			return errors.New("semanticConditions entries must be objects")
		}
		id, err := validateSemanticCondition(condition)
		if err != nil {
			return err
		}
		if conditionIDs[id] {
			return fmt.Errorf("duplicate semantic condition id %q", id)
		}
		conditionIDs[id] = true
	}
	embeddedConditions, err := embeddedSemanticConditionsByBlockID(data)
	if err != nil {
		return err
	}

	for _, item := range blocks {
		block := item.(map[string]any)
		if block["type"] != "CLAUSE" {
			continue
		}
		id, _ := block["blockId"].(string)
		refs, ok := asArray(block["conditionIds"])
		if !ok {
			return fmt.Errorf("clause block %q conditionIds must be an array", id)
		}
		for _, rawConditionID := range refs {
			conditionID, ok := rawConditionID.(string)
			if !ok || !conditionReferenceExists(id, conditionID, conditionIDs, embeddedConditions) {
				return fmt.Errorf("clause block %q references unknown semantic condition %q", id, conditionID)
			}
		}
	}
	return nil
}

func validateSemanticValues(data documentData, requireSemanticValues bool) error {
	values, ok := asArray(data["semanticConditionValues"])
	if !ok {
		return errors.New("semanticConditionValues must be an array")
	}
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := asArray(data["semanticConditions"])
	embeddedConditions, err := embeddedSemanticConditionsByBlockID(data)
	if err != nil {
		return err
	}

	conditionByID := map[string]map[string]any{}
	requiredParams := map[string]map[string]string{}
	for _, item := range conditions {
		condition := item.(map[string]any)
		conditionID := condition["conditionId"].(string)
		conditionByID[conditionID] = condition
		parameters, _ := asArray(condition["parameters"])
		for _, rawParam := range parameters {
			param := rawParam.(map[string]any)
			if !isTrue(param["isRequired"]) {
				continue
			}
			if _, ok := requiredParams[conditionID]; !ok {
				requiredParams[conditionID] = map[string]string{}
			}
			requiredParams[conditionID][param["parameterName"].(string)] = param["type"].(string)
		}
	}

	clauseConditions := map[string]map[string]bool{}
	for _, item := range blocks {
		block := item.(map[string]any)
		if block["type"] != "CLAUSE" {
			continue
		}
		blockID := block["blockId"].(string)
		clauseConditions[blockID] = map[string]bool{}
		refs, _ := asArray(block["conditionIds"])
		for _, rawConditionID := range refs {
			clauseConditions[blockID][rawConditionID.(string)] = true
		}
	}

	provided := map[string]bool{}
	for _, item := range values {
		value, ok := item.(map[string]any)
		if !ok {
			return errors.New("semanticConditionValues entries must be objects")
		}
		blockID, _ := value["blockId"].(string)
		conditionID, _ := value["conditionId"].(string)
		parameterName, _ := value["parameterName"].(string)
		if !clauseConditions[blockID][conditionID] {
			return fmt.Errorf("semantic value references unknown block/condition pair %q/%q", blockID, conditionID)
		}
		condition := embeddedConditions.conditionForBlock(blockID, conditionID)
		if condition == nil {
			condition = conditionByID[conditionID]
		}
		param, found := findParameter(condition, parameterName)
		if !found {
			return fmt.Errorf("semantic value references unknown parameter %q on condition %q", parameterName, conditionID)
		}
		paramType, _ := param["type"].(string)
		if rawValue, ok := value["parameterValue"]; ok && rawValue != nil {
			if !valueMatchesType(rawValue, paramType) {
				return fmt.Errorf("semantic value %q on condition %q does not match type %q", parameterName, conditionID, paramType)
			}
			semanticPath, _ := param["semanticPath"].(string)
			if field, ok := ontologyDomainFieldIndex[semanticPath]; ok && field.Constraint != nil {
				if err := valueMatchesConstraint(rawValue, field.Constraint); err != nil {
					return fmt.Errorf("semantic value %q on condition %q violates constraint: %w", parameterName, conditionID, err)
				}
			}
			provided[semanticValueKey(blockID, conditionID, parameterName)] = true
		}
	}
	markFixedSemanticValuesProvided(blocks, embeddedConditions, conditionByID, provided)

	if !requireSemanticValues {
		return nil
	}
	for blockID, conditionSet := range clauseConditions {
		for conditionID := range conditionSet {
			params := embeddedConditions.requiredParamsForBlock(blockID, conditionID)
			if len(params) == 0 {
				params = requiredParams[conditionID]
			}
			for parameterName := range params {
				if !provided[semanticValueKey(blockID, conditionID, parameterName)] {
					return fmt.Errorf("required semantic value missing: block=%s condition=%s parameter=%s", blockID, conditionID, parameterName)
				}
			}
		}
	}
	return nil
}

func markFixedSemanticValuesProvided(blocks []any, embeddedConditions embeddedSemanticConditions, conditionByID map[string]map[string]any, provided map[string]bool) {
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		refs, _ := asArray(block["conditionIds"])
		for _, rawConditionID := range refs {
			conditionID, _ := rawConditionID.(string)
			condition := embeddedConditions.conditionForBlock(blockID, conditionID)
			if condition == nil {
				condition = conditionByID[conditionID]
			}
			if condition == nil {
				continue
			}
			parameters, _ := asArray(condition["parameters"])
			for _, rawParam := range parameters {
				param, ok := rawParam.(map[string]any)
				if !ok {
					continue
				}
				if _, exists := param["fixedValue"]; !exists {
					continue
				}
				parameterName, _ := param["parameterName"].(string)
				provided[semanticValueKey(blockID, conditionID, parameterName)] = true
			}
		}
	}
}

type semanticValueRecord struct {
	BlockID        string
	ConditionID    string
	ParameterName  string
	EntityType     string
	EntityRole     string
	DomainPath     string
	OntologyTerm   string
	StatementField string
	Type           string
	Value          any
}

type partyStatementDraft struct {
	ID        string
	Role      string
	LegalName string
	Country   string
}

type paymentStatementDraft struct {
	Amount   any
	Currency string
	DueDate  string
}

type sloStatementDraft struct {
	Availability any
}

func buildContractStatements(data documentData) ([]map[string]any, error) {
	records, err := semanticValueRecords(data)
	if err != nil {
		return nil, err
	}
	partyDrafts := map[string]*partyStatementDraft{}
	payment := &paymentStatementDraft{}
	slo := &sloStatementDraft{}

	for _, record := range records {
		if record.EntityType == contractStatementPartyType || strings.HasPrefix(record.StatementField, "party.") {
			draft := partyDrafts[record.ConditionID]
			if draft == nil {
				draft = &partyStatementDraft{}
				partyDrafts[record.ConditionID] = draft
			}
			if record.EntityRole != "" && draft.Role == "" {
				draft.Role = record.EntityRole
			}
		}
		switch record.StatementField {
		case "party.legalName", "party.country", "party.role":
			draft := partyDrafts[record.ConditionID]
			if draft == nil {
				draft = &partyStatementDraft{}
				partyDrafts[record.ConditionID] = draft
			}
			switch record.StatementField {
			case "party.legalName":
				draft.LegalName, _ = record.Value.(string)
			case "party.country":
				draft.Country, _ = record.Value.(string)
			case "party.role":
				role, _ := record.Value.(string)
				draft.Role = canonicalContractPartyRole(role)
			}
		case "payment.amount":
			payment.Amount = record.Value
		case "payment.currency":
			payment.Currency, _ = record.Value.(string)
		case "payment.dueDate":
			payment.DueDate, _ = record.Value.(string)
		case "slo.availability":
			slo.Availability = record.Value
		}
	}

	statements := []map[string]any{}
	providerID := ""
	customerID := ""
	for _, draft := range partyDrafts {
		if draft.Role == "" && draft.LegalName == "" && draft.Country == "" {
			continue
		}
		switch draft.Role {
		case contractStatementProviderRole:
			draft.ID = "party-provider"
			providerID = draft.ID
		case contractStatementCustomerRole:
			draft.ID = "party-customer"
			customerID = draft.ID
		default:
			draft.ID = "party-" + slugify(draft.LegalName)
		}
		statement := map[string]any{
			"@id":   draft.ID,
			"@type": contractStatementPartyType,
		}
		if draft.Role != "" {
			statement["role"] = draft.Role
		}
		if draft.LegalName != "" {
			statement["legalName"] = draft.LegalName
		}
		if draft.Country != "" {
			statement["country"] = draft.Country
		}
		statements = append(statements, statement)
	}

	if payment.Amount != nil || payment.Currency != "" || payment.DueDate != "" {
		statement := map[string]any{
			"@id":   "payment-main",
			"@type": contractStatementPaymentType,
		}
		if customerID != "" {
			statement["payer"] = customerID
		}
		if providerID != "" {
			statement["payee"] = providerID
		}
		if payment.Currency != "" {
			statement["currency"] = payment.Currency
		}
		if payment.Amount != nil {
			statement["amount"] = payment.Amount
		}
		if payment.DueDate != "" {
			statement["dueDate"] = payment.DueDate
		}
		statements = append(statements, statement)
		if customerID != "" {
			statements = append(statements, map[string]any{
				"@id":      "obligation-payment",
				"@type":    contractStatementObligation,
				"assignee": customerID,
				"action":   contractStatementPayAction,
				"target":   "payment-main",
			})
		}
	}

	if slo.Availability != nil {
		statements = append(statements, map[string]any{
			"@id":      "slo-availability",
			"@type":    contractStatementSLOType,
			"metric":   contractStatementAvailability,
			"operator": ">=",
			"value":    slo.Availability,
			"unit":     contractStatementPercentUnit,
		})
		if providerID != "" {
			statements = append(statements, map[string]any{
				"@id":      "obligation-availability",
				"@type":    contractStatementObligation,
				"assignee": providerID,
				"action":   contractStatementSLAAction,
				"target":   "slo-availability",
			})
		}
	}

	return statements, nil
}

func validateContractSemanticsData(data documentData, requireCompleteStatements bool) error {
	if err := validatePlaceholderBindings(data, requireCompleteStatements); err != nil {
		return err
	}
	if _, err := buildContractStatements(data); err != nil {
		return err
	}
	if requireCompleteStatements && hasContractStatementIntent(data) {
		if err := validateContractStatementCompleteness(data); err != nil {
			return err
		}
	}
	return nil
}

func hasContractStatementIntent(data documentData) bool {
	records, err := semanticValueRecords(data)
	if err != nil {
		return false
	}
	for _, record := range records {
		switch record.StatementField {
		case "party.role", "payment.amount", "payment.currency", "payment.dueDate", "slo.availability":
			return true
		}
	}
	return false
}

func validatePlaceholderBindings(data documentData, requireValues bool) error {
	conditions, err := semanticConditionIndex(data)
	if err != nil {
		return err
	}
	values, err := semanticValueRecords(data)
	if err != nil {
		return err
	}
	valueByBinding := map[string]bool{}
	for _, value := range values {
		valueByBinding[semanticValueKey(value.BlockID, value.ConditionID, value.ParameterName)] = true
	}

	bindings, ok := asArray(data["placeholderBindings"])
	if !ok {
		return errors.New("placeholderBindings must be an array")
	}
	bindingByPlaceholder := map[string]map[string]any{}
	for _, item := range bindings {
		binding, ok := item.(map[string]any)
		if !ok {
			return errors.New("placeholderBindings entries must be objects")
		}
		blockID, _ := binding["blockId"].(string)
		placeholder, _ := binding["placeholder"].(string)
		conditionID, _ := binding["boundToCondition"].(string)
		parameterName, _ := binding["boundToParameter"].(string)
		condition := conditions.conditionForBlock(blockID, conditionID)
		if condition == nil {
			return fmt.Errorf("placeholder binding %q references unknown condition %q", placeholder, conditionID)
		}
		if _, found := findParameter(condition, parameterName); !found {
			return fmt.Errorf("placeholder binding %q references unknown parameter %q on condition %q", placeholder, parameterName, conditionID)
		}
		if requireValues && !valueByBinding[semanticValueKey(blockID, conditionID, parameterName)] {
			return fmt.Errorf("placeholder binding %q has no semantic value", placeholder)
		}
		bindingByPlaceholder[blockID+"\x00"+placeholder] = binding
	}

	placeholderPattern := regexp.MustCompile(`\{\{([^}.]+)\.([^}]+)\}\}`)
	blocks, _ := asArray(data["documentBlocks"])
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		text, _ := block["text"].(string)
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			placeholder := match[0]
			if bindingByPlaceholder[blockID+"\x00"+placeholder] == nil {
				return fmt.Errorf("placeholder %q in block %q has no binding", placeholder, blockID)
			}
		}
	}
	return nil
}

func validateContractStatementCompleteness(data documentData) error {
	statements, err := buildContractStatements(data)
	if err != nil {
		return err
	}
	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())
	if len(issues) > 0 {
		return ContractStatementValidationError{Issues: issues}
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func semanticValueRecords(data documentData) ([]semanticValueRecord, error) {
	conditions, err := semanticConditionIndex(data)
	if err != nil {
		return nil, err
	}
	values, ok := asArray(data["semanticConditionValues"])
	if !ok {
		return nil, errors.New("semanticConditionValues must be an array")
	}
	records := []semanticValueRecord{}
	for _, item := range values {
		value, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("semanticConditionValues entries must be objects")
		}
		blockID, _ := value["blockId"].(string)
		conditionID, _ := value["conditionId"].(string)
		parameterName, _ := value["parameterName"].(string)
		condition := conditions.conditionForBlock(blockID, conditionID)
		if condition == nil {
			return nil, fmt.Errorf("semantic value references unknown condition %q", conditionID)
		}
		param, found := findParameter(condition, parameterName)
		if !found {
			return nil, fmt.Errorf("semantic value references unknown parameter %q on condition %q", parameterName, conditionID)
		}
		record, err := semanticValueRecordForParameter(blockID, conditionID, parameterName, condition, param, value["parameterValue"])
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	records = append(records, fixedSemanticValueRecords(data, conditions, records)...)
	return records, nil
}

func semanticValueRecordForParameter(blockID string, conditionID string, parameterName string, condition map[string]any, param map[string]any, value any) (semanticValueRecord, error) {
	semanticPath, _ := param["semanticPath"].(string)
	field, ok := ontologyDomainFieldIndex[semanticPath]
	if !ok {
		return semanticValueRecord{}, fmt.Errorf("semantic value parameter %q uses unknown semanticPath %q", parameterName, semanticPath)
	}
	entityType, _ := condition["entityType"].(string)
	entityRole, _ := condition["entityRole"].(string)
	return semanticValueRecord{
		BlockID:        blockID,
		ConditionID:    conditionID,
		ParameterName:  parameterName,
		EntityType:     canonicalStatementEntityType(entityType),
		EntityRole:     canonicalContractEntityRole(entityRole),
		DomainPath:     field.DomainPath,
		OntologyTerm:   field.OntologyTerm,
		StatementField: field.StatementField,
		Type:           field.Type,
		Value:          value,
	}, nil
}

func fixedSemanticValueRecords(data documentData, conditions semanticConditionsByBlock, existing []semanticValueRecord) []semanticValueRecord {
	existingByBinding := map[string]bool{}
	for _, record := range existing {
		existingByBinding[semanticValueKey(record.BlockID, record.ConditionID, record.ParameterName)] = true
	}
	records := []semanticValueRecord{}
	blocks, _ := asArray(data["documentBlocks"])
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		refs, _ := asArray(block["conditionIds"])
		for _, rawConditionID := range refs {
			conditionID, _ := rawConditionID.(string)
			condition := conditions.conditionForBlock(blockID, conditionID)
			if condition == nil {
				continue
			}
			parameters, _ := asArray(condition["parameters"])
			for _, rawParam := range parameters {
				param, ok := rawParam.(map[string]any)
				if !ok {
					continue
				}
				value, hasFixedValue := param["fixedValue"]
				if !hasFixedValue {
					continue
				}
				parameterName, _ := param["parameterName"].(string)
				if existingByBinding[semanticValueKey(blockID, conditionID, parameterName)] {
					continue
				}
				record, err := semanticValueRecordForParameter(blockID, conditionID, parameterName, condition, param, value)
				if err == nil {
					records = append(records, record)
				}
			}
		}
	}
	return records
}

type semanticConditionsByBlock struct {
	topLevel map[string]map[string]any
	embedded embeddedSemanticConditions
}

func (conditions semanticConditionsByBlock) conditionForBlock(blockID string, conditionID string) map[string]any {
	if condition := conditions.embedded.conditionForBlock(blockID, conditionID); condition != nil {
		return condition
	}
	return conditions.topLevel[conditionID]
}

func semanticConditionIndex(data documentData) (semanticConditionsByBlock, error) {
	conditions := semanticConditionsByBlock{topLevel: map[string]map[string]any{}}
	topLevelConditions, _ := asArray(data["semanticConditions"])
	for _, item := range topLevelConditions {
		condition, ok := item.(map[string]any)
		if !ok {
			return conditions, errors.New("semanticConditions entries must be objects")
		}
		conditionID, _ := condition["conditionId"].(string)
		conditions.topLevel[conditionID] = condition
	}
	embedded, err := embeddedSemanticConditionsByBlockID(data)
	if err != nil {
		return conditions, err
	}
	conditions.embedded = embedded
	return conditions, nil
}

func canonicalContractPartyRole(value string) string {
	return canonicalContractEntityRole(value)
}

func canonicalContractEntityRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "provider", "supplier":
		return contractStatementProviderRole
	case "customer", "client":
		return contractStatementCustomerRole
	default:
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			return value
		}
		return ontologyDCSTBase + "role-" + slugify(value)
	}
}

func canonicalStatementEntityType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "party", "company", "customer", "client", "provider", "supplier", "dcs:party", "dcs:company", strings.ToLower(contractStatementPartyType):
		return contractStatementPartyType
	default:
		if strings.HasPrefix(value, ontologyDCSBase) {
			return value
		}
		return ""
	}
}

func entityRoleFromEntityType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "provider", "supplier":
		return contractStatementProviderRole
	case "customer", "client":
		return contractStatementCustomerRole
	default:
		return ""
	}
}

func allowedValuesForDomainPath(domainPath string) []any {
	field, ok := ontologyDomainFieldIndex[domainPath]
	if !ok || field.Constraint == nil {
		return []any{}
	}
	values := make([]any, len(field.Constraint.AllowedValues))
	for i, value := range field.Constraint.AllowedValues {
		values[i] = value
	}
	return values
}

func statementsToAny(statements []map[string]any) []any {
	result := make([]any, len(statements))
	for i, statement := range statements {
		result[i] = statement
	}
	return result
}

type embeddedSemanticConditions struct {
	byOuterBlock map[string]map[string]map[string]any
}

func (conditions embeddedSemanticConditions) blockHasCondition(blockID string, conditionID string) bool {
	return conditions.conditionForBlock(blockID, conditionID) != nil
}

func (conditions embeddedSemanticConditions) hasKnownOuterBlock(blockID string) bool {
	outerBlockID, _ := splitEmbeddedBlockID(blockID)
	if outerBlockID == "" {
		return false
	}
	return conditions.byOuterBlock[outerBlockID] != nil
}

func (conditions embeddedSemanticConditions) conditionForBlock(blockID string, conditionID string) map[string]any {
	outerBlockID, _ := splitEmbeddedBlockID(blockID)
	if outerBlockID == "" {
		return nil
	}
	return conditions.byOuterBlock[outerBlockID][conditionID]
}

func (conditions embeddedSemanticConditions) requiredParamsForBlock(blockID string, conditionID string) map[string]string {
	condition := conditions.conditionForBlock(blockID, conditionID)
	if condition == nil {
		return nil
	}
	requiredParams := map[string]string{}
	parameters, _ := asArray(condition["parameters"])
	for _, rawParam := range parameters {
		param := rawParam.(map[string]any)
		if !isTrue(param["isRequired"]) {
			continue
		}
		requiredParams[param["parameterName"].(string)] = param["type"].(string)
	}
	return requiredParams
}

func conditionReferenceExists(blockID string, conditionID string, topLevelConditionIDs map[string]bool, embeddedConditions embeddedSemanticConditions) bool {
	if embeddedConditions.hasKnownOuterBlock(blockID) {
		return embeddedConditions.blockHasCondition(blockID, conditionID)
	}
	return topLevelConditionIDs[conditionID]
}

func embeddedSemanticConditionsByBlockID(data documentData) (embeddedSemanticConditions, error) {
	result := embeddedSemanticConditions{byOuterBlock: map[string]map[string]map[string]any{}}
	outerBlockByTemplateID := map[string]string{}
	blocks, _ := asArray(data["documentBlocks"])
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		if blockType != "APPROVED_TEMPLATE" && blockType != "MERGED_APPROVED_TEMPLATE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		templateID, _ := block["templateId"].(string)
		if strings.TrimSpace(blockID) == "" || strings.TrimSpace(templateID) == "" {
			continue
		}
		outerBlockByTemplateID[templateID] = blockID
	}

	snapshots, ok := asArray(data["subTemplateSnapshots"])
	if !ok {
		return result, nil
	}
	for _, rawSnapshot := range snapshots {
		snapshot, ok := rawSnapshot.(map[string]any)
		if !ok {
			return result, errors.New("subTemplateSnapshots entries must be objects")
		}
		did, _ := snapshot["did"].(string)
		outerBlockID := outerBlockByTemplateID[did]
		if outerBlockID == "" {
			continue
		}
		templateData, ok := snapshot["template_data"].(map[string]any)
		if !ok {
			return result, fmt.Errorf("subTemplateSnapshot %q template_data must be an object", did)
		}
		conditions, ok := asArray(templateData["semanticConditions"])
		if !ok {
			return result, fmt.Errorf("subTemplateSnapshot %q semanticConditions must be an array", did)
		}
		conditionIDs := map[string]map[string]any{}
		for _, item := range conditions {
			condition, ok := item.(map[string]any)
			if !ok {
				return result, fmt.Errorf("subTemplateSnapshot %q semanticConditions entries must be objects", did)
			}
			id, err := validateSemanticCondition(condition)
			if err != nil {
				return result, err
			}
			if conditionIDs[id] != nil {
				return result, fmt.Errorf("duplicate semantic condition id %q in subTemplateSnapshot %q", id, did)
			}
			conditionIDs[id] = condition
		}
		result.byOuterBlock[outerBlockID] = conditionIDs
	}
	return result, nil
}

func splitEmbeddedBlockID(blockID string) (string, string) {
	parts := strings.SplitN(blockID, "::", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func validateSemanticCondition(condition map[string]any) (string, error) {
	id, _ := condition["conditionId"].(string)
	if strings.TrimSpace(id) == "" {
		return "", errors.New("semanticConditions entries require conditionId")
	}
	if version, _ := condition["schemaVersion"].(string); version != "v1" {
		return "", fmt.Errorf("semantic condition %q must use schemaVersion v1", id)
	}
	if err := validateSemanticConditionEntity(id, condition); err != nil {
		return "", err
	}
	parameters, ok := asArray(condition["parameters"])
	if !ok {
		return "", fmt.Errorf("semantic condition %q parameters must be an array", id)
	}
	for _, rawParam := range parameters {
		param, ok := rawParam.(map[string]any)
		if !ok {
			return "", fmt.Errorf("semantic condition %q parameter entries must be objects", id)
		}
		name, _ := param["parameterName"].(string)
		paramType, _ := param["type"].(string)
		if strings.TrimSpace(name) == "" || !validSemanticType(paramType) {
			return "", fmt.Errorf("semantic condition %q has invalid parameter", id)
		}
		if err := validateDomainParameter(id, param); err != nil {
			return "", err
		}
		if err := validateFixedSemanticValue(id, param); err != nil {
			return "", err
		}
		if err := validateSemanticOperators(id, param); err != nil {
			return "", err
		}
	}
	return id, nil
}

func validateSemanticConditionEntity(conditionID string, condition map[string]any) error {
	rawEntityType, _ := condition["entityType"].(string)
	rawEntityRole, hasEntityRole := condition["entityRole"].(string)
	if strings.TrimSpace(rawEntityType) == "" {
		if hasEntityRole && strings.TrimSpace(rawEntityRole) != "" {
			return fmt.Errorf("semantic condition %q entityRole requires entityType", conditionID)
		}
		return nil
	}
	entityType := canonicalStatementEntityType(rawEntityType)
	if entityType == "" {
		return fmt.Errorf("semantic condition %q uses unsupported entityType %q", conditionID, rawEntityType)
	}
	condition["entityType"] = entityType
	if strings.TrimSpace(rawEntityRole) == "" {
		if inferredRole := entityRoleFromEntityType(rawEntityType); inferredRole != "" {
			condition["entityRole"] = inferredRole
		}
		return nil
	}
	if hasEntityRole && strings.TrimSpace(rawEntityRole) != "" {
		if entityType != contractStatementPartyType {
			return fmt.Errorf("semantic condition %q entityRole is only supported for Party entities", conditionID)
		}
		condition["entityRole"] = canonicalContractEntityRole(rawEntityRole)
	}
	return nil
}

func asArray(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []map[string]any:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	case []string:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func validBlockType(value string) bool {
	switch value {
	case "SECTION", "TEXT", "CLAUSE", "APPROVED_TEMPLATE", "MERGED_APPROVED_TEMPLATE":
		return true
	default:
		return false
	}
}

func normalizeBlockCatalogue(block map[string]any) {
	if _, ok := block["blockCatalogueId"].(string); ok {
		return
	}
	switch block["type"] {
	case "SECTION":
		applyBlockDefinition(block, "facis.block.document.section")
	case "TEXT":
		applyBlockDefinition(block, "facis.block.text.free")
	case "CLAUSE":
		applyBlockDefinition(block, "facis.block.clause.custom")
	case "APPROVED_TEMPLATE":
		applyBlockDefinition(block, "facis.block.template.approved-embed")
	case "MERGED_APPROVED_TEMPLATE":
		applyBlockDefinition(block, "facis.block.template.merged-approved")
	}
}

func applyBlockDefinition(block map[string]any, catalogueID string) {
	def, ok := blockCatalogue[catalogueID]
	if !ok {
		return
	}
	block["blockCatalogueId"] = catalogueID
	block["schemaRef"] = def.SchemaRef
	block["semanticPath"] = def.SemanticPath
}

func validateBlockCatalogue(block map[string]any) error {
	catalogueID, _ := block["blockCatalogueId"].(string)
	def, ok := blockCatalogue[catalogueID]
	if !ok {
		return fmt.Errorf("unknown blockCatalogueId %q", catalogueID)
	}
	schemaRef, _ := block["schemaRef"].(string)
	semanticPath, _ := block["semanticPath"].(string)
	if schemaRef != def.SchemaRef {
		return fmt.Errorf("schemaRef must be %q for blockCatalogueId %q", def.SchemaRef, catalogueID)
	}
	if semanticPath != def.SemanticPath {
		return fmt.Errorf("semanticPath must be %q for blockCatalogueId %q", def.SemanticPath, catalogueID)
	}
	return nil
}

func validSemanticType(value string) bool {
	switch value {
	case "date", "string", "integer", "decimal", "boolean", "enum":
		return true
	default:
		return false
	}
}

func validateDomainParameter(conditionID string, param map[string]any) error {
	semanticPath, _ := param["semanticPath"].(string)
	if strings.TrimSpace(semanticPath) == "" {
		return fmt.Errorf("semantic condition %q parameter %q requires semanticPath", conditionID, param["parameterName"])
	}
	field, ok := ontologyDomainFieldIndex[semanticPath]
	if !ok {
		return fmt.Errorf("semantic condition %q uses unknown domain semanticPath %q", conditionID, semanticPath)
	}
	if field.OntologyTerm != "" {
		param["semanticPath"] = field.OntologyTerm
	}
	schemaRef, _ := param["schemaRef"].(string)
	if schemaRef != field.SchemaRef {
		return fmt.Errorf("semantic condition %q parameter %q schemaRef must be %q", conditionID, param["parameterName"], field.SchemaRef)
	}
	paramType, _ := param["type"].(string)
	if paramType != field.Type {
		return fmt.Errorf("semantic condition %q parameter %q type must be %q for semanticPath %q", conditionID, param["parameterName"], field.Type, semanticPath)
	}
	if field.Constraint != nil {
		param["valueConstraint"] = field.Constraint.asMap()
	}
	return nil
}

func validateFixedSemanticValue(conditionID string, param map[string]any) error {
	value, exists := param["fixedValue"]
	if !exists || value == nil {
		return nil
	}
	paramType, _ := param["type"].(string)
	if !valueMatchesType(value, paramType) {
		return fmt.Errorf("semantic condition %q parameter %q fixedValue does not match type %q", conditionID, param["parameterName"], paramType)
	}
	semanticPath, _ := param["semanticPath"].(string)
	field, ok := ontologyDomainFieldIndex[semanticPath]
	if ok && field.Constraint != nil {
		if err := valueMatchesConstraint(value, field.Constraint); err != nil {
			return fmt.Errorf("semantic condition %q parameter %q fixedValue violates constraint: %w", conditionID, param["parameterName"], err)
		}
	}
	return nil
}

func validateSemanticOperators(conditionID string, param map[string]any) error {
	rawOperators, exists := param["operators"]
	if !exists {
		param["operators"] = []any{}
		return nil
	}
	operators, ok := asArray(rawOperators)
	if !ok {
		return fmt.Errorf("semantic condition %q parameter %q operators must be an array", conditionID, param["parameterName"])
	}
	for _, rawOperator := range operators {
		operate, _ := parseSemanticOperator(rawOperator)
		if normalizeSemanticOperator(operate) == "" {
			return fmt.Errorf("semantic condition %q parameter %q uses unsupported operator %q", conditionID, param["parameterName"], operate)
		}
	}
	return nil
}

func isTrue(value any) bool {
	v, ok := value.(bool)
	return ok && v
}

func findParameter(condition map[string]any, parameterName string) (map[string]any, bool) {
	parameters, _ := asArray(condition["parameters"])
	for _, rawParam := range parameters {
		param := rawParam.(map[string]any)
		if param["parameterName"] == parameterName {
			return param, true
		}
	}
	return nil, false
}

func valueMatchesType(value any, paramType string) bool {
	switch paramType {
	case "string", "date", "enum":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "decimal":
		_, ok := value.(float64)
		return ok
	default:
		return false
	}
}

func valueMatchesConstraint(value any, constraint *valueConstraint) error {
	if len(constraint.AllowedValues) > 0 {
		text, ok := value.(string)
		if !ok || !containsString(constraint.AllowedValues, text) {
			return fmt.Errorf("expected one of %s", strings.Join(constraint.AllowedValues, ", "))
		}
	}
	if constraint.Pattern != "" {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected value matching %s", constraint.Pattern)
		}
		matched, err := regexp.MatchString(constraint.Pattern, text)
		if err != nil {
			return fmt.Errorf("invalid constraint pattern %q: %w", constraint.Pattern, err)
		}
		if !matched {
			return fmt.Errorf("expected value matching %s", constraint.Pattern)
		}
	}
	if constraint.Min != nil || constraint.Max != nil {
		number, ok := value.(float64)
		if !ok {
			return errors.New("expected numeric constrained value")
		}
		if constraint.Min != nil && number < *constraint.Min {
			return fmt.Errorf("expected value greater than or equal to %v", *constraint.Min)
		}
		if constraint.Max != nil && number > *constraint.Max {
			return fmt.Errorf("expected value less than or equal to %v", *constraint.Max)
		}
	}
	return nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateContractParties(data documentData) error {
	rawParties, exists := data["parties"]
	if !exists {
		return nil
	}
	roleField, ok := ontologyDomainFieldIndex[ontologyDCSTBase+"field-company-role"]
	if !ok || roleField.Constraint == nil || len(roleField.Constraint.AllowedValues) == 0 {
		return errors.New("ontology domain field company.role requires allowed role values")
	}
	parties, ok := asArray(rawParties)
	if !ok {
		return errors.New("parties must be an array")
	}
	for index, rawParty := range parties {
		party, ok := rawParty.(map[string]any)
		if !ok {
			return fmt.Errorf("parties.%d must be an object", index)
		}
		partyType, _ := party["@type"].(string)
		if partyType != "Company" && partyType != "dcs:Company" {
			return fmt.Errorf("parties.%d.@type must be Company", index)
		}
		role, _ := party["role"].(string)
		if !containsString(roleField.Constraint.AllowedValues, role) {
			return fmt.Errorf("parties.%d.role must be one of %s", index, strings.Join(roleField.Constraint.AllowedValues, ", "))
		}
	}
	return nil
}

func semanticValueKey(blockID, conditionID, parameterName string) string {
	return blockID + "\x00" + conditionID + "\x00" + parameterName
}
