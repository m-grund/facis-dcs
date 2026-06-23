// Package mapper builds interoperable JSON-LD envelopes from DCS database rows.
//
// The exported JSON-LD separates metadata, human-readable document structure,
// machine-readable contract data, and ODRL policies. The mapper remains read-only:
// it projects stored JSONB into the public JSON-LD shape without mutating storage.
package mapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"
)

const (
	dcsContextV1  = "https://w3id.org/facis/dcs/ontology/v1#"
	odrlContextV2 = "http://www.w3.org/ns/odrl/2/"
	xsdContext    = "http://www.w3.org/2001/XMLSchema#"
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
	"contractData":        true,
}

// BuildTemplateJSONLD assembles the public ContractTemplate JSON-LD envelope.
func BuildTemplateJSONLD(template templatedb.ContractTemplate, profile OntologyProfile) (map[string]any, error) {
	inner, err := parseJSONB(template.TemplateData)
	if err != nil {
		return nil, fmt.Errorf("parse template_data: %w", err)
	}
	if isCanonicalJSONLDEnvelope(inner) {
		inner["@context"] = map[string]any{"dcs": dcsContextV1, "odrl": odrlContextV2, "xsd": xsdContext}
		inner["@id"] = template.DID
		inner["@type"] = "dcs:ContractTemplate"
		inner["dcs:metadata"] = mergeMetadata(inner["dcs:metadata"], buildTemplateMetadata(template))
		return inner, nil
	}

	baseID := stableBaseID(template.DID)
	envelope := map[string]any{
		"@context": map[string]any{
			"dcs":  dcsContextV1,
			"odrl": odrlContextV2,
			"xsd":  xsdContext,
		},
		"@id":                   template.DID,
		"@type":                 "dcs:ContractTemplate",
		"dcs:metadata":          buildTemplateMetadata(template),
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
	if isCanonicalJSONLDEnvelope(inner) {
		if err := expandApprovedTemplateSnapshots(inner, stableBaseID(contract.DID)); err != nil {
			return nil, fmt.Errorf("expand approved template snapshots: %w", err)
		}
		inner["@context"] = map[string]any{"dcs": dcsContextV1, "odrl": odrlContextV2, "xsd": xsdContext}
		inner["@id"] = contract.DID
		inner["@type"] = "dcs:Contract"
		inner["dcs:metadata"] = mergeMetadata(inner["dcs:metadata"], buildContractMetadata(contract, sourceTemplate))
		if err := materializeCanonicalContractData(
			inner,
			stableBaseID(contract.DID),
			isFinalContractState(contract.State),
		); err != nil {
			return nil, fmt.Errorf("materialize canonical contract data: %w", err)
		}
		cleanupPublishedContractEnvelope(inner)
		return inner, nil
	}

	baseID := stableBaseID(contract.DID)
	envelope := map[string]any{
		"@context": map[string]any{
			"dcs":  dcsContextV1,
			"odrl": odrlContextV2,
			"xsd":  xsdContext,
		},
		"@id":                   contract.DID,
		"@type":                 "dcs:Contract",
		"dcs:metadata":          buildContractMetadata(contract, sourceTemplate),
		"dcs:documentStructure": buildDocumentStructure(baseID, inner),
		"dcs:contractData":      buildContractData(baseID, inner),
		"dcs:policies":          buildPolicies(baseID, inner),
	}

	return envelope, nil
}

// MaterializeStoredContractJSONLD finalizes a stored workflow contract through
// the same mapper used by direct contract JSON-LD generation. Source-template
// identity is read from the stored contract envelope.
func MaterializeStoredContractJSONLD(contract contractdb.Contract, profile OntologyProfile) (map[string]any, error) {
	inner, err := parseJSONB(contract.ContractData)
	if err != nil {
		return nil, fmt.Errorf("parse stored contract_data: %w", err)
	}
	sourceDID, sourceVersion := storedSourceTemplate(inner)
	sourceTemplate := templatedb.ContractTemplate{
		DID:     sourceDID,
		Version: sourceVersion,
	}
	return BuildContractJSONLD(contract, sourceTemplate, profile)
}

func storedSourceTemplate(data map[string]any) (string, int) {
	var did string
	var version int
	if source, ok := data["sourceTemplate"].(map[string]any); ok {
		did, _ = source["did"].(string)
		version = intFromAny(source["version"])
	}
	if did == "" {
		did, _ = data["derivedFromTemplate"].(string)
	}
	if metadata, ok := data["dcs:metadata"].(map[string]any); ok {
		if did == "" {
			did = resourceID(metadata["dcs:derivedFromTemplate"])
		}
		if version == 0 {
			version = intFromAny(metadata["dcs:sourceTemplateVersion"])
		}
		if version == 0 {
			version = intFromAny(metadata["dcs:templateVersion"])
		}
	}
	return did, version
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := strconv.Atoi(string(typed))
		return parsed
	default:
		return 0
	}
}

func isCanonicalJSONLDEnvelope(value map[string]any) bool {
	_, hasDocumentStructure := value["dcs:documentStructure"]
	return hasDocumentStructure
}

func expandApprovedTemplateSnapshots(data map[string]any, baseID string) error {
	metadata, _ := data["dcs:metadata"].(map[string]any)
	rawSnapshots, _ := asArray(metadata["dcs:subTemplates"])
	snapshots := map[string]map[string]any{}
	for _, rawSnapshot := range rawSnapshots {
		snapshot, ok := rawSnapshot.(map[string]any)
		if !ok {
			continue
		}
		templateDID, _ := snapshot["@id"].(string)
		if _, ok := snapshot["dcs:template"].(map[string]any); ok && templateDID != "" {
			snapshots[templateDID] = snapshot
			if version := intFromAny(snapshot["dcs:version"]); version > 0 {
				snapshots[templateSnapshotKey(templateDID, version)] = snapshot
			}
		}
	}

	structure, _ := data["dcs:documentStructure"].(map[string]any)
	rawBlocks, _ := asArray(structure["dcs:blocks"])
	rawLayout, _ := asArray(structure["dcs:layout"])
	contractData, _ := asArray(data["dcs:contractData"])
	policies, _ := asArray(data["dcs:policies"])

	expandedBlocks := make([]any, 0, len(rawBlocks))
	expandedLayout := append([]any(nil), rawLayout...)
	for _, rawBlock := range rawBlocks {
		block, ok := rawBlock.(map[string]any)
		if !ok || block["@type"] != "dcs:ApprovedTemplate" {
			expandedBlocks = append(expandedBlocks, rawBlock)
			continue
		}
		templateDID, _ := block["dcs:templateDid"].(string)
		snapshot := snapshots[templateSnapshotKey(templateDID, intFromAny(block["dcs:version"]))]
		if snapshot == nil {
			snapshot = snapshots[templateDID]
		}
		template, ok := snapshot["dcs:template"].(map[string]any)
		if !ok {
			return fmt.Errorf("approved template block references missing snapshot %q", templateDID)
		}
		nested, err := cloneJSONMap(template)
		if err != nil {
			return fmt.Errorf("clone approved template %q: %w", templateDID, err)
		}
		if err := expandApprovedTemplateSnapshots(nested, baseID); err != nil {
			return err
		}
		ownerID, _ := block["@id"].(string)
		ownerFragment := localIRI(ownerID)
		rebaseNestedTemplateIDs(nested, templateDID, baseID, ownerFragment)

		nestedStructure, _ := nested["dcs:documentStructure"].(map[string]any)
		nestedBlocks, _ := asArray(nestedStructure["dcs:blocks"])
		nestedLayout, _ := asArray(nestedStructure["dcs:layout"])
		rootChildren := nestedRootChildren(nestedLayout)

		section := map[string]any{
			"@id":   ownerID,
			"@type": "dcs:Section",
		}
		if title, _ := snapshot["dcs:name"].(string); strings.TrimSpace(title) != "" {
			section["dcs:title"] = title
		}
		expandedBlocks = append(expandedBlocks, section)
		expandedBlocks = append(expandedBlocks, nestedBlocks...)
		replaceLayoutChildren(expandedLayout, ownerID, rootChildren)
		for _, rawNode := range nestedLayout {
			node, ok := rawNode.(map[string]any)
			if !ok || isTrue(node["dcs:isRoot"]) {
				continue
			}
			expandedLayout = append(expandedLayout, node)
		}
		nestedData, _ := asArray(nested["dcs:contractData"])
		nestedPolicies, _ := asArray(nested["dcs:policies"])
		contractData = append(contractData, nestedData...)
		policies = append(policies, nestedPolicies...)
	}

	structure["dcs:blocks"] = expandedBlocks
	structure["dcs:layout"] = expandedLayout
	data["dcs:contractData"] = contractData
	data["dcs:policies"] = policies
	if metadata != nil {
		delete(metadata, "dcs:subTemplates")
	}
	canonicalizeContractFieldIDs(data, baseID)
	return nil
}

func templateSnapshotKey(did string, version int) string {
	return fmt.Sprintf("%s\x00%d", did, version)
}

func cloneJSONMap(value map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func rebaseNestedTemplateIDs(value any, templateDID string, baseID string, ownerFragment string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok && strings.HasPrefix(text, templateDID+"#") {
				fragment := strings.TrimPrefix(text, templateDID+"#")
				typed[key] = baseID + "#" + ownerFragment + "-" + fragment
				continue
			}
			rebaseNestedTemplateIDs(nested, templateDID, baseID, ownerFragment)
		}
	case []any:
		for _, nested := range typed {
			rebaseNestedTemplateIDs(nested, templateDID, baseID, ownerFragment)
		}
	}
}

func localIRI(id string) string {
	if index := strings.LastIndex(id, "#"); index >= 0 && index < len(id)-1 {
		return id[index+1:]
	}
	return slugify(id)
}

func nestedRootChildren(layout []any) []any {
	for _, rawNode := range layout {
		node, ok := rawNode.(map[string]any)
		if !ok || !isTrue(node["dcs:isRoot"]) {
			continue
		}
		children, _ := jsonLDListValues(node["dcs:children"])
		return append([]any(nil), children...)
	}
	return nil
}

func jsonLDListValues(value any) ([]any, bool) {
	container, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return asArray(container["@list"])
}

func replaceLayoutChildren(layout []any, nodeID string, children []any) {
	for _, rawNode := range layout {
		node, ok := rawNode.(map[string]any)
		if !ok || node["@id"] != nodeID {
			continue
		}
		node["dcs:children"] = map[string]any{"@list": children}
		return
	}
}

func canonicalizeContractFieldIDs(data map[string]any, baseID string) {
	rawRequirements, _ := asArray(data["dcs:contractData"])
	replacements := map[string]string{}
	for _, rawRequirement := range rawRequirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok || requirement["@type"] != "dcs:DataRequirement" {
			continue
		}
		conditionID, _ := requirement["dcs:conditionId"].(string)
		fields, _ := asArray(requirement["dcs:fields"])
		for _, rawField := range fields {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			parameterName, _ := field["dcs:parameterName"].(string)
			oldID, _ := field["@id"].(string)
			newID := baseID + "#field-" + slugify(conditionID) + "-" + slugifyCamel(parameterName)
			if oldID != "" {
				replacements[oldID] = newID
			}
			field["@id"] = newID
		}
	}
	replaceExactIRIReferences(data, replacements)
}

func replaceExactIRIReferences(value any, replacements map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				if replacement := replacements[text]; replacement != "" {
					typed[key] = replacement
					continue
				}
			}
			replaceExactIRIReferences(nested, replacements)
		}
	case []any:
		for _, nested := range typed {
			replaceExactIRIReferences(nested, replacements)
		}
	}
}

func materializeCanonicalContractData(data map[string]any, baseID string, requireComplete bool) error {
	rawRequirements, _ := asArray(data["dcs:contractData"])
	rawValues, _ := asArray(data["semanticConditionValues"])
	hasRequirements := false
	for _, rawRequirement := range rawRequirements {
		requirement, ok := rawRequirement.(map[string]any)
		if ok && requirement["@type"] == "dcs:DataRequirement" {
			hasRequirements = true
			break
		}
	}
	if !hasRequirements {
		delete(data, "semanticConditionValues")
		return nil
	}

	valuesByCondition := map[string][]map[string]any{}
	for index, rawValue := range rawValues {
		value, ok := rawValue.(map[string]any)
		if !ok {
			return fmt.Errorf("semanticConditionValues.%d must be an object", index)
		}
		conditionID, _ := value["conditionId"].(string)
		parameterName, _ := value["parameterName"].(string)
		if strings.TrimSpace(conditionID) == "" || strings.TrimSpace(parameterName) == "" {
			return fmt.Errorf("semanticConditionValues.%d requires conditionId and parameterName", index)
		}
		valuesByCondition[conditionID] = append(valuesByCondition[conditionID], value)
	}

	valuesByFieldID := map[string]any{}
	domainFieldByFieldID := map[string]string{}
	contractFields := make([]any, 0)
	materialized := make([]any, 0, len(rawRequirements))
	for _, rawRequirement := range rawRequirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok || requirement["@type"] != "dcs:DataRequirement" {
			materialized = append(materialized, rawRequirement)
			continue
		}
		item, err := materializeDataRequirement(baseID, requirement, valuesByCondition)
		if err != nil {
			return err
		}
		fields, err := materializeContractFields(
			requirement,
			item,
			valuesByCondition,
			valuesByFieldID,
			domainFieldByFieldID,
		)
		if err != nil {
			return err
		}
		contractFields = append(contractFields, fields...)
		materialized = append(materialized, item)
	}

	if err := validateDocumentPlaceholderBindings(data, valuesByFieldID, requireComplete); err != nil {
		return err
	}
	if requireComplete {
		if err := validateMaterializedPolicyOperands(data["dcs:policies"], valuesByFieldID); err != nil {
			return err
		}
	}
	typePolicyOperands(data["dcs:policies"], domainFieldByFieldID)
	data["dcs:contractData"] = materialized
	data["dcs:contractFields"] = contractFields
	delete(data, "semanticConditionValues")
	return nil
}

func validateMaterializedPolicyOperands(policies any, valuesByFieldID map[string]any) error {
	rawPolicies, _ := asArray(policies)
	for index, rawPolicy := range rawPolicies {
		policy, ok := rawPolicy.(map[string]any)
		if !ok {
			continue
		}
		constraint, _ := policy["odrl:constraint"].(map[string]any)
		leftOperand, _ := constraint["odrl:leftOperand"].(map[string]any)
		fieldID, _ := leftOperand["@id"].(string)
		if strings.TrimSpace(fieldID) == "" {
			continue
		}
		if _, found := valuesByFieldID[fieldID]; !found {
			return fmt.Errorf("final contract policy %d references non-materialized field %q", index, fieldID)
		}
	}
	return nil
}

func materializeContractFields(
	requirement map[string]any,
	sourceObject map[string]any,
	valuesByCondition map[string][]map[string]any,
	valuesByFieldID map[string]any,
	domainFieldByFieldID map[string]string,
) ([]any, error) {
	conditionID, _ := requirement["dcs:conditionId"].(string)
	sourceObjectID, _ := sourceObject["@id"].(string)
	fields, _ := asArray(requirement["dcs:fields"])
	result := make([]any, 0, len(fields))
	for _, rawField := range fields {
		field, ok := rawField.(map[string]any)
		if !ok {
			continue
		}
		fieldID, _ := field["@id"].(string)
		parameterName, _ := field["dcs:parameterName"].(string)
		semanticPath, _ := field["dcs:semanticPath"].(string)
		value, found, err := canonicalFieldValue(valuesByCondition[conditionID], parameterName, semanticPath)
		if err != nil {
			return nil, fmt.Errorf("condition %q field %q: %w", conditionID, parameterName, err)
		}
		if found && strings.TrimSpace(fieldID) != "" {
			valuesByFieldID[fieldID] = value
			domainFieldID := semanticPath
			if domainField, ok := field["dcs:domainField"].(map[string]any); ok {
				if id, _ := domainField["@id"].(string); id != "" {
					domainFieldID = id
					domainFieldByFieldID[fieldID] = id
				}
			}
			contractField := map[string]any{
				"@id":              fieldID,
				"@type":            "dcs:ContractField",
				"dcs:sourceObject": map[string]any{"@id": sourceObjectID},
				"dcs:path":         canonicalFieldProperty(parameterName, semanticPath),
			}
			if dataType := validation.SemanticDataType(domainFieldID); dataType != "" {
				contractField["dcs:dataType"] = map[string]any{"@id": dataType}
			}
			if domainField, ok := field["dcs:domainField"].(map[string]any); ok {
				contractField["dcs:domainField"] = domainField
			}
			result = append(result, contractField)
		}
	}
	return result, nil
}

func validateDocumentPlaceholderBindings(
	data map[string]any,
	valuesByFieldID map[string]any,
	requireComplete bool,
) error {
	structure, _ := data["dcs:documentStructure"].(map[string]any)
	blocks, _ := asArray(structure["dcs:blocks"])
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		content, ok := block["dcs:content"].(map[string]any)
		if !ok {
			continue
		}
		segments, ok := asArray(content["@list"])
		if !ok {
			continue
		}
		for _, rawSegment := range segments {
			placeholder, ok := rawSegment.(map[string]any)
			if !ok || placeholder["@type"] != "dcs:Placeholder" {
				continue
			}
			bindsTo, _ := placeholder["dcs:bindsTo"].(map[string]any)
			fieldID, _ := bindsTo["@id"].(string)
			_, found := valuesByFieldID[fieldID]
			if !found {
				if requireComplete {
					return fmt.Errorf("final contract placeholder bound to %q has no value", fieldID)
				}
			}
			if requireComplete {
				delete(placeholder, "dcs:token")
			}
		}
	}
	return nil
}

func isFinalContractState(state string) bool {
	switch state {
	case "APPROVED", "SIGNED", "EXECUTED", "TERMINATED", "EXPIRED", "ARCHIVED":
		return true
	default:
		return false
	}
}

func materializeDataRequirement(
	baseID string,
	requirement map[string]any,
	valuesByCondition map[string][]map[string]any,
) (map[string]any, error) {
	conditionID, _ := requirement["dcs:conditionId"].(string)
	if strings.TrimSpace(conditionID) == "" {
		return nil, errors.New("dcs:DataRequirement requires dcs:conditionId")
	}
	entityRole, _ := requirement["dcs:entityRole"].(string)
	entityID := conditionID
	if strings.TrimSpace(entityRole) != "" {
		entityID = entityRole
	}
	entityType, _ := requirement["dcs:entityType"].(string)
	if strings.TrimSpace(entityType) == "" {
		entityType = "dcs:ContractDataObject"
	} else if !strings.Contains(entityType, ":") {
		entityType = "dcs:" + entityType
	}
	item := map[string]any{
		"@id":   baseID + "#" + slugify(entityID),
		"@type": entityType,
	}
	if strings.TrimSpace(entityRole) != "" {
		item["dcs:role"] = map[string]any{
			"@id": "dcs:" + upperFirst(slugifyCamel(entityRole)),
		}
	}

	fields, _ := asArray(requirement["dcs:fields"])
	for _, rawField := range fields {
		field, ok := rawField.(map[string]any)
		if !ok {
			continue
		}
		parameterName, _ := field["dcs:parameterName"].(string)
		semanticPath, _ := field["dcs:semanticPath"].(string)
		value, found, err := canonicalFieldValue(valuesByCondition[conditionID], parameterName, semanticPath)
		if err != nil {
			return nil, fmt.Errorf("condition %q field %q: %w", conditionID, parameterName, err)
		}
		if !found {
			continue
		}
		domainFieldID := semanticPath
		if domainField, ok := field["dcs:domainField"].(map[string]any); ok {
			if id, _ := domainField["@id"].(string); id != "" {
				domainFieldID = id
			}
		}
		item[canonicalFieldProperty(parameterName, semanticPath)] = validation.TypeODRLOperand(domainFieldID, value)
	}
	return item, nil
}

func canonicalFieldValue(
	values []map[string]any,
	parameterName string,
	semanticPath string,
) (any, bool, error) {
	var result any
	found := false
	for _, value := range values {
		candidateName, _ := value["parameterName"].(string)
		if candidateName != parameterName && candidateName != semanticPath {
			continue
		}
		candidate, exists := value["parameterValue"]
		if !exists {
			continue
		}
		if found && !reflect.DeepEqual(result, candidate) {
			return nil, false, errors.New("has conflicting submitted values")
		}
		result = candidate
		found = true
	}
	return result, found, nil
}

func canonicalFieldProperty(parameterName string, semanticPath string) string {
	leaf := parameterName
	if index := strings.LastIndex(semanticPath, "."); index >= 0 && index < len(semanticPath)-1 {
		leaf = semanticPath[index+1:]
	}
	return "dcs:" + lowerFirst(slugifyCamel(leaf))
}

func typePolicyOperands(policies any, domainFieldByFieldID map[string]string) {
	rawPolicies, _ := asArray(policies)
	for _, rawPolicy := range rawPolicies {
		policy, ok := rawPolicy.(map[string]any)
		if !ok {
			continue
		}
		constraint, _ := policy["odrl:constraint"].(map[string]any)
		leftOperand, _ := constraint["odrl:leftOperand"].(map[string]any)
		fieldID, _ := leftOperand["@id"].(string)
		domainField := domainFieldByFieldID[fieldID]
		rawRightOperand, exists := constraint["odrl:rightOperand"]
		if !exists {
			continue
		}
		if values, ok := asArray(rawRightOperand); ok {
			typedValues := make([]any, len(values))
			for index, value := range values {
				typedValues[index] = validation.TypeODRLOperand(domainField, unwrapJSONLDOperand(value))
			}
			constraint["odrl:rightOperand"] = typedValues
			continue
		}
		constraint["odrl:rightOperand"] = validation.TypeODRLOperand(
			domainField,
			unwrapJSONLDOperand(rawRightOperand),
		)
	}
}

func unwrapJSONLDOperand(value any) any {
	if literal, ok := value.(map[string]any); ok {
		if rawValue, exists := literal["@value"]; exists {
			return rawValue
		}
		if id, exists := literal["@id"]; exists {
			return id
		}
	}
	return value
}

func cleanupPublishedContractEnvelope(data map[string]any) {
	allowed := map[string]bool{
		"@context":              true,
		"@id":                   true,
		"@type":                 true,
		"dcs:metadata":          true,
		"dcs:contractData":      true,
		"dcs:contractFields":    true,
		"dcs:documentStructure": true,
		"dcs:policies":          true,
	}
	for key := range data {
		if !allowed[key] {
			delete(data, key)
		}
	}
	if metadata, ok := data["dcs:metadata"].(map[string]any); ok {
		delete(metadata, "dcs:templateType")
		delete(metadata, "dcs:templateVersion")
		delete(metadata, "dcs:subTemplates")
	}
}

func mergeMetadata(existing any, authoritative map[string]any) map[string]any {
	result := map[string]any{}
	if metadata, ok := existing.(map[string]any); ok {
		for key, value := range metadata {
			result[key] = value
		}
	}
	for key, value := range authoritative {
		result[key] = value
	}
	return result
}

func buildTemplateMetadata(template templatedb.ContractTemplate) map[string]any {
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

func buildContractMetadata(contract contractdb.Contract, sourceTemplate templatedb.ContractTemplate) map[string]any {
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
		"dcs:derivedFromTemplate":   map[string]any{"@id": sourceTemplate.DID},
		"dcs:sourceTemplateVersion": sourceTemplate.Version,
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
			semanticPath, _ := param["semanticPath"].(string)
			operators, _ := asArray(param["operators"])
			for _, rawOperator := range operators {
				operate, targets := parseOperator(rawOperator)
				odrlOperator := odrlOperatorFor(operate)
				if odrlOperator == "" {
					continue
				}
				typedTargets := make([]any, 0, len(targets))
				for _, target := range targets {
					typedTargets = append(
						typedTargets,
						validation.TypeODRLOperand(semanticPath, unwrapTypedOperand(target)),
					)
				}
				rightOperand := any(typedTargets)
				if len(targets) == 1 && !isSetOperator(operate) {
					rightOperand = typedTargets[0]
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
			return operate, unwrapTypedOperands(targets)
		}
		if rightOperand, exists := value["rightOperand"]; exists {
			if values, ok := asArray(rightOperand); ok {
				return operate, unwrapTypedOperands(values)
			}
			return operate, []any{unwrapTypedOperand(rightOperand)}
		}
		if rightOperand, exists := value["odrl:rightOperand"]; exists {
			if values, ok := asArray(rightOperand); ok {
				return operate, unwrapTypedOperands(values)
			}
			return operate, []any{unwrapTypedOperand(rightOperand)}
		}
		return operate, nil
	default:
		return "", nil
	}
}

func typedRightOperand(value any, parameterType string) any {
	xsdType := xsdTypeForParameter(parameterType)
	if xsdType == "" {
		return value
	}
	return map[string]any{
		"@value": lexicalOperandValue(unwrapTypedOperand(value)),
		"@type":  xsdType,
	}
}

func xsdTypeForParameter(parameterType string) string {
	switch parameterType {
	case "decimal":
		return "xsd:decimal"
	case "integer":
		return "xsd:integer"
	case "boolean":
		return "xsd:boolean"
	case "date":
		return "xsd:date"
	case "string", "enum":
		return "xsd:string"
	default:
		return ""
	}
}

func lexicalOperandValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func unwrapTypedOperands(values []any) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = unwrapTypedOperand(value)
	}
	return result
}

func unwrapTypedOperand(value any) any {
	literal, ok := value.(map[string]any)
	if !ok {
		return value
	}
	raw, exists := literal["@value"]
	if !exists {
		return value
	}
	text, ok := raw.(string)
	if !ok {
		return raw
	}
	switch literal["@type"] {
	case "xsd:decimal":
		parsed, err := strconv.ParseFloat(text, 64)
		if err == nil {
			return parsed
		}
	case "xsd:integer":
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed
		}
	case "xsd:boolean":
		parsed, err := strconv.ParseBool(text)
		if err == nil {
			return parsed
		}
	}
	return text
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
