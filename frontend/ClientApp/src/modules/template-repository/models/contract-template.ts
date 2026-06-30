import type {
  DcsOperator,
  ParameterType,
  UiMetadata,
} from '@/models/semantic/facis-dcs-semantic'

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
  schemaRef: string
  semanticPath: DomainSemanticPath
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

export const FACIS_SCHEMA_REFS = {
  documentStructure: 'facis.dcs.document-structure.v1',
  templateData: 'facis.dcs.template-data.v1',
  contractData: 'facis.dcs.contract-data.v1',
  semanticCondition: 'facis.dcs.semantic-condition.v1',
  party: 'facis.dcs.party.v1',
  contract: 'facis.dcs.contract.v1',
  service: 'facis.dcs.service.v1',
  signature: 'facis.dcs.signature.v1',
} as const

export type DomainSemanticPath = string

export interface DomainFieldDefinition {
  ontologyId: string
  semanticPath: DomainSemanticPath
  schemaRef: string
  type: SemanticParameterType
  label: string
  statementType?: string
  statementTypeLabel?: string
  valueConstraint?: SemanticValueConstraint
}

export interface SchemaReferenceSet {
  documentStructure: string
  semanticCondition: string
  templateData?: string
  contractData?: string
  jsonLdContext?: string
  ontology?: string
  shaclShapes?: string
}

export interface PolicyReference {
  policyId: string
  version: string
  enforcementPoint:
    | 'template:create'
    | 'template:update'
    | 'template:verify'
    | 'contract:create'
    | 'contract:update'
    | 'contract:submit'
}

export interface ValidationProfile {
  schemaVersion: 'v1'
  profile: 'FACIS_DCS_TEMPLATE_V1' | 'FACIS_DCS_CONTRACT_V1'
  requiredPolicies: string[]
}

export const FACIS_TEMPLATE_POLICY_REFS: PolicyReference[] = [
  { policyId: 'facis.dcs.template.structure', version: 'v1', enforcementPoint: 'template:create' },
  { policyId: 'facis.dcs.template.semantic-conditions', version: 'v1', enforcementPoint: 'template:verify' },
]

export const FACIS_CONTRACT_POLICY_REFS: PolicyReference[] = [
  { policyId: 'facis.dcs.contract.structure', version: 'v1', enforcementPoint: 'contract:create' },
  { policyId: 'facis.dcs.contract.semantic-values', version: 'v1', enforcementPoint: 'contract:update' },
]

export const FACIS_TEMPLATE_VALIDATION_PROFILE: ValidationProfile = {
  schemaVersion: 'v1',
  profile: 'FACIS_DCS_TEMPLATE_V1',
  requiredPolicies: FACIS_TEMPLATE_POLICY_REFS.map((policy) => policy.policyId),
}

export const FACIS_CONTRACT_VALIDATION_PROFILE: ValidationProfile = {
  schemaVersion: 'v1',
  profile: 'FACIS_DCS_CONTRACT_V1',
  requiredPolicies: FACIS_CONTRACT_POLICY_REFS.map((policy) => policy.policyId),
}

// ---- MetaData ----

export interface MetaData {
  name: string
  value: string
}

// ---- TemplateTypeValue ----

export const TemplateType = {
  subContract: 'SUB_CONTRACT',
  frameContract: 'FRAME_CONTRACT',
} as const

export type TemplateTypeValue = (typeof TemplateType)[keyof typeof TemplateType]
