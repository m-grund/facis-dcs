package validation

import (
	"fmt"
	"strconv"
	"strings"
)

func LoadValidationProfileSHACL(raw []byte) (ValidationProfile, error) {
	statements := ontologyStatements(string(raw))
	profile := ValidationProfile{}
	rules := []ValidationRule{}
	for _, statement := range statements {
		switch {
		case strings.Contains(statement, " a facisv:ValidationProfile"):
			profile.ID = ontologyString(statement, "facisv:profileId")
			profile.Version = ontologyString(statement, "facisv:version")
			profile.Description = ontologyString(statement, "rdfs:comment")
		case strings.Contains(statement, " a sh:NodeShape") && ontologyString(statement, "facisv:ruleId") != "":
			rule, err := shaclRule(statement)
			if err != nil {
				return profile, err
			}
			rules = append(rules, rule)
		}
	}
	profile.Rules = rules
	return profile, ValidateValidationProfile(profile)
}

func shaclRule(statement string) (ValidationRule, error) {
	rule := ValidationRule{
		ID:             ontologyString(statement, "facisv:ruleId"),
		Type:           ontologyString(statement, "facisv:ruleType"),
		Severity:       shaclSeverity(statement),
		Message:        ontologyString(statement, "sh:message"),
		Target:         ontologyString(statement, "facisv:target"),
		Where:          shaclWhereClause(ontologyStrings(statement, "facisv:where")),
		Operator:       ontologyString(statement, "facisv:operator"),
		Value:          shaclValue(statement),
		RequiredFields: ontologyStrings(statement, "facisv:requiredField"),
	}
	referenceFields, err := shaclReferenceFields(ontologyStrings(statement, "facisv:referenceField"))
	if err != nil {
		return rule, fmt.Errorf("validation rule %q: %w", rule.ID, err)
	}
	rule.ReferenceFields = referenceFields
	return rule, nil
}

func shaclSeverity(statement string) string {
	severity := ontologyResource(statement, "sh:severity")
	switch severity {
	case "sh:Warning":
		return "warning"
	case "sh:Info":
		return "info"
	default:
		return "error"
	}
}

func shaclWhereClause(expressions []string) map[string]any {
	where := map[string]any{}
	for _, expression := range expressions {
		key, value, ok := strings.Cut(expression, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		where[key] = expandSHACLValue(value)
	}
	return where
}

func shaclReferenceFields(expressions []string) ([]StatementReference, error) {
	references := []StatementReference{}
	for _, expression := range expressions {
		field, whereExpression, _ := strings.Cut(expression, "|")
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, fmt.Errorf("referenceField requires a field")
		}
		where := map[string]any{}
		for _, part := range strings.Split(whereExpression, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				return nil, fmt.Errorf("invalid reference where expression %q", part)
			}
			where[strings.TrimSpace(key)] = expandSHACLValue(strings.TrimSpace(value))
		}
		references = append(references, StatementReference{Field: field, Where: where})
	}
	return references, nil
}

func shaclValue(statement string) any {
	line := shaclPredicateLine(statement, "facisv:value")
	if line == "" {
		return nil
	}
	quoted := ontologyQuotedValue.FindStringSubmatch(line)
	if len(quoted) == 2 {
		return expandSHACLValue(quoted[1])
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil
	}
	raw := strings.TrimSuffix(strings.TrimSuffix(fields[1], ";"), ".")
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		if float64(int(number)) == number {
			return int(number)
		}
		return number
	}
	return expandSHACLValue(raw)
}

func shaclPredicateLine(statement string, predicate string) string {
	for _, line := range strings.Split(statement, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, predicate+" ") {
			return line
		}
	}
	return ""
}

func expandSHACLValue(value string) string {
	switch {
	case strings.HasPrefix(value, "dcs:"):
		return ontologyDCSBase + strings.TrimPrefix(value, "dcs:")
	case strings.HasPrefix(value, "dcst:"):
		return ontologyDCSTBase + strings.TrimPrefix(value, "dcst:")
	default:
		return value
	}
}
