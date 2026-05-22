export type DcsDid = `did:${string}`
export type DcsUuidUrn = `urn:uuid:${string}`
export type HashRef = `${'sha256' | 'sha384' | 'sha512'}:${string}`
export type DateTimeString = string
export type DateString = string
export type JsonPath = `$${string}`
export type PlaceholderRef = `{{${string}.${string}}}` | `{{${string}}}`

export type SemanticProfileVersion = 'v1'

export interface SemanticProfile {
  name: 'FACIS DCS Semantic Contract Profile'
  version: SemanticProfileVersion
  context: string
  ontology: string
  shapes?: string
}

export type ParameterType = 'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'enum'

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

export type LegacySemanticOperate =
  | 'equal'
  | 'notEqual'
  | 'greaterThan'
  | 'greaterThanOrEqual'
  | 'lessThan'
  | 'lessThanOrEqual'
  | 'between'
  | 'contains'
  | 'matchesRegex'

export const LEGACY_OPERATOR_TO_DCS: Record<LegacySemanticOperate, DcsOperator> = {
  equal: 'Equals',
  notEqual: 'NotEquals',
  greaterThan: 'GreaterThan',
  greaterThanOrEqual: 'GreaterThanOrEqual',
  lessThan: 'LessThan',
  lessThanOrEqual: 'LessThanOrEqual',
  between: 'Between',
  contains: 'Contains',
  matchesRegex: 'MatchesRegex',
}

export interface UiMetadata {
  label?: string
  description?: string
  input?: 'text' | 'textarea' | 'number' | 'date' | 'checkbox' | 'select' | 'country' | 'url'
  placeholder?: string
  suffix?: string
  order?: number
  group?: string
}

export interface ParameterConstraint {
  '@type'?: 'ParameterConstraint' | 'Constraint'
  operator: DcsOperator
  rightOperand?: unknown
  minValue?: string | number
  maxValue?: string | number
  allowedValues?: Array<string | number | boolean>
  regexPattern?: string
  message?: string
  severity?: ValidationSeverity
}

export interface ValueConstraint {
  format?: string
  pattern?: string
  allowedValues?: string[]
  allowedValuesRef?: string
  min?: number
  max?: number
  description?: string
}

export interface TemplateVariable {
  '@type'?: 'TemplateVariable' | 'Parameter' | 'StringParameter' | 'DecimalParameter' | 'IntegerParameter' | 'BooleanParameter' | 'DateParameter' | 'EnumParameter'
  parameterName: string
  parameterType: ParameterType
  required: boolean
  defaultValue?: unknown
  semanticMeaning?: string
  uiMetadata?: UiMetadata
  constraints?: ParameterConstraint[]
}

export interface Parameter extends TemplateVariable {
  '@type'?: 'Parameter' | 'StringParameter' | 'DecimalParameter' | 'IntegerParameter' | 'BooleanParameter' | 'DateParameter' | 'EnumParameter'
  parameterValue?: unknown
}

export interface PlaceholderBinding {
  '@type'?: 'PlaceholderBinding'
  placeholder: PlaceholderRef
  boundToCondition: string
  boundToParameter: string
  blockId: string
}

export interface LegacySemanticParameterOperator {
  operate: LegacySemanticOperate
  targets: string[]
}

export interface LegacySemanticConditionParameter {
  parameterName: string
  type: ParameterType
  isRequired: boolean
  operators: LegacySemanticParameterOperator[]
  schemaRef?: string
  semanticPath?: string
  valueConstraint?: ValueConstraint
  value?: unknown
  defaultValue?: unknown
  semanticMeaning?: string
  uiMetadata?: UiMetadata
}

export interface SemanticCondition {
  '@type'?: 'SemanticCondition'
  conditionId: string
  conditionName: string
  schemaVersion: 'v1'
  parameters: LegacySemanticConditionParameter[]
  appliesToClause?: string[]
}

export interface SemanticConditionValue {
  blockId: string
  conditionId: string
  parameterName: string
  parameterValue?: string | number | boolean | null
}

export type DocumentBlockType = 'SECTION' | 'TEXT' | 'CLAUSE' | 'APPROVED_TEMPLATE' | 'MERGED_APPROVED_TEMPLATE'

export interface DocumentOutlineBlock {
  blockId: string
  isRoot?: boolean
  children: string[]
}

export interface DocumentBlock {
  blockId: string
  type: DocumentBlockType
  title?: string
  text: string
  conditionIds?: string[]
  version?: number
  templateId?: string
  document_number?: string
  contentHash?: HashRef | string
}

export interface SemanticTemplateDataExtension {
  semanticProfile?: SemanticProfile
  templateVariables?: TemplateVariable[]
  placeholderBindings?: PlaceholderBinding[]
  semanticRules?: SemanticRule[]
}

export interface SemanticContractDataExtension extends SemanticTemplateDataExtension {
  semanticConditionValues: SemanticConditionValue[]
  validationReports?: ValidationReport[]
}

export interface DcsTemplateData extends SemanticTemplateDataExtension {
  documentOutline: DocumentOutlineBlock[]
  documentBlocks: DocumentBlock[]
  semanticConditions: SemanticCondition[]
  customMetaData: Array<{ name: string; value: string }>
  subTemplateSnapshots?: unknown[]
  templateDataVersion: number
}

export interface DcsContractData extends SemanticContractDataExtension {
  documentOutline: DocumentOutlineBlock[]
  documentBlocks: DocumentBlock[]
  semanticConditions: SemanticCondition[]
  subTemplateSnapshots: unknown[]
  templateDataVersion: number
}

export type ContractLifecycleState =
  | 'Draft'
  | 'Offered'
  | 'InNegotiation'
  | 'SubmittedForReview'
  | 'Reviewed'
  | 'Approved'
  | 'ReadyForSignature'
  | 'Signed'
  | 'Executed'
  | 'Deployed'
  | 'Active'
  | 'Suspended'
  | 'Terminated'
  | 'Expired'
  | 'Archived'
  | 'Revoked'
  | 'Replaced'

export interface Party {
  '@type'?: 'Party' | 'Company' | 'Signatory'
  identifier: string
  role: ContractPartyRole | string
  name: string
  did?: DcsDid | string
  uuid?: DcsUuidUrn | string
  country?: string
  credentialReferences?: CredentialReference[]
}

export type ContractPartyRole = 'supplier' | 'customer' | 'provider' | 'client'

export interface CompanyLocation {
  street?: string
  postalCode?: string
  city?: string
  region?: string
  country: string
}

export interface Company extends Party {
  '@type'?: 'Company'
  legalName: string
  registrationNumber?: string
  vatId?: string
  location?: CompanyLocation
}

export interface Signatory extends Party {
  '@type'?: 'Signatory'
  signatureLevel?: 'SES' | 'AES' | 'QES'
  signingOrder?: number
}

export interface CredentialReference {
  '@type'?: 'CredentialReference'
  credentialId: string
  credentialType: 'OrganizationCredential' | 'PowerOfAttorneyCredential' | 'IdentityCredential' | 'RoleCredential' | string
  issuer?: DcsDid | string
  subject?: DcsDid | string
  statusListRef?: string
  proofHash?: HashRef | string
  validFrom?: DateTimeString
  validUntil?: DateTimeString
}

export interface ContractVersion {
  '@type'?: 'ContractVersion'
  contractVersion: number
  createdAt: DateTimeString
  createdBy?: string
  contentHash: HashRef | string
  changedClauses?: string[]
  previousVersion?: number
}

export interface Clause {
  '@type'?: 'Clause'
  blockId: string
  clauseId?: string
  clauseVersion: number
  conditionIds?: string[]
  contentHash?: HashRef | string
}

export type SloType = 'availability' | 'responseTime' | 'resolutionTime' | 'errorRate' | 'throughput'

export interface Service {
  '@type'?: 'Service'
  serviceId: string
  name: string
  targetEndpoint?: string
  serviceType?: string
  slos: SLO[]
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

export interface Remedy {
  '@type'?: 'Remedy' | 'ServiceCredit'
  identifier: string
  description?: string
  creditPercentage?: number
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

export type ValidationSeverity = 'info' | 'warning' | 'error' | 'blocking'

export interface SemanticRule {
  '@type'?: 'SemanticRule' | 'ThresholdRule' | 'DateConstraintRule'
  ruleId: string
  appliesToClause?: string[]
  leftOperand: JsonPath | PlaceholderRef
  operator: DcsOperator
  rightOperand: unknown
  valueType: ParameterType
  severity: ValidationSeverity
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
  createdAt: DateTimeString
  source: 'runtime' | 'shacl' | 'policy' | 'credential'
  findings: ValidationFinding[]
  contentHash?: HashRef | string
}

export interface ContractAdjustmentOperation {
  operation: 'add' | 'remove' | 'replace' | 'move' | 'setValue'
  targetType: 'Clause' | 'Section' | 'Parameter' | 'SemanticCondition'
  targetId: string
  path: JsonPath
  oldHash?: HashRef | string
  newHash?: HashRef | string
}

export interface ContractAdjustment {
  '@type'?: 'ContractAdjustment'
  adjustmentId: DcsUuidUrn | string
  contractDid: DcsDid | string
  baseVersion: number
  contractVersion?: number
  operations: ContractAdjustmentOperation[]
  semanticImpact?: {
    conditionIds: string[]
    requiresRevalidation: boolean
  }
}

export interface PolicyBundle {
  '@type'?: 'PolicyBundle'
  format: 'odrl-jsonld' | 'rego-json' | 'gateway-policy-json' | string
  rules: string[]
  credentialRequirements?: string[]
  contentHash?: HashRef | string
}

export interface DeploymentReceipt {
  '@type'?: 'DeploymentReceipt'
  receiptId: DcsUuidUrn | string
  state: 'Requested' | 'Acknowledged' | 'Failed' | 'Activated' | string
  occurredAt: DateTimeString
  contentHash?: HashRef | string
}

export interface Deployment {
  '@type'?: 'Deployment'
  identifier: string
  targetSystem: string
  targetEndpoint: string
  contractVersion: number
  correlationId?: string
  policyBundle: PolicyBundle
  receipt?: DeploymentReceipt
}

export interface ProvenanceEvent {
  '@type'?: 'ProvenanceEvent'
  eventId: DcsUuidUrn | string
  eventType: string
  actor: DcsDid | string
  actorRole: string
  credentialRef?: string
  occurredAt: DateTimeString
  entity?: DcsDid | string
  entityVersion?: number
  contentHash?: HashRef | string
  previousEventHash?: HashRef | string
}

export interface C2PAManifestReference {
  manifestUrl: string
  fileHash: HashRef | string
  status: 'draft' | 'active' | 'amended' | 'suspended' | 'terminated' | 'expired' | 'replaced' | string
  reason?: string
  effectiveAt: DateTimeString
  authority: DcsDid | string
  vcId?: string
  previousManifestHash?: HashRef | string
}

export interface DcsSemanticContract {
  '@context': string | Record<string, unknown> | Array<string | Record<string, unknown>>
  '@id': DcsDid | string
  '@type': 'Contract'
  did: DcsDid | string
  uuid?: DcsUuidUrn | string
  contractVersion: number
  state: string
  lifecycleState: ContractLifecycleState
  name?: string
  description?: string
  createdAt: DateTimeString
  updatedAt: DateTimeString
  validFrom?: DateTimeString
  validUntil?: DateTimeString
  derivedFromTemplate?: DcsDid | string
  templateVersion?: number
  semanticProfile: SemanticProfile
  parties: Company[]
  signatories?: Signatory[]
  contractData: DcsContractData
  clauses?: Clause[]
  contractVersions?: ContractVersion[]
  adjustments?: ContractAdjustment[]
  sla?: SLAAgreement
  semanticRules?: SemanticRule[]
  validationReports?: ValidationReport[]
  deployment?: Deployment
  provenance?: ProvenanceEvent[]
  c2paManifest?: C2PAManifestReference
  statusCredential?: Record<string, unknown>
  contentHash?: HashRef | string
}

export interface DcsSemanticContractTemplate {
  '@context': string | Record<string, unknown> | Array<string | Record<string, unknown>>
  '@id': DcsDid | string
  '@type': 'ContractTemplate'
  did: DcsDid | string
  uuid?: DcsUuidUrn | string
  documentNumber?: string
  templateVersion: number
  schemaVersion: 'v1'
  semanticProfile: SemanticProfile
  name: string
  description?: string
  createdAt: DateTimeString
  updatedAt: DateTimeString
  template_data: DcsTemplateData
  sla?: SLAAgreement
  semanticRules?: SemanticRule[]
  provenance?: ProvenanceEvent[]
}
