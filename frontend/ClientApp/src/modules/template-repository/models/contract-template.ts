import type { DcsOperator, ParameterType, UiMetadata } from '@/models/semantic/facis-dcs-semantic'

// ---- SemanticCondition ----

export interface SemanticCondition {
  conditionId: string
  conditionName: string
  schemaVersion: 'v1'
  entityType?: SemanticEntityType
  entityRole?: SemanticEntityRole
  parameters: SemanticConditionParameter[]
}

export type SemanticParameterType = ParameterType
export type SemanticEntityType = string
export type SemanticEntityRole = string

export const SemanticOperate = {
  lessThan: 'odrl:lt',
  lessThanOrEqual: 'odrl:lteq',
  greaterThan: 'odrl:gt',
  greaterThanOrEqual: 'odrl:gteq',
  equal: 'odrl:eq',
  notEqual: 'odrl:neq',
  in: 'odrl:isAnyOf',
  notIn: 'odrl:isNoneOf',
  contains: 'odrl:hasPart',
  between: 'dcs:between',
  matchesRegex: 'dcs:matchesRegex',
} as const

export type SemanticOperateType = DcsOperator

export interface SemanticParameterOperator {
  operate: SemanticOperateType
  /**
   * Target could be other parameters or basic value,
   * for example:
   * { "operate": "GreaterThan", "value": "100" }
   * { "operate": "GreaterThan", "value": "{{startDate}}" }
   */
  targets: unknown[]
}

export interface SemanticConditionParameter {
  parameterName: string
  /** The DCS field IRI (e.g. urn:uuid:field-... or did:...#field-...) — populated by contractDataToSemanticConditions. */
  fieldId?: string
  type: SemanticParameterType
  /** The ontology domain-field IRI this parameter binds (its identity); the requirement field's own @id when unbound. */
  fieldIri: string
  valueConstraint?: SemanticValueConstraint
  defaultValue?: unknown
  semanticMeaning?: string
  uiMetadata?: UiMetadata
  isRequired: boolean
  operators: SemanticParameterOperator[]
  value: unknown
}

export const SEMANTIC_CONDITION_SCHEMA_VERSION = 'v1'

export interface SemanticValueConstraint {
  format?: 'iso-3166-1-alpha-3' | 'iso-4217' | 'eidas-signature-level' | 'controlled-vocabulary'
  pattern?: string
  allowedValues?: readonly string[]
  valueOptions?: readonly SemanticValueOption[]
  allowedValuesRef?: string
  min?: number
  max?: number
  description?: string
}

export interface SemanticValueOption {
  value: string
  label?: string
  symbol?: string
}

// ---- Validation Metadata ----

export interface DomainFieldDefinition {
  /** The field's IRI — its identity in documents (dcs:domainField @id). */
  ontologyId: string
  /** The dcs:parameterName new requirement fields declare, derived from the field IRI's local name. */
  parameterName: string
  type: SemanticParameterType
  label: string
  /** The rdfs:domain class IRI grouping the field, when declared. */
  domain?: string
  domainLabel?: string
  valueConstraint?: SemanticValueConstraint
  /** The Semantic Hub schema this field was discovered in (name + kind), for grouping in the picker. */
  source?: { name: string; kind: string }
}

// ---- MetaData ----

export interface MetaData {
  name: string
  value: string
}

// ---- TemplateTypeValue ----

export const TemplateType = {
  component: 'COMPONENT',
  contractTemplate: 'CONTRACT_TEMPLATE',
} as const

export type TemplateTypeValue = (typeof TemplateType)[keyof typeof TemplateType]
