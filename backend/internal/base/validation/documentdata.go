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

	PolicyTemplateStructureV1          = "facis.dcs.template.structure"
	PolicyTemplateSemanticConditionsV1 = "facis.dcs.template.semantic-conditions"
	PolicyContractStructureV1          = "facis.dcs.contract.structure"
	PolicyContractSemanticValuesV1     = "facis.dcs.contract.semantic-values"
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
	SchemaRef  string
	Type       string
	Constraint *valueConstraint
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

func numberPtr(value float64) *float64 {
	return &value
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

var domainFields = map[string]domainField{
	"company.legalName":           {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.registrationNumber":  {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.vatId":               {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.representative.name": {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.representative.role": {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.contact.email":       {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.contact.phone":       {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.location.street":     {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.location.postalCode": {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.location.city":       {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.location.region":     {SchemaRef: SchemaPartyV1, Type: "string"},
	"company.location.country": {
		SchemaRef: SchemaPartyV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:           "iso-3166-1-alpha-3",
			Pattern:          "^[A-Z]{3}$",
			AllowedValuesRef: "ISO 3166-1 alpha-3",
			AllowedValues:    []string{"DEU", "AUT", "CHE", "FRA", "NLD", "BEL", "LUX", "POL", "CZE", "ESP", "ITA", "GBR", "USA"},
			Description:      "Use ISO 3166-1 alpha-3 country codes, for example DEU instead of Germany.",
		},
	},
	"contract.title": {SchemaRef: SchemaContractV1, Type: "string"},
	"contract.type": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"frameworkAgreement", "serviceAgreement", "dataProcessingAgreement", "nda", "leaseAgreement", "purchaseAgreement"},
			Description:   "Use the FACIS contract type vocabulary.",
		},
	},
	"contract.jurisdiction": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:           "iso-3166-1-alpha-3",
			Pattern:          "^[A-Z]{3}$",
			AllowedValuesRef: "ISO 3166-1 alpha-3",
			AllowedValues:    []string{"DEU", "AUT", "CHE", "FRA", "NLD", "BEL", "LUX", "POL", "CZE", "ESP", "ITA", "GBR", "USA"},
			Description:      "Use ISO 3166-1 alpha-3 jurisdiction codes.",
		},
	},
	"contract.governingLaw": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"DE", "AT", "CH", "FR", "NL", "BE", "EU"},
			Description:   "Use the agreed governing-law code vocabulary.",
		},
	},
	"contract.disputeResolution.venue": {SchemaRef: SchemaContractV1, Type: "string"},
	"contract.disputeResolution.method": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"court", "arbitration", "mediation"},
			Description:   "Use the FACIS dispute resolution vocabulary.",
		},
	},
	"contract.validity.startDate":           {SchemaRef: SchemaContractV1, Type: "date"},
	"contract.validity.endDate":             {SchemaRef: SchemaContractV1, Type: "date"},
	"contract.effectiveDate":                {SchemaRef: SchemaContractV1, Type: "date"},
	"contract.renewal.noticePeriodDays":     {SchemaRef: SchemaContractV1, Type: "integer"},
	"contract.termination.noticePeriodDays": {SchemaRef: SchemaContractV1, Type: "integer"},
	"contract.payment.amount":               {SchemaRef: SchemaContractV1, Type: "decimal"},
	"contract.payment.currency": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:           "iso-4217",
			Pattern:          "^[A-Z]{3}$",
			AllowedValuesRef: "ISO 4217",
			AllowedValues:    []string{"EUR", "USD", "GBP", "CHF", "PLN", "CZK"},
			Description:      "Use ISO 4217 currency codes.",
		},
	},
	"contract.payment.dueDate":                {SchemaRef: SchemaContractV1, Type: "date"},
	"contract.payment.dueDays":                {SchemaRef: SchemaContractV1, Type: "integer"},
	"contract.liability.capAmount":            {SchemaRef: SchemaContractV1, Type: "decimal"},
	"contract.liability.currency":             {SchemaRef: SchemaContractV1, Type: "string"},
	"contract.insurance.minimumCoverage":      {SchemaRef: SchemaContractV1, Type: "decimal"},
	"contract.confidentiality.durationMonths": {SchemaRef: SchemaContractV1, Type: "integer"},
	"contract.dataProtection.role": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"controller", "processor", "jointController", "subProcessor"},
			Description:   "Use the GDPR role vocabulary.",
		},
	},
	"contract.auditRights.frequency": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"onDemand", "annual", "semiAnnual", "quarterly"},
			Description:   "Use the FACIS audit frequency vocabulary.",
		},
	},
	"contract.ipRights.ownership": {
		SchemaRef: SchemaContractV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "controlled-vocabulary",
			AllowedValues: []string{"customer", "supplier", "joint", "preExistingRetained"},
			Description:   "Use the FACIS IP ownership vocabulary.",
		},
	},
	"contract.forceMajeure.noticeDays":       {SchemaRef: SchemaContractV1, Type: "integer"},
	"service.description":                    {SchemaRef: SchemaServiceV1, Type: "string"},
	"service.scope":                          {SchemaRef: SchemaServiceV1, Type: "string"},
	"service.deliverable.name":               {SchemaRef: SchemaServiceV1, Type: "string"},
	"service.deliverable.acceptanceCriteria": {SchemaRef: SchemaServiceV1, Type: "string"},
	"service.sla.availability": {
		SchemaRef: SchemaServiceV1,
		Type:      "decimal",
		Constraint: &valueConstraint{
			Min:         numberPtr(0),
			Max:         numberPtr(100),
			Description: "Availability percentage from 0 to 100.",
		},
	},
	"service.sla.responseTime":   {SchemaRef: SchemaServiceV1, Type: "integer"},
	"service.sla.resolutionTime": {SchemaRef: SchemaServiceV1, Type: "integer"},
	"service.sla.supportHours":   {SchemaRef: SchemaServiceV1, Type: "string"},
	"signature.requiredLevel": {
		SchemaRef: SchemaSignatureV1,
		Type:      "string",
		Constraint: &valueConstraint{
			Format:        "eidas-signature-level",
			AllowedValues: []string{"SES", "AES", "QES"},
			Description:   "Use eIDAS signature level codes.",
		},
	},
	"signature.requiredSignerRole": {SchemaRef: SchemaSignatureV1, Type: "string"},
	"signature.deadline":           {SchemaRef: SchemaSignatureV1, Type: "date"},
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
	data["schemaRefs"] = map[string]any{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"templateData":      SchemaTemplateDataV1,
	}
	data["policyRefs"] = templatePolicyRefs
	data["validation"] = map[string]any{
		"schemaVersion":     "v1",
		"profile":           "FACIS_DCS_TEMPLATE_V1",
		"requiredPolicies":  []string{PolicyTemplateStructureV1, PolicyTemplateSemanticConditionsV1},
		"validatedBySchema": true,
	}
}

func normalizeContractMetadata(data documentData) {
	data["schemaRefs"] = map[string]any{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"contractData":      SchemaContractDataV1,
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
		id, _ := condition["conditionId"].(string)
		if strings.TrimSpace(id) == "" {
			return errors.New("semanticConditions entries require conditionId")
		}
		if conditionIDs[id] {
			return fmt.Errorf("duplicate semantic condition id %q", id)
		}
		conditionIDs[id] = true
		if version, _ := condition["schemaVersion"].(string); version != "v1" {
			return fmt.Errorf("semantic condition %q must use schemaVersion v1", id)
		}
		parameters, ok := asArray(condition["parameters"])
		if !ok {
			return fmt.Errorf("semantic condition %q parameters must be an array", id)
		}
		for _, rawParam := range parameters {
			param, ok := rawParam.(map[string]any)
			if !ok {
				return fmt.Errorf("semantic condition %q parameter entries must be objects", id)
			}
			name, _ := param["parameterName"].(string)
			paramType, _ := param["type"].(string)
			if strings.TrimSpace(name) == "" || !validSemanticType(paramType) {
				return fmt.Errorf("semantic condition %q has invalid parameter", id)
			}
			if err := validateDomainParameter(id, param); err != nil {
				return err
			}
		}
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
			if !ok || !conditionIDs[conditionID] {
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
		condition := conditionByID[conditionID]
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
			if field, ok := domainFields[semanticPath]; ok && field.Constraint != nil {
				if err := valueMatchesConstraint(rawValue, field.Constraint); err != nil {
					return fmt.Errorf("semantic value %q on condition %q violates constraint: %w", parameterName, conditionID, err)
				}
			}
			provided[semanticValueKey(blockID, conditionID, parameterName)] = true
		}
	}

	if !requireSemanticValues {
		return nil
	}
	for blockID, conditionSet := range clauseConditions {
		for conditionID := range conditionSet {
			for parameterName := range requiredParams[conditionID] {
				if !provided[semanticValueKey(blockID, conditionID, parameterName)] {
					return fmt.Errorf("required semantic value missing: block=%s condition=%s parameter=%s", blockID, conditionID, parameterName)
				}
			}
		}
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
	case "date", "string", "integer", "decimal":
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
	field, ok := domainFields[semanticPath]
	if !ok {
		return fmt.Errorf("semantic condition %q uses unknown domain semanticPath %q", conditionID, semanticPath)
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
	case "string", "date":
		_, ok := value.(string)
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

func semanticValueKey(blockID, conditionID, parameterName string) string {
	return blockID + "\x00" + conditionID + "\x00" + parameterName
}
