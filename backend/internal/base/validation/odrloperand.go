package validation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TypeODRLOperand converts a policy value into a JSON-LD resource reference or
// typed literal using the ontology metadata for the referenced domain field.
func TypeODRLOperand(domainField string, value any) any {
	field, found := ontologyDomainFieldIndex[domainField]
	if !found {
		return typeFallbackODRLOperand(value)
	}
	if iri := taxonomyValueIRI(field.Constraint, value); iri != "" {
		return map[string]any{"@id": iri}
	}
	switch field.Type {
	case "decimal":
		return typedNumericODRLOperand("xsd:decimal", value)
	case "integer":
		return typedNumericODRLOperand("xsd:integer", value)
	case "boolean":
		return map[string]any{
			"@type":  "xsd:boolean",
			"@value": strings.ToLower(fmt.Sprint(value)),
		}
	case "date":
		return typedTemporalODRLOperand(fmt.Sprint(value))
	default:
		return typeFallbackODRLOperand(value)
	}
}

// SemanticDataType returns the ontology or XSD datatype associated with a
// domain field. Taxonomy-backed fields use the value type declared by their
// ontology constraint.
func SemanticDataType(domainField string) string {
	field, found := ontologyDomainFieldIndex[domainField]
	if !found {
		return ""
	}
	if field.Constraint != nil && field.Constraint.ValueType != "" {
		return compactSemanticDataType(field.Constraint.ValueType)
	}
	switch field.Type {
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

// SemanticObjectType returns the ontology class associated with a domain field.
func SemanticObjectType(domainField string) string {
	field, found := ontologyDomainFieldIndex[domainField]
	if !found {
		return ""
	}
	return compactSemanticDataType(field.StatementType)
}

func compactSemanticDataType(value string) string {
	const (
		dcsNamespace = "https://w3id.org/facis/dcs/ontology/v1#"
		xsdNamespace = "http://www.w3.org/2001/XMLSchema#"
	)
	switch {
	case strings.HasPrefix(value, dcsNamespace):
		return "dcs:" + strings.TrimPrefix(value, dcsNamespace)
	case strings.HasPrefix(value, xsdNamespace):
		return "xsd:" + strings.TrimPrefix(value, xsdNamespace)
	default:
		return value
	}
}

func taxonomyValueIRI(constraint *valueConstraint, value any) string {
	if constraint == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	for _, option := range valueOptionsForConstraint(constraint) {
		if option.Value == text {
			return option.IRI
		}
	}
	return ""
}

func typedNumericODRLOperand(datatype string, value any) map[string]any {
	text := fmt.Sprint(value)
	switch number := value.(type) {
	case float64:
		if datatype == "xsd:integer" {
			text = strconv.FormatInt(int64(number), 10)
		} else {
			text = strconv.FormatFloat(number, 'f', -1, 64)
		}
	case float32:
		text = strconv.FormatFloat(float64(number), 'f', -1, 32)
	case int:
		text = strconv.Itoa(number)
	case int64:
		text = strconv.FormatInt(number, 10)
	}
	return map[string]any{"@type": datatype, "@value": text}
}

func typeFallbackODRLOperand(value any) any {
	if resource, ok := value.(string); ok {
		if strings.HasPrefix(resource, "http://") || strings.HasPrefix(resource, "https://") ||
			strings.HasPrefix(resource, "dcs:") || strings.HasPrefix(resource, "dcst:") ||
			strings.HasPrefix(resource, "sla:") || strings.HasPrefix(resource, "slat:") {
			return map[string]any{"@id": expandOntologyResource(resource)}
		}
		if temporal := typedTemporalODRLOperand(resource); temporal != nil {
			return temporal
		}
		return map[string]any{"@type": "xsd:string", "@value": resource}
	}
	if boolean, ok := value.(bool); ok {
		return map[string]any{"@type": "xsd:boolean", "@value": strconv.FormatBool(boolean)}
	}
	return value
}

func typedTemporalODRLOperand(value string) map[string]any {
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return map[string]any{"@type": "xsd:date", "@value": value}
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return map[string]any{"@type": "xsd:dateTime", "@value": value}
	}
	return nil
}
