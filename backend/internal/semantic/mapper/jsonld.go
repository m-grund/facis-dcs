// Package mapper builds interoperable JSON-LD envelopes from DCS database rows.
//
// The exported JSON-LD separates metadata, human-readable document structure,
// machine-readable contract data, and ODRL policies. The mapper remains read-only:
// it projects stored JSONB into the public JSON-LD shape without mutating storage.
package mapper

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"
)

const (
	jsonLDContextV1     = "https://w3id.org/facis/dcs/context/v1"
	dcsContextV1        = "https://w3id.org/facis/dcs/ontology/v1#"
	odrlContextV2       = "http://www.w3.org/ns/odrl/2/"
	semanticProfileName = "FACIS DCS Semantic Contract Profile"
	semanticProfileV1   = "v1"
	ontologyV1          = "https://w3id.org/facis/dcs/ontology/v1"
	shaclShapesV1       = "https://w3id.org/facis/dcs/shapes/v1"
)

// templateEnvelopeSet holds field names that are set exclusively from the DB row
// (or are structural envelope keys). They are never copied from the inner JSONB
// into the outer envelope to prevent stale values from overriding DB values.
var templateEnvelopeSet = map[string]bool{
	"@context":        true,
	"@id":             true,
	"@type":           true,
	"did":             true,
	"uuid":            true,
	"schemaVersion":   true,
	"documentNumber":  true,
	"templateVersion": true,
	"name":            true,
	"description":     true,
	"createdAt":       true,
	"updatedAt":       true,
	"semanticProfile": true,
	"template_data":   true,
}

// contractEnvelopeSet holds field names that are set exclusively from the DB row
// for contracts. They are not read from the inner JSONB.
var contractEnvelopeSet = map[string]bool{
	"@context":            true,
	"@id":                 true,
	"@type":               true,
	"did":                 true,
	"uuid":                true,
	"contractVersion":     true,
	"state":               true,
	"lifecycleState":      true,
	"name":                true,
	"description":         true,
	"createdAt":           true,
	"updatedAt":           true,
	"validFrom":           true,
	"validUntil":          true,
	"derivedFromTemplate": true,
	"templateVersion":     true,
	"semanticProfile":     true,
	"contractData":        true,
}

// BuildTemplateJSONLD assembles the public ContractTemplate JSON-LD envelope.
func BuildTemplateJSONLD(template templatedb.ContractTemplate, profile OntologyProfile) (map[string]any, error) {
	inner, err := parseJSONB(template.TemplateData)
	if err != nil {
		return nil, fmt.Errorf("parse template_data: %w", err)
	}

	baseID := stableBaseID(template.DID)
	envelope := map[string]any{
		"@context": map[string]any{
			"dcs":  dcsContextV1,
			"odrl": odrlContextV2,
		},
		"@id":                   template.DID,
		"@type":                 "dcs:ContractTemplate",
		"dcs:metadata":          buildTemplateMetadata(template, profile),
		"dcs:documentStructure": buildDocumentStructure(baseID, inner),
		"dcs:contractData":      buildContractData(baseID, inner),
		"dcs:policies":          buildPolicies(baseID, inner),
	}

	return envelope, nil
}

// BuildContractJSONLD assembles the public Contract JSON-LD envelope.
func BuildContractJSONLD(contract contractdb.Contract, sourceTemplate templatedb.ContractTemplate, profile OntologyProfile) (map[string]any, error) {
	inner, err := parseJSONB(contract.ContractData)
	if err != nil {
		return nil, fmt.Errorf("parse contract_data: %w", err)
	}

	baseID := stableBaseID(contract.DID)
	envelope := map[string]any{
		"@context": map[string]any{
			"dcs":  dcsContextV1,
			"odrl": odrlContextV2,
		},
		"@id":                   contract.DID,
		"@type":                 "dcs:Contract",
		"dcs:metadata":          buildContractMetadata(contract, sourceTemplate, profile),
		"dcs:documentStructure": buildDocumentStructure(baseID, inner),
		"dcs:contractData":      buildContractData(baseID, inner),
		"dcs:policies":          buildPolicies(baseID, inner),
	}

	return envelope, nil
}

func buildTemplateMetadata(template templatedb.ContractTemplate, profile OntologyProfile) map[string]any {
	metadata := map[string]any{
		"@id":                  stableBaseID(template.DID) + "#metadata",
		"@type":                "dcs:TemplateMetadata",
		"dcs:did":              template.DID,
		"dcs:templateVersion":  template.Version,
		"dcs:state":            template.State,
		"dcs:templateType":     template.TemplateType,
		"dcs:createdBy":        template.CreatedBy,
		"dcs:createdAt":        template.CreatedAt.UTC().Format(time.RFC3339),
		"dcs:updatedAt":        template.UpdatedAt.UTC().Format(time.RFC3339),
		"dcs:semanticProfile":  buildSemanticProfile(profile),
		"dcs:schemaVersion":    "v1",
		"dcs:sourceSystemType": "template-repository",
	}
	if template.DocumentNumber != nil && *template.DocumentNumber != "" {
		metadata["dcs:documentNumber"] = *template.DocumentNumber
	}
	if template.Name != nil && *template.Name != "" {
		metadata["dcs:name"] = *template.Name
	}
	if template.Description != nil && *template.Description != "" {
		metadata["dcs:description"] = *template.Description
	}
	if uuid := uuidURNFromDID(template.DID); uuid != "" {
		metadata["dcs:uuid"] = uuid
	}
	return metadata
}

func buildContractMetadata(contract contractdb.Contract, sourceTemplate templatedb.ContractTemplate, profile OntologyProfile) map[string]any {
	metadata := map[string]any{
		"@id":                       stableBaseID(contract.DID) + "#metadata",
		"@type":                     "dcs:ContractMetadata",
		"dcs:did":                   contract.DID,
		"dcs:contractVersion":       contract.ContractVersion,
		"dcs:state":                 contract.State,
		"dcs:lifecycleState":        semanticLifecycleState(contract.State),
		"dcs:createdBy":             contract.CreatedBy,
		"dcs:createdAt":             contract.CreatedAt.UTC().Format(time.RFC3339),
		"dcs:updatedAt":             contract.UpdatedAt.UTC().Format(time.RFC3339),
		"dcs:derivedFromTemplate":   sourceTemplate.DID,
		"dcs:sourceTemplateVersion": sourceTemplate.Version,
		"dcs:semanticProfile":       buildSemanticProfile(profile),
	}
	if contract.Name != nil && *contract.Name != "" {
		metadata["dcs:name"] = *contract.Name
	}
	if contract.Description != nil && *contract.Description != "" {
		metadata["dcs:description"] = *contract.Description
	}
	if contract.StartDate != nil {
		metadata["dcs:validFrom"] = contract.StartDate.UTC().Format(time.RFC3339)
	}
	if contract.ExpDate != nil {
		metadata["dcs:validUntil"] = contract.ExpDate.UTC().Format(time.RFC3339)
	}
	if uuid := uuidURNFromDID(contract.DID); uuid != "" {
		metadata["dcs:uuid"] = uuid
	}
	return metadata
}

func buildDocumentStructure(baseID string, inner map[string]any) map[string]any {
	outline, blocks := readDocumentParts(inner)
	referenced := referencedBlockIDs(outline)
	blockByID := map[string]map[string]any{}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		blockID, _ := block["blockId"].(string)
		if strings.TrimSpace(blockID) == "" || !referenced[blockID] {
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "CLAUSE":
			if isEmptyClause(block) {
				continue
			}
		case "SECTION":
			if isEmptySection(blockID, outline) {
				continue
			}
		}
		blockByID[blockID] = block
	}

	sections := []any{}
	clauses := []any{}
	for index, rawNode := range outline {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		blockID, _ := node["blockId"].(string)
		if strings.TrimSpace(blockID) == "" {
			continue
		}
		if isTrue(node["isRoot"]) {
			sections = append(sections, map[string]any{
				"@id":          blockIRI(baseID, blockID),
				"@type":        "dcs:Section",
				"dcs:isRoot":   true,
				"dcs:order":    index + 1,
				"dcs:children": childRefs(baseID, node, blockByID),
			})
			continue
		}
		block := blockByID[blockID]
		if block == nil {
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "SECTION":
			section := map[string]any{
				"@id":          blockIRI(baseID, blockID),
				"@type":        "dcs:Section",
				"dcs:order":    index + 1,
				"dcs:children": childRefs(baseID, node, blockByID),
			}
			if title, _ := block["title"].(string); strings.TrimSpace(title) != "" {
				section["dcs:title"] = title
			}
			sections = append(sections, section)
		case "CLAUSE":
			clause := map[string]any{
				"@id":              blockIRI(baseID, blockID),
				"@type":            "dcs:Clause",
				"dcs:order":        index + 1,
				"dcs:text":         blockText(block),
				"dcs:placeholders": placeholdersForBlock(baseID, block),
			}
			if title, _ := block["title"].(string); strings.TrimSpace(title) != "" {
				clause["dcs:title"] = title
			}
			clauses = append(clauses, clause)
		}
	}
	documentStructure := map[string]any{
		"@id":          baseID + "#document-structure",
		"@type":        "dcs:DocumentStructure",
		"dcs:sections": sections,
		"dcs:clauses":  clauses,
	}
	if subTemplates := buildSubTemplateDocumentStructures(inner); len(subTemplates) > 0 {
		documentStructure["dcs:subTemplates"] = subTemplates
	}
	return documentStructure
}

func buildContractData(baseID string, inner map[string]any) []any {
	if graph := graphFromContent(inner["content"]); len(graph) > 0 {
		result := append([]any{}, graph...)
		for _, subTemplate := range readSubTemplateSnapshots(inner) {
			result = append(result, buildContractDataForSubTemplate(subTemplate)...)
		}
		return result
	}
	requirements := readRequirements(inner)
	result := []any{}
	for _, rawRequirement := range requirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, contractDataObjectsForRequirement(baseID, requirement)...)
	}
	for _, subTemplate := range readSubTemplateSnapshots(inner) {
		result = append(result, buildContractDataForSubTemplate(subTemplate)...)
	}
	return result
}

func buildPolicies(baseID string, inner map[string]any) []any {
	requirements := readRequirements(inner)
	policies := []any{}
	for _, rawRequirement := range requirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := requirement["conditionId"].(string)
		parameters, _ := asArray(requirement["parameters"])
		for _, rawParam := range parameters {
			param, ok := rawParam.(map[string]any)
			if !ok {
				continue
			}
			parameterName, _ := param["parameterName"].(string)
			operators, _ := asArray(param["operators"])
			for _, rawOperator := range operators {
				operate, targets := parseOperator(rawOperator)
				odrlOperator := odrlOperatorFor(operate)
				if odrlOperator == "" {
					continue
				}
				rightOperand := any(targets)
				if len(targets) == 1 && !isSetOperator(operate) {
					rightOperand = targets[0]
				}
				policyID := baseID + "#policy-" + slugify(conditionID) + "-" + slugify(parameterName) + "-" + slugify(operate)
				policies = append(policies, map[string]any{
					"@id":   policyID,
					"@type": "odrl:Duty",
					"odrl:constraint": map[string]any{
						"@type":             "odrl:Constraint",
						"odrl:leftOperand":  map[string]any{"@id": fieldIRI(baseID, conditionID, parameterName)},
						"odrl:operator":     map[string]any{"@id": odrlOperator},
						"odrl:rightOperand": rightOperand,
					},
				})
			}
		}
	}
	for _, subTemplate := range readSubTemplateSnapshots(inner) {
		result := buildPoliciesForSubTemplate(subTemplate)
		policies = append(policies, result...)
	}
	return policies
}

func buildSubTemplateDocumentStructures(inner map[string]any) []any {
	subTemplates := []any{}
	for _, snapshot := range readSubTemplateSnapshots(inner) {
		did, _ := snapshot["did"].(string)
		templateData, _ := snapshot["template_data"].(map[string]any)
		if strings.TrimSpace(did) == "" || templateData == nil {
			continue
		}
		subBaseID := stableBaseID(did)
		subTemplates = append(subTemplates, map[string]any{
			"@id":                   subBaseID,
			"@type":                 "dcs:SubTemplate",
			"dcs:documentStructure": buildDocumentStructure(subBaseID, templateData),
		})
	}
	return subTemplates
}

func buildContractDataForSubTemplate(snapshot map[string]any) []any {
	did, _ := snapshot["did"].(string)
	templateData, _ := snapshot["template_data"].(map[string]any)
	if strings.TrimSpace(did) == "" || templateData == nil {
		return nil
	}
	return buildContractData(stableBaseID(did), templateData)
}

func buildPoliciesForSubTemplate(snapshot map[string]any) []any {
	did, _ := snapshot["did"].(string)
	templateData, _ := snapshot["template_data"].(map[string]any)
	if strings.TrimSpace(did) == "" || templateData == nil {
		return nil
	}
	return buildPolicies(stableBaseID(did), templateData)
}

func readDocumentParts(inner map[string]any) ([]any, []any) {
	if doc, ok := inner["document"].(map[string]any); ok {
		outline, _ := asArray(doc["outline"])
		blocks, _ := asArray(doc["blocks"])
		return outline, blocks
	}
	outline, _ := asArray(inner["documentOutline"])
	blocks, _ := asArray(inner["documentBlocks"])
	return outline, blocks
}

func readRequirements(inner map[string]any) []any {
	if requirements, ok := asArray(inner["requirements"]); ok {
		return requirements
	}
	conditions, _ := asArray(inner["semanticConditions"])
	return conditions
}

func readSubTemplateSnapshots(inner map[string]any) []map[string]any {
	rawSnapshots, _ := asArray(inner["subTemplateSnapshots"])
	snapshots := []map[string]any{}
	for _, rawSnapshot := range rawSnapshots {
		if snapshot, ok := rawSnapshot.(map[string]any); ok {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots
}

func graphFromContent(raw any) []any {
	switch value := raw.(type) {
	case []any:
		return value
	case map[string]any:
		if graph, ok := asArray(value["@graph"]); ok {
			return graph
		}
		if statements, ok := asArray(value["statements"]); ok {
			return statements
		}
		if len(value) > 0 {
			return []any{value}
		}
	}
	return nil
}

func contractDataObjectsForRequirement(baseID string, requirement map[string]any) []any {
	conditionID, _ := requirement["conditionId"].(string)
	if strings.TrimSpace(conditionID) == "" {
		return nil
	}
	item := map[string]any{
		"@id":   contractObjectIRI(baseID, conditionID, requirement),
		"@type": contractObjectType(requirement),
	}
	if name, _ := requirement["conditionName"].(string); strings.TrimSpace(name) != "" {
		item["dcs:name"] = name
	}
	if role := contractRole(requirement); role != "" {
		item["dcs:role"] = map[string]any{"@id": "dcs:" + role}
		item["dcs:partyRef"] = map[string]any{"@id": baseID + "#" + lowerFirst(role) + "Party"}
	}
	parameters, _ := asArray(requirement["parameters"])
	for _, rawParam := range parameters {
		param, ok := rawParam.(map[string]any)
		if !ok {
			continue
		}
		parameterName, _ := param["parameterName"].(string)
		if strings.TrimSpace(parameterName) == "" {
			continue
		}
		property := fieldProperty(param, parameterName)
		item[property] = map[string]any{"@id": fieldIRI(baseID, conditionID, parameterName)}
	}
	return []any{item}
}

func contractObjectIRI(baseID string, conditionID string, requirement map[string]any) string {
	if role := contractRole(requirement); role != "" {
		return baseID + "#" + lowerFirst(role)
	}
	if isPaymentRequirement(conditionID, requirement) {
		return baseID + "#payment"
	}
	return baseID + "#" + slugify(conditionID)
}

func contractObjectType(requirement map[string]any) string {
	if contractRole(requirement) != "" {
		return "dcs:ContractPartyRole"
	}
	entityType, _ := requirement["entityType"].(string)
	if strings.TrimSpace(entityType) != "" {
		return "dcs:" + upperFirst(slugifyCamel(entityType))
	}
	conditionID, _ := requirement["conditionId"].(string)
	if isPaymentRequirement(conditionID, requirement) {
		return "dcs:PaymentTerm"
	}
	return "dcs:ContractDataObject"
}

func contractRole(requirement map[string]any) string {
	role, _ := requirement["entityRole"].(string)
	if strings.TrimSpace(role) == "" {
		return ""
	}
	role = slugifyCamel(role)
	return upperFirst(role)
}

func isPaymentRequirement(conditionID string, requirement map[string]any) bool {
	if strings.Contains(strings.ToLower(conditionID), "payment") {
		return true
	}
	parameters, _ := asArray(requirement["parameters"])
	for _, rawParam := range parameters {
		param, ok := rawParam.(map[string]any)
		if !ok {
			continue
		}
		semanticPath, _ := param["semanticPath"].(string)
		if strings.HasPrefix(semanticPath, "contract.payment.") {
			return true
		}
	}
	return false
}

func fieldProperty(param map[string]any, parameterName string) string {
	semanticPath, _ := param["semanticPath"].(string)
	leaf := parameterName
	if index := strings.LastIndex(semanticPath, "."); index >= 0 && index < len(semanticPath)-1 {
		leaf = semanticPath[index+1:]
	}
	return "dcs:" + lowerFirst(slugifyCamel(leaf))
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func slugifyCamel(value string) string {
	parts := regexp.MustCompile(`[^a-zA-Z0-9]+`).Split(strings.TrimSpace(value), -1)
	if len(parts) == 0 {
		return "field"
	}
	result := ""
	for index, part := range parts {
		if part == "" {
			continue
		}
		part = strings.ToLower(part[:1]) + part[1:]
		if index == 0 && result == "" {
			result = part
			continue
		}
		result += strings.ToUpper(part[:1]) + part[1:]
	}
	if result == "" {
		return "field"
	}
	return result
}

func placeholdersForBlock(baseID string, block map[string]any) []any {
	refs := []any{}
	conditionIDs := stringSetFromAny(block["conditionIds"])
	for _, match := range placeholderPattern.FindAllStringSubmatch(blockText(block), -1) {
		if len(match) != 3 {
			continue
		}
		conditionID := match[1]
		parameterName := match[2]
		if len(conditionIDs) > 0 && !conditionIDs[conditionID] {
			continue
		}
		refs = append(refs, map[string]any{
			"@type":       "dcs:Placeholder",
			"dcs:label":   placeholderLabel(conditionID, parameterName),
			"dcs:token":   match[0],
			"dcs:bindsTo": map[string]any{"@id": fieldIRI(baseID, conditionID, parameterName)},
		})
	}
	return refs
}

func placeholderLabel(conditionID string, parameterName string) string {
	words := strings.Fields(strings.ReplaceAll(conditionID+" "+parameterName, "-", " "))
	for index, word := range words {
		words[index] = upperFirst(word)
	}
	return strings.Join(words, " ")
}

var placeholderPattern = regexp.MustCompile(`\{\{([^}.]+)\.([^}]+)\}\}`)

func referencedBlockIDs(outline []any) map[string]bool {
	result := map[string]bool{}
	for _, rawNode := range outline {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if blockID, _ := node["blockId"].(string); strings.TrimSpace(blockID) != "" {
			result[blockID] = true
		}
		if children, ok := asArray(node["children"]); ok {
			for _, rawChild := range children {
				child, ok := rawChild.(string)
				if ok && strings.TrimSpace(child) != "" {
					result[child] = true
				}
			}
		}
	}
	return result
}

func childRefs(baseID string, node map[string]any, blocks map[string]map[string]any) []any {
	children, _ := asArray(node["children"])
	refs := []any{}
	for _, rawChild := range children {
		childID, ok := rawChild.(string)
		if !ok || blocks[childID] == nil {
			continue
		}
		refs = append(refs, map[string]any{"@id": blockIRI(baseID, childID)})
	}
	return refs
}

func isEmptyClause(block map[string]any) bool {
	return strings.TrimSpace(blockText(block)) == "" && len(contentArray(block)) == 0
}

func isEmptySection(blockID string, outline []any) bool {
	for _, rawNode := range outline {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		id, _ := node["blockId"].(string)
		if id != blockID {
			continue
		}
		children, _ := asArray(node["children"])
		return len(children) == 0
	}
	return true
}

func blockText(block map[string]any) string {
	if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
		return text
	}
	if content := contentArray(block); len(content) > 0 {
		parts := []string{}
		for _, item := range content {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func contentArray(block map[string]any) []any {
	content, _ := asArray(block["content"])
	if len(content) == 0 {
		content, _ = asArray(block["dcs:content"])
	}
	return content
}

func stringSetFromAny(raw any) map[string]bool {
	values, _ := asArray(raw)
	result := map[string]bool{}
	for _, item := range values {
		value, ok := item.(string)
		if ok && strings.TrimSpace(value) != "" {
			result[value] = true
		}
	}
	return result
}

func parseOperator(raw any) (string, []any) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case map[string]any:
		operate, _ := value["operate"].(string)
		if strings.TrimSpace(operate) == "" {
			operate, _ = value["operator"].(string)
		}
		if strings.TrimSpace(operate) == "" {
			operate = resourceID(value["odrl:operator"])
		}
		if targets, ok := asArray(value["targets"]); ok {
			return operate, targets
		}
		if rightOperand, exists := value["rightOperand"]; exists {
			if values, ok := asArray(rightOperand); ok {
				return operate, values
			}
			return operate, []any{rightOperand}
		}
		if rightOperand, exists := value["odrl:rightOperand"]; exists {
			if values, ok := asArray(rightOperand); ok {
				return operate, values
			}
			return operate, []any{rightOperand}
		}
		return operate, nil
	default:
		return "", nil
	}
}

func resourceID(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		id, _ := typed["@id"].(string)
		return id
	default:
		return ""
	}
}

func odrlOperatorFor(operator string) string {
	switch operator {
	case "Equals", "odrl:eq":
		return "odrl:eq"
	case "NotEquals", "odrl:neq":
		return "odrl:neq"
	case "In", "odrl:isAnyOf":
		return "odrl:isAnyOf"
	case "NotIn", "odrl:isNoneOf":
		return "odrl:isNoneOf"
	case "GreaterThan", "odrl:gt":
		return "odrl:gt"
	case "GreaterThanOrEqual", "odrl:gteq":
		return "odrl:gteq"
	case "LessThan", "odrl:lt":
		return "odrl:lt"
	case "LessThanOrEqual", "odrl:lteq":
		return "odrl:lteq"
	case "Contains", "odrl:hasPart":
		return "odrl:hasPart"
	default:
		return ""
	}
}

func isSetOperator(operator string) bool {
	return operator == "In" || operator == "NotIn" || operator == "odrl:isAnyOf" || operator == "odrl:isNoneOf"
}

func blockIRI(baseID string, blockID string) string {
	return baseID + "#block-" + slugify(blockID)
}

func fieldIRI(baseID string, conditionID string, parameterName string) string {
	condition := lowerFirst(slugifyCamel(conditionID))
	parameter := slugifyCamel(parameterName)
	return baseID + "#" + condition + upperFirst(parameter)
}

func stableBaseID(id string) string {
	return strings.TrimRight(id, "#")
}

func slugify(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(value, "${1}-${2}")
	value = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "unnamed"
	}
	return strings.ToLower(value)
}

// buildSemanticProfile builds the semanticProfile descriptor for a given profile.
func buildSemanticProfile(p OntologyProfile) map[string]any {
	return map[string]any{
		"name":     p.Name,
		"version":  p.Version,
		"context":  p.ContextURL,
		"ontology": p.OntologyURL,
		"shapes":   p.ShapesURL,
	}
}

// standardSemanticProfile returns the default FACIS DCS v1 semantic profile descriptor.
// Used by test fixtures that build stored JSONB; the mapper itself uses buildSemanticProfile.
func standardSemanticProfile() map[string]any {
	return buildSemanticProfile(DefaultProfile())
}

// semanticLifecycleState maps a DCS DB contract state to the JSON-LD ontology
// lifecycle state as defined in docs/semantic-ontology/README.md §15.
func semanticLifecycleState(state string) string {
	switch state {
	case "DRAFT":
		return "Draft"
	case "NEGOTIATION":
		return "InNegotiation"
	case "SUBMITTED":
		return "SubmittedForReview"
	case "REVIEWED":
		return "Reviewed"
	case "APPROVED":
		return "Approved"
	case "TERMINATED":
		return "Terminated"
	case "EXPIRED":
		return "Expired"
	default:
		return state
	}
}

// parseJSONB decodes a *datatype.JSON JSONB value into a generic map.
// Returns an empty (non-nil) map when raw is nil or JSON null.
func parseJSONB(raw *datatype.JSON) (map[string]any, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(*raw, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

func asArray(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func isTrue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

// encodeMap serializes a generic map to a *datatype.JSON value.
func encodeMap(data map[string]any) (*datatype.JSON, error) {
	j, err := datatype.NewJSON(data)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

var uuidHexPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// uuidURNFromDID attempts to extract a UUID URN from a UUID-like segment embedded
// in the DID. Returns an empty string when no UUID segment is present.
func uuidURNFromDID(did string) string {
	if match := uuidHexPattern.FindString(strings.ToLower(did)); match != "" {
		return "urn:uuid:" + match
	}
	return ""
}
