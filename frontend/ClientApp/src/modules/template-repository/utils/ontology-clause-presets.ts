import type {
  DomainFieldDefinition,
  SemanticConditionParameter,
  SemanticEntityRole,
  SemanticValueConstraint,
} from '@/modules/template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS, ONTOLOGY_ENTITY_ROLES, ONTOLOGY_ENTITY_TYPES } from './ontology-domain-fields'

export interface OntologyClausePreset {
  id: string
  label: string
  entityType: string
  roleRequired: boolean
  fields: readonly DomainFieldDefinition[]
}

export const ontologyRoleOptions = ONTOLOGY_ENTITY_ROLES

export const ONTOLOGY_CLAUSE_PRESETS: readonly OntologyClausePreset[] = buildOntologyClausePresets()
export const ONTOLOGY_TYPE_DOMAIN_FIELD_PATHS: ReadonlySet<string> = new Set(
  ONTOLOGY_CLAUSE_PRESETS.flatMap((preset) => preset.fields.map((field) => field.semanticPath)),
)

export function buildOntologyConditionParameters(
  preset: OntologyClausePreset,
): SemanticConditionParameter[] {
  return preset.fields.map((field) => ({
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

export function buildOntologyClauseText(
  conditionId: string,
  preset: OntologyClausePreset,
  role?: SemanticEntityRole,
): string {
  const roleLabel = role ? roleLabelFor(role) : ''
  const title = roleLabel ? `${roleLabel} ${preset.label}` : preset.label
  const fieldLines = preset.fields.map((field) => buildOntologyClauseFieldLine(conditionId, field))
  return [
    title,
    '',
    ...fieldLines,
  ].join('\n')
}

export function roleLabelFor(role: SemanticEntityRole): string {
  return ONTOLOGY_ENTITY_ROLES.find((option) => option.value === role)?.label ?? role
}

function buildOntologyClauseFieldLine(conditionId: string, field: DomainFieldDefinition): string {
  const label = field.label || field.semanticPath
  return `${label}: {{${conditionId}.${field.semanticPath}}}`
}

function buildOntologyClausePresets(): OntologyClausePreset[] {
  const presets: OntologyClausePreset[] = []
  for (const entityType of ONTOLOGY_ENTITY_TYPES) {
    const fields = fieldsForEntityType(entityType.value)
    if (!fields.length) continue
    presets.push({
        id: entityType.value,
        label: entityType.label,
        entityType: entityType.value,
        roleRequired: fields.some((field) => firstSemanticPathSegment(field.semanticPath) === 'company'),
        fields,
    })
  }
  return presets
}

function fieldsForEntityType(entityType: string): DomainFieldDefinition[] {
  const directlyTyped = ONTOLOGY_DOMAIN_FIELDS.filter(
    (field) => localOntologyName(field.statementType ?? '') === entityType,
  )
  const fieldPrefixes = new Set(directlyTyped.map((field) => firstSemanticPathSegment(field.semanticPath)))
  if (!fieldPrefixes.size) return []
  return ONTOLOGY_DOMAIN_FIELDS
    .filter((field) => fieldPrefixes.has(firstSemanticPathSegment(field.semanticPath)))
    .sort((left, right) => left.label.localeCompare(right.label))
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
