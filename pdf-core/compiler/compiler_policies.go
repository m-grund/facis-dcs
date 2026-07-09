package compiler

import (
	"encoding/json"
	"fmt"
	"strings"
)

// odrlRuleKind maps an odrl rule @type to the human-readable label used as the
// lead word of a rendered policy entry.
func odrlRuleKind(ruleType string) string {
	switch localTermName(ruleType) {
	case "Duty":
		return "Obligation"
	case "Permission":
		return "Permission"
	case "Prohibition":
		return "Prohibition"
	default:
		return "Rule"
	}
}

// odrlOperatorPhrase maps an odrl operator IRI to a readable comparison phrase.
// Unknown operators fall back to their bare term name.
func odrlOperatorPhrase(operator string) string {
	switch localTermName(operator) {
	case "eq":
		return "must equal"
	case "neq":
		return "must not equal"
	case "gt":
		return "must be greater than"
	case "gteq":
		return "must be at least"
	case "lt":
		return "must be less than"
	case "lteq":
		return "must be at most"
	case "isAnyOf":
		return "must be any of"
	case "isNoneOf":
		return "must be none of"
	case "isA":
		return "must be"
	case "isPartOf":
		return "must be part of"
	case "hasPart":
		return "must contain"
	default:
		return localTermName(operator)
	}
}

// localTermName reduces an IRI or prefixed term to its human-facing local name:
// the fragment after the last '#', '/', or ':' separator.
func localTermName(iri string) string {
	s := strings.TrimSpace(iri)
	if s == "" {
		return ""
	}
	for _, sep := range []string{"#", "/", ":"} {
		if idx := strings.LastIndex(s, sep); idx >= 0 && idx < len(s)-1 {
			s = s[idx+1:]
		}
	}
	return s
}

// odrlRightOperandValues resolves an odrl:rightOperand — a bare scalar, an
// array of scalars, or @value objects — into displayable strings.
func odrlRightOperandValues(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	switch v := decoded.(type) {
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s := scalarToString(item); s != "" {
				values = append(values, s)
			}
		}
		return values
	default:
		if s := scalarToString(v); s != "" {
			return []string{s}
		}
		return nil
	}
}

func scalarToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
	case map[string]any:
		if inner, ok := v["@value"]; ok {
			return scalarToString(inner)
		}
		if id, ok := v["@id"].(string); ok {
			return localTermName(id)
		}
	}
	return ""
}

// renderPolicyRuleText produces the multi-line prose for a single policy rule.
func renderPolicyRuleText(rule OdrlRule) string {
	var b strings.Builder
	b.WriteString(odrlRuleKind(rule.Type))
	if action := localTermName(rule.Action.ID); action != "" {
		b.WriteString(": ")
		b.WriteString(action)
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Assigner %s, Assignee %s, Target %s",
		localTermName(rule.Assigner.ID),
		localTermName(rule.Assignee.ID),
		localTermName(rule.Target.ID)))
	if rule.Constraint != nil {
		left := localTermName(rule.Constraint.LeftOperand.ID)
		phrase := odrlOperatorPhrase(rule.Constraint.Operator.ID)
		values := odrlRightOperandValues(rule.Constraint.RightOperand)
		b.WriteString("\n")
		b.WriteString("Condition: ")
		b.WriteString(strings.TrimSpace(strings.Join([]string{left, phrase, strings.Join(values, ", ")}, " ")))
	}
	return b.String()
}

// buildPolicySection turns an odrl:Set into a rendered document section. Rules
// are emitted grouped as duties, then permissions, then prohibitions — a stable
// order independent of the input bucket order. Returns false when the set holds
// no rules, so an empty policy container adds nothing to the document.
func buildPolicySection(set *OdrlSet) (sectionData, bool) {
	if set == nil {
		return sectionData{}, false
	}
	section := sectionData{Heading: "Policies", Clauses: []clauseData{}}
	appendRules := func(rules odrlRuleList) {
		for _, rule := range rules {
			section.Clauses = append(section.Clauses, clauseData{
				Segments: []clauseSegment{{Type: "prose", Text: renderPolicyRuleText(rule)}},
			})
		}
	}
	appendRules(set.Duty)
	appendRules(set.Permission)
	appendRules(set.Prohibition)
	if len(section.Clauses) == 0 {
		return sectionData{}, false
	}
	return section, true
}
