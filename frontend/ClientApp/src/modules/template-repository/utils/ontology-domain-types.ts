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

export function roleLabelFor(role: SemanticEntityRole): string {
  return ONTOLOGY_ENTITY_ROLES.find((option) => option.value === role)?.label ?? role
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
      roleRequired: entityType.roleRequired,
      fields,
    })
  }
  return domainTypes
}

function fieldsForEntityType(entityType: string): DomainFieldDefinition[] {
  return ONTOLOGY_DOMAIN_FIELDS.filter((field) => localOntologyName(field.statementType ?? '') === entityType).sort(
    (left, right) => left.label.localeCompare(right.label),
  )
}

function localOntologyName(resource: string): string {
  return resource.replace(/^.*[:#/]/, '')
}

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
  }
}
