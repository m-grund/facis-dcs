package validation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const ontologyDomainFieldsPath = "docs/semantic-ontology/ontology/facis-dcs-ontology.ttl"

var (
	ontologyQuotedValue      = regexp.MustCompile(`"([^"]*)"`)
	ontologyNumberValue      = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?`)
	ontologyDomainFieldIndex = mustLoadOntologyDomainFields()
)

const (
	ontologyDCSBase  = "https://w3id.org/facis/dcs/ontology/v1#"
	ontologyDCSTBase = "https://w3id.org/facis/dcs/taxonomy/v1#"
)

func mustLoadOntologyDomainFields() map[string]domainField {
	fields, err := loadOntologyDomainFields()
	if err != nil {
		panic(err)
	}
	return fields
}

func loadOntologyDomainFields() (map[string]domainField, error) {
	var failures []string
	for _, path := range ontologyPathCandidates() {
		content, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		fields, err := parseOntologyDomainFields(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse ontology domain fields from %s: %w", path, err)
		}
		return fields, nil
	}
	return nil, fmt.Errorf("load FACIS DCS ontology domain fields: %s", strings.Join(failures, "; "))
}

func ontologyPathCandidates() []string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("FACIS_DCS_ONTOLOGY_PATH")); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, ontologyDomainFieldsPath, filepath.Join("..", ontologyDomainFieldsPath))
	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
		candidates = append(candidates, filepath.Join(repoRoot, ontologyDomainFieldsPath))
	}
	return candidates
}

func parseOntologyDomainFields(content string) (map[string]domainField, error) {
	statements := ontologyStatements(content)
	constraints := map[string]*valueConstraint{}
	for _, statement := range statements {
		if !strings.Contains(statement, " a dcs:ValueConstraint") {
			continue
		}
		subject := ontologySubject(statement)
		constraints[subject] = parseOntologyValueConstraint(statement)
	}

	fields := map[string]domainField{}
	for _, statement := range statements {
		if !strings.Contains(statement, " a dcs:DomainField") {
			continue
		}
		semanticPath := ontologyString(statement, "dcs:semanticPath")
		schemaRef := ontologyString(statement, "dcs:schemaRef")
		parameterType := ontologyString(statement, "dcs:parameterType")
		if semanticPath == "" || schemaRef == "" || parameterType == "" {
			return nil, fmt.Errorf("domain field %s requires semanticPath, schemaRef, and parameterType", ontologySubject(statement))
		}
		subject := ontologySubject(statement)
		field := domainField{
			SchemaRef:      schemaRef,
			Type:           parameterType,
			DomainPath:     semanticPath,
			OntologyTerm:   expandOntologyResource(subject),
			StatementField: ontologyString(statement, "dcs:statementField"),
		}
		if constraintRef := ontologyResource(statement, "dcs:hasValueConstraint"); constraintRef != "" {
			constraint, ok := constraints[constraintRef]
			if !ok {
				return nil, fmt.Errorf("domain field %s references unknown value constraint %s", ontologySubject(statement), constraintRef)
			}
			copy := *constraint
			copy.AllowedValues = append([]string(nil), constraint.AllowedValues...)
			field.Constraint = &copy
		}
		fields[semanticPath] = field
		if alias := legacySemanticPathAlias(semanticPath); alias != "" && alias != semanticPath {
			fields[alias] = field
		}
		fields[subject] = field
		if field.OntologyTerm != "" {
			fields[field.OntologyTerm] = field
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("ontology does not define dcs:DomainField entries")
	}
	return fields, nil
}

func legacySemanticPathAlias(value string) string {
	return strings.ReplaceAll(value, ".", "_")
}

func expandOntologyResource(value string) string {
	switch {
	case strings.HasPrefix(value, "dcs:"):
		return ontologyDCSBase + strings.TrimPrefix(value, "dcs:")
	case strings.HasPrefix(value, "dcst:"):
		return ontologyDCSTBase + strings.TrimPrefix(value, "dcst:")
	case strings.HasPrefix(value, "http://"), strings.HasPrefix(value, "https://"):
		return value
	default:
		return value
	}
}

func canonicalDomainFieldTerm(value string) string {
	field, ok := ontologyDomainFieldIndex[value]
	if ok && field.OntologyTerm != "" {
		return field.OntologyTerm
	}
	return value
}

func equivalentSemanticPath(left string, right string) bool {
	return canonicalDomainFieldTerm(left) == canonicalDomainFieldTerm(right)
}

func ontologyStatements(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var statements []string
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@prefix") {
			continue
		}
		lines = append(lines, line)
		if strings.HasSuffix(line, " .") || line == "." {
			statements = append(statements, strings.Join(lines, "\n"))
			lines = nil
		}
	}
	return statements
}

func ontologySubject(statement string) string {
	fields := strings.Fields(statement)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func parseOntologyValueConstraint(statement string) *valueConstraint {
	constraint := &valueConstraint{
		Format:           ontologyString(statement, "dcs:format"),
		Pattern:          ontologyString(statement, "dcs:pattern"),
		AllowedValues:    ontologyStrings(statement, "dcs:allowedValue"),
		AllowedValuesRef: ontologyString(statement, "dcs:allowedValuesRef"),
		Description:      ontologyString(statement, "rdfs:label"),
	}
	constraint.Min = ontologyNumber(statement, "dcs:minInclusive")
	constraint.Max = ontologyNumber(statement, "dcs:maxInclusive")
	return constraint
}

func ontologyString(statement string, predicate string) string {
	values := ontologyStrings(statement, predicate)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func ontologyStrings(statement string, predicate string) []string {
	values := []string{}
	for _, line := range strings.Split(statement, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), predicate+" ") {
			continue
		}
		matches := ontologyQuotedValue.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			values = append(values, match[1])
		}
	}
	return values
}

func ontologyNumber(statement string, predicate string) *float64 {
	for _, line := range strings.Split(statement, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), predicate+" ") {
			continue
		}
		match := ontologyNumberValue.FindString(line)
		if match == "" {
			return nil
		}
		value, err := strconv.ParseFloat(match, 64)
		if err == nil {
			return &value
		}
	}
	return nil
}

func ontologyResource(statement string, predicate string) string {
	for _, line := range strings.Split(statement, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == predicate {
			return strings.TrimSuffix(fields[1], ";")
		}
	}
	return ""
}
