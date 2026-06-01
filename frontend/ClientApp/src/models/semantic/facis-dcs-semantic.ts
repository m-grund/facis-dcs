export type SemanticProfileVersion = 'v1'
export type ValidationSeverity = 'info' | 'warning' | 'error' | 'blocking'
export type ParameterType = 'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'enum'
export type JsonPath = `$${string}`
export type PlaceholderRef = `{{${string}.${string}}}` | `{{${string}}}`

export interface SemanticProfile {
  name: 'FACIS DCS Semantic Contract Profile'
  version: SemanticProfileVersion
  context: string
  ontology: string
  shapes?: string
}

export const FACIS_DCS_SEMANTIC_PROFILE: SemanticProfile = {
  name: 'FACIS DCS Semantic Contract Profile',
  version: 'v1',
  context: 'https://w3id.org/facis/dcs/context/v1',
  ontology: 'https://w3id.org/facis/sla/ontology',
  shapes: 'https://w3id.org/facis/dcs/shapes/v1',
}

export type DcsOperator =
  | 'Equals'
  | 'NotEquals'
  | 'GreaterThan'
  | 'GreaterThanOrEqual'
  | 'LessThan'
  | 'LessThanOrEqual'
  | 'Between'
  | 'Contains'
  | 'MatchesRegex'

export interface UiMetadata {
  label?: string
  description?: string
  input?: 'text' | 'textarea' | 'number' | 'date' | 'checkbox' | 'select' | 'country' | 'url'
  placeholder?: string
  suffix?: string
  order?: number
  group?: string
}

export interface TemplateVariable {
  '@type'?: 'TemplateVariable'
  parameterName: string
  parameterType: ParameterType
  required: boolean
  defaultValue?: unknown
  semanticMeaning?: string
  uiMetadata?: UiMetadata
}

export interface PlaceholderBinding {
  '@type'?: 'PlaceholderBinding'
  placeholder: PlaceholderRef
  boundToCondition: string
  boundToParameter: string
  blockId: string
  source?: 'clause-placeholder' | 'manual'
}

export interface SemanticRule {
  '@type'?: 'SemanticRule' | 'ThresholdRule' | 'DateConstraintRule'
  ruleId: string
  conditionId?: string
  parameterName?: string
  blockIds?: string[]
  leftOperand: JsonPath | PlaceholderRef
  operator: DcsOperator
  rightOperand: unknown
  valueType: ParameterType
  severity: ValidationSeverity
  source?: 'semanticCondition' | 'manual' | 'sla'
  message?: string
}

export interface ValidationFinding {
  '@type'?: 'ValidationFinding'
  ruleId: string
  severity: ValidationSeverity
  path?: string
  message: string
  source: 'runtime' | 'shacl' | 'policy' | 'credential'
}

export interface ValidationReport {
  '@type'?: 'ValidationReport'
  identifier: string
  createdAt: string
  source: 'runtime' | 'shacl' | 'policy' | 'credential'
  findings: ValidationFinding[]
  contentHash?: string
}

export interface MeasurementMetric {
  '@type'?: 'MeasurementMetric'
  metricId: string
  name: string
  unit?: string
  source?: string
}

export interface SLI {
  '@type'?: 'SLI'
  metric: MeasurementMetric
}

export interface MeasurementRule {
  '@type'?: 'MeasurementRule'
  identifier: string
  measurementWindow: string
  leftOperand: JsonPath
  operator: DcsOperator
  rightOperand: unknown
  valueType: ParameterType
}

export interface Remedy {
  '@type'?: 'Remedy' | 'ServiceCredit'
  identifier: string
  description?: string
  creditPercentage?: number
}

export type SloType = 'availability' | 'responseTime' | 'resolutionTime' | 'errorRate' | 'throughput'

export interface SLO {
  '@type'?: 'SLO'
  sloType: SloType
  targetValue: number
  unit?: string
  operator?: DcsOperator
  measurementWindow?: string
  identifier?: string
  name?: string
  sli?: SLI
  measurementRules?: MeasurementRule[]
  remedies?: Remedy[]
}

export interface Service {
  '@type'?: 'Service'
  serviceId: string
  name: string
  targetEndpoint?: string
  serviceType?: string
  slos: SLO[]
}

export interface ClaimPolicy {
  '@type'?: 'ClaimPolicy'
  identifier: string
  description?: string
  allowedClaimWindow?: string
}

export interface ExclusionEvent {
  '@type'?: 'ExclusionEvent'
  identifier: string
  name?: string
  description?: string
}

export interface SLAAgreement {
  '@type'?: 'SLAAgreement'
  services: Service[]
  claimPolicy?: ClaimPolicy
  exclusionEvents?: ExclusionEvent[]
}

type SemanticConditionLike = {
  conditionId: string
  conditionName?: string
  parameters: {
    parameterName: string
    type: ParameterType
    isRequired?: boolean
    fixedValue?: unknown
    operators?: Array<{ operate: DcsOperator; targets?: string[] } | DcsOperator>
  }[]
}

type DocumentBlockLike = {
  blockId: string
  type: string
  text?: string
  conditionIds?: string[]
}

export interface SemanticTemplateRuntimeExtension {
  semanticProfile: SemanticProfile
  placeholderBindings: PlaceholderBinding[]
  semanticRules: SemanticRule[]
}

export function buildSemanticTemplateExtension(
  documentBlocks: DocumentBlockLike[],
  semanticConditions: SemanticConditionLike[],
  semanticProfile: SemanticProfile = FACIS_DCS_SEMANTIC_PROFILE,
): SemanticTemplateRuntimeExtension {
  return {
    semanticProfile,
    placeholderBindings: buildPlaceholderBindings(documentBlocks, semanticConditions),
    semanticRules: buildSemanticRulesFromConditions(documentBlocks, semanticConditions),
  }
}

export function buildPlaceholderBindings(
  documentBlocks: DocumentBlockLike[],
  semanticConditions: SemanticConditionLike[],
): PlaceholderBinding[] {
  const conditionById = new Map(semanticConditions.map((condition) => [condition.conditionId, condition]))
  const bindings = new Map<string, PlaceholderBinding>()
  const placeholderPattern = /\{\{([^}.]+)\.([^}]+)\}\}/g

  for (const block of documentBlocks) {
    if (block.type !== 'CLAUSE' || !block.text) continue
    placeholderPattern.lastIndex = 0
    for (const match of block.text.matchAll(placeholderPattern)) {
      const conditionId = match[1]
      const parameterName = match[2]
      if (!conditionId || !parameterName) continue
      const condition = conditionById.get(conditionId)
      if (!condition?.parameters.some((param) => param.parameterName === parameterName)) continue
      const key = `${block.blockId}:${conditionId}:${parameterName}`
      bindings.set(key, {
        '@type': 'PlaceholderBinding',
        placeholder: `{{${conditionId}.${parameterName}}}`,
        boundToCondition: conditionId,
        boundToParameter: parameterName,
        blockId: block.blockId,
        source: 'clause-placeholder',
      })
    }
  }

  return [...bindings.values()]
}

export function buildSemanticRulesFromConditions(
  documentBlocks: DocumentBlockLike[],
  semanticConditions: SemanticConditionLike[],
): SemanticRule[] {
  const blockIdsByCondition = new Map<string, string[]>()
  for (const block of documentBlocks) {
    if (block.type !== 'CLAUSE') continue
    for (const conditionId of block.conditionIds ?? []) {
      blockIdsByCondition.set(conditionId, [...(blockIdsByCondition.get(conditionId) ?? []), block.blockId])
    }
  }

  const rules: SemanticRule[] = []
  for (const condition of semanticConditions) {
    for (const parameter of condition.parameters) {
      if (parameter.fixedValue !== undefined && parameter.fixedValue !== null && parameter.fixedValue !== '') continue
      for (const rawOperator of parameter.operators ?? []) {
        const operate = typeof rawOperator === 'string' ? rawOperator : rawOperator.operate
        const operator = normalizeSemanticOperator(operate)
        if (!operator) continue
        const targets = typeof rawOperator === 'string' ? [] : rawOperator.targets ?? []
        rules.push({
          '@type': parameter.type === 'date' ? 'DateConstraintRule' : parameter.type === 'decimal' || parameter.type === 'integer' ? 'ThresholdRule' : 'SemanticRule',
          ruleId: buildRuleId(condition.conditionId, parameter.parameterName, operator),
          conditionId: condition.conditionId,
          parameterName: parameter.parameterName,
          blockIds: blockIdsByCondition.get(condition.conditionId) ?? [],
          leftOperand: `{{${condition.conditionId}.${parameter.parameterName}}}`,
          operator,
          rightOperand: targets.length === 1 ? targets[0] : targets,
          valueType: parameter.type,
          severity: parameter.isRequired ? 'blocking' : 'error',
          source: 'semanticCondition',
          message: buildRuleMessage(condition.conditionName ?? condition.conditionId, parameter.parameterName, operator, targets),
        })
      }
    }
  }
  return rules
}

export function normalizeSemanticOperator(value: string): DcsOperator | null {
  return isDcsOperator(value) ? value : null
}

function isDcsOperator(value: string): value is DcsOperator {
  return [
    'Equals',
    'NotEquals',
    'GreaterThan',
    'GreaterThanOrEqual',
    'LessThan',
    'LessThanOrEqual',
    'Between',
    'Contains',
    'MatchesRegex',
  ].includes(value)
}

function buildRuleId(conditionId: string, parameterName: string, operator: DcsOperator): string {
  return `rule-${slugify(conditionId)}-${slugify(parameterName)}-${slugify(operator)}`
}

function buildRuleMessage(conditionName: string, parameterName: string, operator: DcsOperator, targets: string[]): string {
  const target = targets.length ? ` ${targets.join(', ')}` : ''
  return `${conditionName}.${parameterName} must satisfy ${operator}${target}.`
}

function slugify(value: string): string {
  return value
    .replace(/([a-z])([A-Z])/g, '$1-$2')
    .replace(/[^a-zA-Z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .toLowerCase()
}
