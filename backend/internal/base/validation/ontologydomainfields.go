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

var (
	ontologyQuotedValue      = regexp.MustCompile(`"([^"]*)"`)
	ontologyNumberValue      = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?`)
	ontologyDomainFieldsPath = "docs/ontology/facis-sla-ontology.ttl"
	ontologyPrefixIndex      = mustLoadOntologyPrefixes()
	ontologyDomainFieldIndex = mustLoadOntologyDomainFields()
	ontologyClassIndex       = mustLoadOntologyClasses()
	ontologyRuntime          = buildOntologyRuntime()
)

type ontologyRuntimeMetadata struct {
	StatementSetType         string
	RoleEntityType           string
	EntityRoleField          string
	EntityRoleStatementField string
	EntityRoleValuePrefix    string
	EntityRoleAllowedValues  []string
	RoleEntityDocumentField  string
	StatementSetProperty     string
}

func mustLoadOntologyPrefixes() map[string]string {
	prefixes, err := loadOntologyPrefixes()
	if err != nil {
		panic(err)
	}
	return prefixes
}

func loadOntologyPrefixes() (map[string]string, error) {
	var failures []string
	for _, path := range ontologyPathCandidates() {
		content, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		return parseOntologyPrefixes(string(content)), nil
	}
	return nil, fmt.Errorf("load FACIS DCS ontology prefixes: %s", strings.Join(failures, "; "))
}

func parseOntologyPrefixes(content string) map[string]string {
	prefixes := map[string]string{}
	pattern := regexp.MustCompile(`@prefix\s+([^:\s]+):\s+<([^>]+)>`)
	for _, match := range pattern.FindAllStringSubmatch(content, -1) {
		prefixes[match[1]] = match[2]
	}
	return prefixes
}

func mustLoadOntologyDomainFields() map[string]domainField {
	fields, err := loadOntologyDomainFields()
	if err != nil {
		panic(err)
	}
	return fields
}

func mustLoadOntologyClasses() map[string]string {
	classes, err := loadOntologyClasses()
	if err != nil {
		panic(err)
	}
	return classes
}

func loadOntologyClasses() (map[string]string, error) {
	var failures []string
	for _, path := range ontologyPathCandidates() {
		content, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		return parseOntologyClasses(string(content)), nil
	}
	return nil, fmt.Errorf("load FACIS DCS ontology classes: %s", strings.Join(failures, "; "))
}

func parseOntologyClasses(content string) map[string]string {
	classes := map[string]string{}
	for _, statement := range ontologyStatements(content) {
		if !strings.Contains(statement, " a rdfs:Class") && !strings.Contains(statement, " a owl:Class") {
			continue
		}
		subject := ontologySubject(statement)
		if subject == "" {
			continue
		}
		classes[expandOntologyResource(subject)] = statement
	}
	return classes
}

func buildOntologyRuntime() ontologyRuntimeMetadata {
	roleEntityType := expandOntologyResource("dcs:CompanyParty")
	statementSetType, statementProperty := statementSetRuntime()
	roleEntityDocumentField := documentPropertyForRange(roleEntityType)
	return ontologyRuntimeMetadata{
		StatementSetType:         statementSetType,
		RoleEntityType:           roleEntityType,
		EntityRoleField:          expandOntologyResource("dcs:role"),
		EntityRoleStatementField: "role",
		EntityRoleValuePrefix:    expandOntologyResource("dcst:role-"),
		EntityRoleAllowedValues:  entityRoleAllowedValues(),
		RoleEntityDocumentField:  roleEntityDocumentField,
		StatementSetProperty:     statementProperty,
	}
}

func entityRoleAllowedValues() []string {
	for _, statement := range ontologyStatementsFromConfiguredFile() {
		if ontologySubject(statement) != "dcst:constraint-contract-party-role" {
			continue
		}
		constraint := parseOntologyValueConstraint(statement)
		return append([]string(nil), constraint.AllowedValues...)
	}
	return nil
}

func domainFieldByStatementField(statementField string) domainField {
	for _, field := range ontologyDomainFieldIndex {
		if field.StatementField == statementField {
			return field
		}
	}
	return domainField{}
}

func statementTypeByStatementField(statementField string) string {
	return domainFieldByStatementField(statementField).StatementType
}

func statementSetRuntime() (string, string) {
	for class, statement := range ontologyClassIndex {
		if property := ontologyString(statement, "dcs:documentProperty"); property != "" {
			return class, property
		}
	}
	return "", ""
}

func documentPropertyForRange(rangeValue string) string {
	for _, statement := range ontologyStatementsFromConfiguredFile() {
		if expandOntologyResource(ontologyResource(statement, "rdfs:range")) != rangeValue {
			continue
		}
		if property := ontologyString(statement, "dcs:documentProperty"); property != "" {
			return property
		}
	}
	return ""
}

func ontologyStatementsFromConfiguredFile() []string {
	for _, path := range ontologyPathCandidates() {
		content, err := os.ReadFile(path)
		if err == nil {
			return ontologyStatements(string(content))
		}
	}
	return nil
}

func statementLeaf(statementField string) string {
	_, leaf, ok := splitStatementField(statementField)
	if !ok {
		return statementField
	}
	return leaf
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
			StatementType:  expandOntologyResource(ontologyResource(statement, "dcs:statementType")),
			StatementID:    ontologyString(statement, "dcs:statementId"),
			ValuePrefix:    expandOntologyResource(ontologyResource(statement, "dcs:statementValuePrefix")),
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

func expandOntologyResource(value string) string {
	prefix, suffix, ok := strings.Cut(value, ":")
	if ok && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		if base := ontologyPrefixIndex[prefix]; base != "" {
			return base + suffix
		}
	}
	switch {
	case strings.HasPrefix(value, "http://"), strings.HasPrefix(value, "https://"):
		return value
	default:
		return value
	}
}

func statementSetOntologyType() string {
	return ontologyRuntime.StatementSetType
}

func statementSetDocumentProperty() string {
	return ontologyRuntime.StatementSetProperty
}

func ontologyIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.Contains(value, ":") {
		return expandOntologyResource(value)
	}
	if class := ontologyClassByLocalName(value); class != "" {
		return class
	}
	return value
}

func ontologyClassByLocalName(value string) string {
	for class := range ontologyClassIndex {
		if ontologyLocalName(class) == value {
			return class
		}
	}
	return ""
}

func ontologyLocalName(value string) string {
	if hash := strings.LastIndex(value, "#"); hash >= 0 && hash < len(value)-1 {
		return value[hash+1:]
	}
	if slash := strings.LastIndex(value, "/"); slash >= 0 && slash < len(value)-1 {
		return value[slash+1:]
	}
	return value
}

func canonicalStatementEntityType(value string) string {
	identifier := ontologyIdentifier(value)
	if _, ok := ontologyClassIndex[identifier]; ok {
		return identifier
	}
	return ""
}

func statementEntityTypeSupportsRole(value string) bool {
	return value == ontologyRuntime.RoleEntityType
}

func canonicalEntityRole(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.Contains(value, ":") {
		return expandOntologyResource(value)
	}
	if ontologyRuntime.EntityRoleValuePrefix != "" {
		return ontologyRuntime.EntityRoleValuePrefix + slugify(value)
	}
	return value
}

func entityRoleFromEntityType(value string) string {
	return ""
}

func applyStatementEntityRole(statement map[string]any, role string) {
	if role != "" {
		statement[ontologyRuntime.EntityRoleStatementField] = role
	}
}

func normalizeStatementValue(record semanticValueRecord) any {
	if record.OntologyTerm == ontologyRuntime.EntityRoleField {
		if role, ok := record.Value.(string); ok {
			return canonicalEntityRole(role)
		}
	}
	return record.Value
}

func validateOntologyRoleEntity(entity map[string]any) error {
	entityType, _ := entity["@type"].(string)
	if ontologyIdentifier(entityType) != ontologyRuntime.RoleEntityType {
		return fmt.Errorf("@type must be %s", compactOntologyResource(ontologyRuntime.RoleEntityType))
	}
	if len(ontologyRuntime.EntityRoleAllowedValues) == 0 {
		return fmt.Errorf("role ontology requires allowed values")
	}
	role, _ := entity[ontologyRuntime.EntityRoleStatementField].(string)
	if !containsString(ontologyRuntime.EntityRoleAllowedValues, role) && !containsString(ontologyRuntime.EntityRoleAllowedValues, compactEntityRole(role)) {
		return fmt.Errorf("role must be one of %s", strings.Join(ontologyRuntime.EntityRoleAllowedValues, ", "))
	}
	return nil
}

func compactEntityRole(value string) string {
	prefix := ontologyRuntime.EntityRoleValuePrefix
	if prefix != "" && strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix)
	}
	return value
}

func compactOntologyResource(value string) string {
	for prefix, base := range ontologyPrefixIndex {
		if strings.HasPrefix(value, base) {
			return prefix + ":" + strings.TrimPrefix(value, base)
		}
	}
	return value
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

func ontologyStatementHasType(statement string, class string) bool {
	expandedClass := expandOntologyResource(class)
	for _, line := range strings.Split(statement, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 || fields[1] != "a" {
			continue
		}
		for _, rawClass := range fields[2:] {
			candidate := strings.TrimSuffix(strings.TrimSuffix(rawClass, ";"), ",")
			if expandOntologyResource(candidate) == expandedClass {
				return true
			}
		}
	}
	return false
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

func ontologyBool(statement string, predicate string) bool {
	for _, line := range strings.Split(statement, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == predicate {
			value := strings.TrimSuffix(fields[1], ";")
			return value == "true" || value == "true^^xsd:boolean"
		}
	}
	return false
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
