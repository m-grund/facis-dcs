import type { TemplateDataVersion } from '@/modules/template-repository/models/template-draft-store'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { TemplateType } from '@/types/template-type'
import type { ContractTemplateResponsible } from './contract-template-responsible'
import type {
  SchemaReferenceSet,
  PolicyReference,
  ValidationProfile,
  DocumentOutline,
  DocumentBlock,
  MetaData,
  SemanticCondition,
} from '@/modules/template-repository/models/contract-template'
import type {
  SemanticProfile,
  TemplateVariable,
  PlaceholderBinding,
  SemanticRule,
  SLAAgreement,
} from './semantic/facis-dcs-semantic'

export interface ContractTemplate {
  did: string
  created_by: string
  created_at: string
  document_number?: string
  version: number
  template_type: TemplateType
  state: ContractTemplateState
  name?: string
  description?: string
  template_data?: ContractTemplateData
  updated_at: string
  responsible?: ContractTemplateResponsible
  outdated: boolean
}

export type PartialContractTemplate = ContractTemplate

export interface ContractTemplateData {
  '@context'?: string
  documentOutline: DocumentOutline
  semanticConditions: SemanticCondition[]
  documentBlocks: DocumentBlock[]
  customMetaData: MetaData[]
  schemaRefs?: SchemaReferenceSet
  policyRefs?: PolicyReference[]
  validation?: ValidationProfile
  semanticProfile?: SemanticProfile
  templateVariables?: TemplateVariable[]
  placeholderBindings?: PlaceholderBinding[]
  semanticRules?: SemanticRule[]
  sla?: SLAAgreement
  // Only when the template is a frame contract, it can have sub-templates
  subTemplateSnapshots?: SubTemplateSnapshot[]
  templateDataVersion?: TemplateDataVersion
}

export interface SubTemplateSnapshot {
  did: string
  document_number?: string
  version: number
  name?: string
  description?: string
  template_data?: Omit<ContractTemplateData, 'subTemplateSnapshots' | 'templateDataVersion'>
}
