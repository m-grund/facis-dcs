import type {
  DomainFieldDefinition,
  SemanticConditionParameter,
  SemanticEntityRole,
  SemanticValueConstraint,
} from '@/modules/template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS, ONTOLOGY_ENTITY_ROLES, ONTOLOGY_ENTITY_TYPES } from './ontology-domain-fields'

export interface OntologyDomainType {
  id: string
  label: string
  entityType: string
  roleRequired: boolean
  fields: readonly DomainFieldDefinition[]
}

export const ontologyRoleOptions = ONTOLOGY_ENTITY_ROLES

export const ONTOLOGY_DOMAIN_TYPES: readonly OntologyDomainType[] = buildOntologyDomainTypes()
export const ONTOLOGY_DOMAIN_TYPE_FIELD_PATHS: ReadonlySet<string> = new Set(
  ONTOLOGY_DOMAIN_TYPES.flatMap((domainType) => domainType.fields.map((field) => field.semanticPath)),
)

export function buildOntologyDomainTypeParameters(domainType: OntologyDomainType): SemanticConditionParameter[] {
  return domainType.fields.map((field) => ({
    parameterName: field.semanticPath,
    type: field.type,
    schemaRef: field.schemaRef,
    semanticPath: field.semanticPath,
    valueConstraint: cloneValueConstraint(field.valueConstraint),
    uiMetadata: { label: field.label },
    isRequired: true,
    operators: [],
    value: undefined,
  }))
}

export function buildOntologyDomainTypeClauseText(
  conditionId: string,
  domainType: OntologyDomainType,
  role?: SemanticEntityRole,
): string {
  const roleLabel = role ? roleLabelFor(role) : ''
  const title = roleLabel ? `${roleLabel} ${domainType.label}` : domainType.label
  const fieldLines = domainType.fields.map((field) => buildDomainTypeClauseFieldLine(conditionId, field))
  return [title, '', ...fieldLines].join('\n')
}

export function roleLabelFor(role: SemanticEntityRole): string {
  return ONTOLOGY_ENTITY_ROLES.find((option) => option.value === role)?.label ?? role
}

function buildDomainTypeClauseFieldLine(conditionId: string, field: DomainFieldDefinition): string {
  const label = field.label || field.semanticPath
  return `${label}: {{${conditionId}.${field.semanticPath}}}`
}

function buildOntologyDomainTypes(): OntologyDomainType[] {
  const domainTypes: OntologyDomainType[] = []
  for (const entityType of ONTOLOGY_ENTITY_TYPES) {
    const fields = fieldsForEntityType(entityType.value)
    if (!fields.length) continue
    domainTypes.push({
      id: entityType.value,
      label: entityType.label,
      entityType: entityType.value,
      roleRequired: fields.some((field) => firstSemanticPathSegment(field.semanticPath) === 'company'),
      fields,
    })
  }
  return domainTypes
}

function fieldsForEntityType(entityType: string): DomainFieldDefinition[] {
  const directlyTyped = ONTOLOGY_DOMAIN_FIELDS.filter(
    (field) => localOntologyName(field.statementType ?? '') === entityType,
  )
  const fieldPrefixes = new Set(directlyTyped.map((field) => firstSemanticPathSegment(field.semanticPath)))
  if (!fieldPrefixes.size) return []
  return ONTOLOGY_DOMAIN_FIELDS.filter((field) => fieldPrefixes.has(firstSemanticPathSegment(field.semanticPath))).sort(
    (left, right) => left.label.localeCompare(right.label),
  )
}

function firstSemanticPathSegment(path: string): string {
  return path.split('.', 1)[0] ?? path
}

function localOntologyName(resource: string): string {
  return resource.replace(/^.*[:#/]/, '')
}

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
  }
}
