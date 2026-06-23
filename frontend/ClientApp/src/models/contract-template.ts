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
  TemplateVariable,
  PlaceholderBinding,
  PolicyBundle,
  SemanticRule,
  SLAAgreement,
} from './semantic/facis-dcs-semantic'
import type { DcsTemplateData } from './dcs-jsonld'

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
  template_data?: DcsTemplateData
  updated_at: string
  responsible?: ContractTemplateResponsible
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
  templateVariables?: TemplateVariable[]
  placeholderBindings?: PlaceholderBinding[]
  semanticRules?: SemanticRule[]
  policyBundle?: PolicyBundle
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
  template_data?: DcsTemplateData
}
