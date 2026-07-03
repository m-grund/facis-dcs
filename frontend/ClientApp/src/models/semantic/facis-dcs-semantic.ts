export type ValidationSeverity = 'info' | 'warning' | 'error' | 'blocking'
export type ParameterType = 'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'enum'
export type JsonPath = `$${string}`
export type PlaceholderRef = `{{${string}.${string}}}` | `{{${string}}}`

export type DcsOperator =
  | 'odrl:eq'
  | 'odrl:neq'
  | 'odrl:gt'
  | 'odrl:gteq'
  | 'odrl:lt'
  | 'odrl:lteq'
  | 'odrl:isAnyOf'
  | 'odrl:isNoneOf'
  | 'odrl:hasPart'
  | 'dcs:between'
  | 'dcs:matchesRegex'

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

export interface OdrlConstraint {
  '@type': 'odrl:Constraint'
  'odrl:leftOperand': unknown
  'odrl:operator': { '@id': string }
  'odrl:rightOperand': unknown
}

export interface OdrlDuty {
  '@type': 'odrl:Duty'
  '@id': string
  'odrl:constraint': OdrlConstraint[]
}

export interface PolicyBundle {
  '@type': 'PolicyBundle'
  format: 'odrl-jsonld'
  rules: OdrlDuty[]
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

export interface CompanyParty {
  '@type'?: 'CompanyParty' | 'dcs:CompanyParty'
  '@id'?: string
  role: string
  legalName?: string
  identifier?: string
  name?: string
  location?: {
    country?: string
    address?: string
  }
}

interface SemanticConditionLike {
  conditionId: string
  conditionName?: string
  parameters: {
    parameterName: string
    type: ParameterType
    isRequired?: boolean
    operators?: ({ operate: DcsOperator; targets?: unknown[] } | DcsOperator)[]
  }[]
}

interface DocumentBlockLike {
  blockId: string
  type: string
  text?: string
  conditionIds?: string[]
}

export interface SemanticTemplateRuntimeExtension {
  placeholderBindings: PlaceholderBinding[]
  semanticRules: SemanticRule[]
  policyBundle?: PolicyBundle
}

export function buildSemanticTemplateExtension(
  documentBlocks: DocumentBlockLike[],
  semanticConditions: SemanticConditionLike[],
): SemanticTemplateRuntimeExtension {
  const semanticRules = buildSemanticRulesFromConditions(documentBlocks, semanticConditions)
  return {
    placeholderBindings: buildPlaceholderBindings(documentBlocks, semanticConditions),
    semanticRules,
    policyBundle: buildPolicyBundleFromSemanticRules(semanticRules),
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
      for (const rawOperator of parameter.operators ?? []) {
        const operate = typeof rawOperator === 'string' ? rawOperator : rawOperator.operate
        const operator = normalizeSemanticOperator(operate)
        if (!operator) continue
        const targets = typeof rawOperator === 'string' ? [] : (rawOperator.targets ?? [])
        rules.push({
          '@type':
            parameter.type === 'date'
              ? 'DateConstraintRule'
              : parameter.type === 'decimal' || parameter.type === 'integer'
                ? 'ThresholdRule'
                : 'SemanticRule',
          ruleId: buildRuleId(condition.conditionId, parameter.parameterName, operator),
          conditionId: condition.conditionId,
          parameterName: parameter.parameterName,
          blockIds: blockIdsByCondition.get(condition.conditionId) ?? [],
          leftOperand: `{{${condition.conditionId}.${parameter.parameterName}}}`,
          operator,
          rightOperand: targets.length === 1 && !isSetOperator(operator) ? targets[0] : targets,
          valueType: parameter.type,
          severity: parameter.isRequired ? 'blocking' : 'error',
          source: 'semanticCondition',
          message: buildRuleMessage(
            condition.conditionName ?? condition.conditionId,
            parameter.parameterName,
            operator,
            targets,
          ),
        })
      }
    }
  }
  return rules
}

export function buildPolicyBundleFromSemanticRules(semanticRules: SemanticRule[]): PolicyBundle | undefined {
  const rules = semanticRules
    .map((rule): OdrlDuty | null => {
      const operator = odrlOperatorFor(rule.operator)
      if (!operator) return null
      return {
        '@type': 'odrl:Duty',
        '@id': `${rule.ruleId}-duty`,
        'odrl:constraint': [
          {
            '@type': 'odrl:Constraint',
            'odrl:leftOperand': rule.leftOperand,
            'odrl:operator': { '@id': operator },
            'odrl:rightOperand': rule.rightOperand,
          },
        ],
      }
    })
    .filter((rule): rule is OdrlDuty => rule !== null)

  if (!rules.length) return undefined
  return {
    '@type': 'PolicyBundle',
    format: 'odrl-jsonld',
    rules,
  }
}

export function normalizeSemanticOperator(value: string): DcsOperator | null {
  return isDcsOperator(value) ? value : null
}

function odrlOperatorFor(operator: DcsOperator): string | null {
  return isStandardOdrlOperator(operator) ? operator : null
}

function isSetOperator(operator: DcsOperator): boolean {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf'
}

function isStandardOdrlOperator(value: string): value is Exclude<DcsOperator, 'dcs:between' | 'dcs:matchesRegex'> {
  return [
    'odrl:eq',
    'odrl:neq',
    'odrl:gt',
    'odrl:gteq',
    'odrl:lt',
    'odrl:lteq',
    'odrl:isAnyOf',
    'odrl:isNoneOf',
    'odrl:hasPart',
  ].includes(value)
}

function isDcsOperator(value: string): value is DcsOperator {
  return [
    'odrl:eq',
    'odrl:neq',
    'odrl:gt',
    'odrl:gteq',
    'odrl:lt',
    'odrl:lteq',
    'odrl:isAnyOf',
    'odrl:isNoneOf',
    'odrl:hasPart',
    'dcs:between',
    'dcs:matchesRegex',
  ].includes(value)
}

function buildRuleId(conditionId: string, parameterName: string, operator: DcsOperator): string {
  return `rule-${slugify(conditionId)}-${slugify(parameterName)}-${slugify(operator)}`
}

function buildRuleMessage(
  conditionName: string,
  parameterName: string,
  operator: DcsOperator,
  targets: unknown[],
): string {
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
