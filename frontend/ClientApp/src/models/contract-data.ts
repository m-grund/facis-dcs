import type {
  DocumentBlock,
  DocumentOutline,
  PolicyReference,
  SchemaReferenceSet,
  SemanticCondition,
  ValidationProfile,
} from '@/modules/template-repository/models/contract-template'
import type { TemplateDataVersion } from '@/modules/template-repository/models/template-draft-store'
import type {
  CompanyParty,
  PlaceholderBinding,
  PolicyBundle,
  SemanticRule,
  SLAAgreement,
  TemplateVariable,
  ValidationReport,
} from '@/models/semantic/facis-dcs-semantic'
import type { SubTemplateSnapshot } from './contract-template'
import type { DcsContractData } from './dcs-jsonld'

export interface ContractData extends Partial<DcsContractData> {
  documentOutline?: DocumentOutline
  documentBlocks?: DocumentBlock[]
  semanticConditions?: SemanticCondition[]
  subTemplateSnapshots?: SubTemplateSnapshot[]
  templateDataVersion?: TemplateDataVersion
  schemaRefs?: SchemaReferenceSet
  policyRefs?: PolicyReference[]
  validation?: ValidationProfile
  sourceTemplate?: {
    did: string
    version?: number
    document_number?: string
  }
  templateVariables?: TemplateVariable[]
  placeholderBindings?: PlaceholderBinding[]
  semanticRules?: SemanticRule[]
  policyBundle?: PolicyBundle
  parties?: CompanyParty[]
  sla?: SLAAgreement
  validationReports?: ValidationReport[]
  semanticConditionValues?: SemanticConditionValue[]
}

export interface SemanticConditionValue {
  /** Block ID from top-level template_data.documentBlocks */
  blockId: string
  conditionId: string
  parameterName: string
  parameterValue?: string | number | boolean
}
