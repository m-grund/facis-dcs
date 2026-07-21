package validation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrContractNotClosed is a client-input error (map to 4xx): a contract still
// carries unresolved placeholders and so is not yet a contract. Templates
// (odrl:Offer) may stay open; a contract must be closed before it leaves draft
// and is signed.
var ErrContractNotClosed = errors.New("contract is not closed")

// ValidateContractClosed enforces the SRS contract-completeness invariant. A
// template's ODRL is open — parties, negotiated boundaries and values are
// placeholders resolved during generation and negotiation. A contract must be
// closed: the SRS requires Contract Generation to "fill in the necessary
// placeholders" so the "filled-out contract MUST be ready to be sent to the
// Responder", and Contract Approval to verify "schema completeness". This gate
// runs at approval and again at the signing seal.
//
// A contract is closed when every placeholder the policy relies on is
// materialized: each negotiated-boundary right operand references a filled
// field, each required data field a policy enforces carries a value, and each
// prose placeholder binds to a filled field.
func ValidateContractClosed(contractDocument any) error {
	data, err := normalizeObject(contractDocument)
	if err != nil {
		return fmt.Errorf("decode contract document: %w", err)
	}
	fields := contractFieldValues(data)

	seen := map[string]bool{}
	unresolved := []string{}
	add := func(message string) {
		if !seen[message] {
			seen[message] = true
			unresolved = append(unresolved, message)
		}
	}

	for _, rule := range collectODRLPolicyRules(topLevelValue(data, "policies")) {
		constraints := policyConstraints(rule["odrl:constraint"])
		constraints = append(constraints, dutyConstraints(rule["odrl:duty"])...)
		for _, constraint := range constraints {
			for _, leaf := range compactConstraintLeaves(constraint) {
				// A required data field a policy enforces must carry a value; a
				// context operand (spatial, dateTime, …) is use-time context, not
				// a document field.
				if left := nodeReferenceID(leaf["odrl:leftOperand"]); left != "" && !isODRLContextOperandTerm(left) {
					if info, ok := fields[left]; ok && info.required && !info.hasValue {
						add(fmt.Sprintf("required data field %q has no value", left))
					}
				}
				// A negotiated boundary (a right operand referencing a field)
				// must have its agreed value.
				if right := nodeReferenceID(leaf["odrl:rightOperand"]); right != "" {
					if info, ok := fields[right]; ok && !info.hasValue {
						add(fmt.Sprintf("negotiated boundary %q has no agreed value", right))
					}
				}
			}
		}
	}

	for _, message := range unresolvedProsePlaceholders(data, fields) {
		add(message)
	}

	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return fmt.Errorf("%w: %s", ErrContractNotClosed, strings.Join(unresolved, "; "))
	}
	return nil
}

type contractFieldInfo struct {
	required bool
	hasValue bool
}

// contractFieldValues indexes the document's requirement fields by @id, noting
// whether each is required and carries an inline value (dcs:parameterValue).
func contractFieldValues(data documentData) map[string]contractFieldInfo {
	out := map[string]contractFieldInfo{}
	requirements, _ := topLevelValue(data, "contractData").([]any)
	for _, rawReq := range requirements {
		req, ok := rawReq.(map[string]any)
		if !ok {
			continue
		}
		fields, _ := req["dcs:fields"].([]any)
		for _, rawField := range fields {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			id, _ := field["@id"].(string)
			if id == "" {
				continue
			}
			required, _ := field["dcs:required"].(bool)
			out[id] = contractFieldInfo{required: required, hasValue: hasInlineValue(field)}
		}
	}
	return out
}

// hasInlineValue reports whether a requirement field carries a non-empty
// submitted value (dcs:parameterValue); an absent key or empty string is unset.
func hasInlineValue(field map[string]any) bool {
	value, present := field["dcs:parameterValue"]
	if !present || value == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(value)) != ""
}

// nodeReferenceID returns the @id of a JSON-LD reference node ({"@id": …}); a
// value node ({"@value": …}) or a list is not a reference and returns "".
func nodeReferenceID(value any) string {
	node, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if _, isValue := node["@value"]; isValue {
		return ""
	}
	id, _ := node["@id"].(string)
	return id
}

// unresolvedProsePlaceholders reports document placeholders that bind to a
// field carrying no value — a placeholder that never got materialized.
func unresolvedProsePlaceholders(data documentData, fields map[string]contractFieldInfo) []string {
	messages := []string{}
	structure, ok := topLevelValue(data, "documentStructure").(map[string]any)
	if !ok {
		return messages
	}
	blocks, ok := jsonLDList(structure["dcs:blocks"])
	if !ok {
		blocks, _ = structure["dcs:blocks"].([]any)
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		content, ok := jsonLDList(block["dcs:content"])
		if !ok {
			continue
		}
		for _, rawSegment := range content {
			segment, ok := rawSegment.(map[string]any)
			if !ok || segment["@type"] != "dcs:Placeholder" {
				continue
			}
			bindsTo, _ := segment["dcs:bindsTo"].(map[string]any)
			fieldID, _ := bindsTo["@id"].(string)
			if info, ok := fields[fieldID]; !ok || !info.hasValue {
				messages = append(messages, fmt.Sprintf("prose placeholder binds to unfilled field %q", fieldID))
			}
		}
	}
	return messages
}
