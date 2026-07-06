import ontologyText from '../../../../../../docs/ontology/facis-sla-ontology.ttl?raw'
import type {
  DomainFieldDefinition,
  SemanticEntityRole,
  SemanticEntityType,
  SemanticParameterType,
  SemanticValueConstraint,
  SemanticValueOption,
} from '@/modules/template-repository/models/contract-template'

interface OntologyStatement {
  subject: string
  text: string
}

export interface OntologySelectOption<TValue extends string = string> {
  value: TValue
  label: string
}

export interface OntologyEntityTypeOption extends OntologySelectOption<SemanticEntityType> {
  roleRequired: boolean
}

const quotedValue = /"([^"]*)"/g
const numericValue = /[-+]?[0-9]+(?:\.[0-9]+)?/

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = parseOntologyDomainFields(ontologyText)
export const ONTOLOGY_ENTITY_TYPES: readonly OntologyEntityTypeOption[] = parseOntologyEntityTypes(ontologyText)
export const ONTOLOGY_ENTITY_ROLES: readonly OntologySelectOption<SemanticEntityRole>[] =
  parseOntologyEntityRoles(ontologyText)

function parseOntologyDomainFields(source: string): DomainFieldDefinition[] {
  const statements = parseStatements(source)
  const constraints = parseValueConstraints(statements)
  const classLabels = parseClassLabels(statements)

  return statements
    .filter((statement) => statement.text.includes(' a dcs:DomainField'))
    .map((statement) => {
      const semanticPath = firstLiteral(statement.text, 'dcs:semanticPath')
      const schemaRef = firstLiteral(statement.text, 'dcs:schemaRef')
      const type = firstLiteral(statement.text, 'dcs:parameterType') as SemanticParameterType
      const label = firstLiteral(statement.text, 'rdfs:label')
      if (!semanticPath || !schemaRef || !type || !label) {
        throw new Error(`Ontology domain field ${statement.subject} is incomplete.`)
      }
      const valueConstraintRef = firstResource(statement.text, 'dcs:hasValueConstraint')
      const statementType = firstResource(statement.text, 'dcs:statementType') || undefined
      return {
        ontologyId: expandResource(statement.subject),
        semanticPath,
        schemaRef,
        type,
        label,
        statementType,
        statementTypeLabel: statementType ? classLabels.get(localName(statementType)) : undefined,
        valueConstraint: valueConstraintRef ? cloneConstraint(constraints.get(valueConstraintRef)) : undefined,
      }
    })
    .sort((left, right) => left.semanticPath.localeCompare(right.semanticPath))
}

function parseOntologyEntityTypes(source: string): OntologyEntityTypeOption[] {
  const statements = parseStatements(source)
  const statementTypeNames = new Set(
    statements
      .filter((statement) => statement.text.includes(' a dcs:DomainField'))
      .map((statement) => localName(firstResource(statement.text, 'dcs:statementType')))
      .filter(Boolean),
  )
  const documentEntityTypeNames = new Set(
    statements
      .filter((statement) => statement.text.includes('dcs:documentProperty'))
      .map((statement) => localName(firstResource(statement.text, 'rdfs:range')))
      .filter((name) => name && statementTypeNames.has(name)),
  )
  const entityTypeNames = new Set(
    documentEntityTypeNames.size
      ? documentEntityTypeNames
      : [...statementTypeNames].filter((name) => !isPrimitiveCodeType(name)),
  )
  const roleRequired = parseOntologyEntityRoles(source).length > 0

  return statements
    .filter((statement) => statement.text.includes(' a rdfs:Class'))
    .filter((statement) => entityTypeNames.has(localName(statement.subject)))
    .map((statement) => {
      const value = localName(statement.subject)
      const label = firstLiteral(statement.text, 'rdfs:label') || value
      if (!value) throw new Error(`Ontology entity type ${statement.subject} is incomplete.`)
      return { value, label, roleRequired }
    })
    .sort((left, right) => left.label.localeCompare(right.label))
}

function parseOntologyEntityRoles(source: string): OntologySelectOption<SemanticEntityRole>[] {
  const statements = parseStatements(source)
  const constraints = parseValueConstraints(statements)
  const roleConstraintRefs = statements
    .filter((statement) => statement.text.includes(' a rdf:Property'))
    .filter((statement) => localName(firstResource(statement.text, 'rdfs:range')).endsWith('RoleCode'))
    .map((statement) => firstResource(statement.text, 'dcs:hasValueConstraint'))
    .filter(Boolean)
  const allowedValues = roleConstraintRefs.flatMap((ref) => constraints.get(ref)?.allowedValues ?? [])

  return allowedValues
    .map((value) => ({ value, label: formatOntologyLabel(value) }))
    .sort((left, right) => left.label.localeCompare(right.label))
}

function isPrimitiveCodeType(name: string): boolean {
  return name.endsWith('Code')
}

function parseClassLabels(statements: readonly OntologyStatement[]): ReadonlyMap<string, string> {
  const labels = new Map<string, string>()
  for (const statement of statements) {
    if (!statement.text.includes(' a rdfs:Class') && !statement.text.includes(' a owl:Class')) continue
    const name = localName(statement.subject)
    const label = firstLiteral(statement.text, 'rdfs:label') || name
    if (name) labels.set(name, label)
  }
  return labels
}

function parseValueConstraints(statements: readonly OntologyStatement[]): ReadonlyMap<string, SemanticValueConstraint> {
  const constraints = new Map<string, SemanticValueConstraint>()
  const valueOptions = parseValueOptions(statements)

  for (const statement of statements) {
    if (!statement.text.includes(' a dcs:ValueConstraint')) continue
    constraints.set(statement.subject, parseValueConstraint(statement.text, valueOptions))
  }
  return constraints
}

function parseStatements(source: string): OntologyStatement[] {
  const statements: OntologyStatement[] = []
  let lines: string[] = []

  for (const rawLine of source.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#') || line.startsWith('@prefix')) continue
    lines.push(line)
    if (!line.endsWith(' .') && line !== '.') continue
    const text = lines.join('\n')
    const subject = text.split(/\s+/, 1)[0] ?? ''
    statements.push({ subject, text })
    lines = []
  }
  return statements
}

function parseValueConstraint(
  statement: string,
  catalogOptions: ReadonlyMap<string, SemanticValueOption>,
): SemanticValueConstraint {
  const allowedValues = literals(statement, 'dcs:allowedValue')
  return {
    format: firstLiteral(statement, 'dcs:format') as SemanticValueConstraint['format'],
    pattern: firstLiteral(statement, 'dcs:pattern') || undefined,
    allowedValues,
    valueOptions: allowedValues
      .map((value) => catalogOptions.get(value))
      .filter((option): option is SemanticValueOption => !!option),
    allowedValuesRef: firstLiteral(statement, 'dcs:allowedValuesRef') || undefined,
    min: firstNumber(statement, 'dcs:minInclusive'),
    max: firstNumber(statement, 'dcs:maxInclusive'),
    description: firstLiteral(statement, 'rdfs:label') || undefined,
  }
}

function parseValueOptions(statements: readonly OntologyStatement[]): ReadonlyMap<string, SemanticValueOption> {
  const options = new Map<string, SemanticValueOption>()
  for (const statement of statements) {
    const value = firstLiteral(statement.text, 'skos:notation')
    if (!value) continue
    options.set(value, {
      value,
      label: firstLiteral(statement.text, 'skos:prefLabel') || undefined,
      symbol: firstLiteral(statement.text, 'dcs:valueSymbol') || undefined,
    })
  }
  return options
}

function firstLiteral(statement: string, predicate: string): string {
  return literals(statement, predicate)[0] ?? ''
}

function literals(statement: string, predicate: string): string[] {
  const line = predicateLine(statement, predicate)
  if (!line) return []
  return [...line.matchAll(quotedValue)].map((match) => match[1] ?? '')
}

function firstNumber(statement: string, predicate: string): number | undefined {
  const match = numericValue.exec(predicateLine(statement, predicate))
  return match ? Number(match[0]) : undefined
}

function firstResource(statement: string, predicate: string): string {
  const line = predicateLine(statement, predicate)
  if (!line) return ''
  return line.split(/\s+/)[1]?.replace(/[;.]+$/, '') ?? ''
}

function localName(resource: string): string {
  return resource.replace(/^.*[:#/]/, '')
}

function expandResource(resource: string): string {
  if (resource.startsWith('dcst:')) return `https://w3id.org/facis/dcs/taxonomy/v1#${resource.slice('dcst:'.length)}`
  if (resource.startsWith('dcs:')) return `https://w3id.org/facis/dcs/ontology/v1#${resource.slice(4)}`
  return resource
}

function formatOntologyLabel(value: string): string {
  const spaced = value.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[-_]+/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function predicateLine(statement: string, predicate: string): string {
  return (
    statement
      .split('\n')
      .map((line) => line.trim())
      .find((line) => line.startsWith(`${predicate} `)) ?? ''
  )
}

function cloneConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
  }
}
