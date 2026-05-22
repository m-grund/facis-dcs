import ontologyText from '../../../../../../docs/semantic-ontology/ontology/facis-dcs-ontology.ttl?raw'
import type {
  DomainFieldDefinition,
  SemanticParameterType,
  SemanticValueConstraint,
} from '@/modules/template-repository/models/contract-template'

interface OntologyStatement {
  subject: string
  text: string
}

const quotedValue = /"([^"]*)"/g
const numericValue = /[-+]?[0-9]+(?:\.[0-9]+)?/

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] =
  parseOntologyDomainFields(ontologyText)

export function parseOntologyDomainFields(source: string): DomainFieldDefinition[] {
  const statements = parseStatements(source)
  const constraints = new Map<string, SemanticValueConstraint>()

  for (const statement of statements) {
    if (!statement.text.includes(' a dcs:ValueConstraint')) continue
    constraints.set(statement.subject, parseValueConstraint(statement.text))
  }

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
      return {
        semanticPath,
        schemaRef,
        type,
        label,
        group: inferDomainFieldGroup(semanticPath),
        valueConstraint: valueConstraintRef ? cloneConstraint(constraints.get(valueConstraintRef)) : undefined,
      }
    })
    .sort((left, right) => left.semanticPath.localeCompare(right.semanticPath))
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

function parseValueConstraint(statement: string): SemanticValueConstraint {
  return {
    format: firstLiteral(statement, 'dcs:format') as SemanticValueConstraint['format'],
    pattern: firstLiteral(statement, 'dcs:pattern') || undefined,
    allowedValues: literals(statement, 'dcs:allowedValue'),
    allowedValuesRef: firstLiteral(statement, 'dcs:allowedValuesRef') || undefined,
    min: firstNumber(statement, 'dcs:minInclusive'),
    max: firstNumber(statement, 'dcs:maxInclusive'),
    description: firstLiteral(statement, 'rdfs:label') || undefined,
  }
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
  const match = predicateLine(statement, predicate)?.match(numericValue)
  return match ? Number(match[0]) : undefined
}

function firstResource(statement: string, predicate: string): string {
  const line = predicateLine(statement, predicate)
  if (!line) return ''
  return line.split(/\s+/)[1]?.replace(/[;.]+$/, '') ?? ''
}

function predicateLine(statement: string, predicate: string): string {
  return statement
    .split('\n')
    .map((line) => line.trim())
    .find((line) => line.startsWith(`${predicate} `)) ?? ''
}

function cloneConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
  }
}

function inferDomainFieldGroup(semanticPath: string): string {
  if (semanticPath.startsWith('company.location.')) return 'Address'
  if (semanticPath.startsWith('company.')) return 'Parties'
  if (semanticPath.startsWith('service.sla.')) return 'SLA'
  if (semanticPath.startsWith('service.')) return 'Service'
  if (semanticPath.startsWith('signature.')) return 'Signature'
  if (
    semanticPath.startsWith('contract.payment.') ||
    semanticPath.startsWith('contract.renewal.') ||
    semanticPath.startsWith('contract.termination.')
  ) {
    return 'Commercial'
  }
  if (
    semanticPath.startsWith('contract.liability.') ||
    semanticPath.startsWith('contract.insurance.') ||
    semanticPath.startsWith('contract.forceMajeure.')
  ) {
    return 'Risk'
  }
  if (
    semanticPath.startsWith('contract.dataProtection.') ||
    semanticPath.startsWith('contract.auditRights.')
  ) {
    return 'Compliance'
  }
  if (
    semanticPath.startsWith('contract.disputeResolution.') ||
    semanticPath.startsWith('contract.confidentiality.') ||
    semanticPath.startsWith('contract.ipRights.') ||
    semanticPath === 'contract.governingLaw' ||
    semanticPath === 'contract.jurisdiction'
  ) {
    return 'Legal'
  }
  if (semanticPath.startsWith('contract.validity.') || semanticPath === 'contract.effectiveDate') return 'Dates'
  return 'Contract basics'
}
