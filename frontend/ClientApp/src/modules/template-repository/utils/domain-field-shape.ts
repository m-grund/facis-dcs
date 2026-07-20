import type { DomainFieldDefinition } from '@template-repository/models/contract-template'

/**
 * Turns a domain field into a SHACL NodeShape (Turtle) that <shacl-form> can
 * render — the same engine that renders typed clauses, so field entry and
 * clause entry share one form. The field's own constraints become SHACL:
 * datatype from its type, sh:in from its value options, sh:pattern/min/max
 * from its value constraint. The form emits an instance carrying the value on
 * dcs:parameterValue, which maps straight onto the RequirementField the
 * document already uses (values live inline on the field — see the value-
 * inlining unification).
 */

const SH = 'http://www.w3.org/ns/shacl#'
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'
const XSD = 'http://www.w3.org/2001/XMLSchema#'
const RDFS = 'http://www.w3.org/2000/01/rdf-schema#'

const DATATYPE: Record<DomainFieldDefinition['type'], string> = {
  string: 'xsd:string',
  decimal: 'xsd:decimal',
  integer: 'xsd:integer',
  boolean: 'xsd:boolean',
  date: 'xsd:date',
  enum: 'xsd:string',
}

/** The class a field-value instance carries, so shacl-form targets the shape. */
export const DOMAIN_FIELD_CLASS = 'dcs:RequirementField'
export const DOMAIN_FIELD_SHAPE = 'dcs:DomainFieldValueShape'

/**
 * In a template the value is a declaration default (optional — the real value
 * is filled at contract time through the placeholder); at contract time it is
 * required.
 */
export function domainFieldShape(field: DomainFieldDefinition, valueRequired = false): string {
  const constraint = field.valueConstraint
  const property: string[] = [
    'sh:path dcs:parameterValue',
    `sh:name ${ttlString(field.label)}`,
    `sh:datatype ${DATATYPE[field.type] ?? 'xsd:string'}`,
    `sh:minCount ${valueRequired ? 1 : 0}`,
    'sh:maxCount 1',
  ]

  const options = constraint?.valueOptions?.map((option) => option.value) ?? constraint?.allowedValues
  if (options?.length) {
    property.push(`sh:in ( ${options.map(ttlString).join(' ')} )`)
  }
  if (constraint?.pattern) property.push(`sh:pattern ${ttlString(constraint.pattern)}`)
  if (typeof constraint?.min === 'number') property.push(`sh:minInclusive ${constraint.min}`)
  if (typeof constraint?.max === 'number') property.push(`sh:maxInclusive ${constraint.max}`)
  if (constraint?.description) property.push(`sh:description ${ttlString(constraint.description)}`)

  return `@prefix sh: <${SH}> .
@prefix dcs: <${DCS}> .
@prefix xsd: <${XSD}> .
@prefix rdfs: <${RDFS}> .

dcs:DomainFieldValueShape a sh:NodeShape ;
  sh:targetClass ${DOMAIN_FIELD_CLASS} ;
  rdfs:label ${ttlString(field.label)} ;
  sh:property [
    ${property.join(' ;\n    ')} ;
  ] .
`
}

/** Reads the value a shacl-form field instance carries (dcs:parameterValue),
 *  unwrapping the value-object and array forms JSON-LD serialization emits. */
export function fieldInstanceValue(instance: Record<string, unknown>): string | number | boolean | undefined {
  return scalarValue(instance['dcs:parameterValue'] ?? instance[`${DCS}parameterValue`])
}

function scalarValue(raw: unknown): string | number | boolean | undefined {
  if (Array.isArray(raw)) return raw.length ? scalarValue(raw[0]) : undefined
  if (raw != null && typeof raw === 'object') return scalarValue((raw as Record<string, unknown>)['@value'])
  if (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean') return raw
  return undefined
}

function ttlString(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}
