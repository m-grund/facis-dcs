import type { ContractTemplateState } from '@/types/contract-template-state'
import type { TemplateType } from '@/types/template-type'
import type { ContractTemplateResponsible } from './contract-template-responsible'
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
  outdated?: boolean
  latest_did?: string
}

export type PartialContractTemplate = ContractTemplate

export type ContractTemplateData = DcsTemplateData

export interface SubTemplateSnapshot {
  did: string
  document_number?: string
  version: number
  name?: string
  description?: string
  template_data?: DcsTemplateData
}
