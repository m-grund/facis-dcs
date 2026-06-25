import type { ContractTemplateData } from '@/models/contract-template'

export interface TemplateResource {
  did: string
  document_number?: string
  version: number
  name?: string
  description?: string
  template_type?: string
  participant_id?: string
  created_at?: string
  updated_at?: string
  template_data?: ContractTemplateData
}

export interface TemplateResourcesItem {
  did: string
  document_number?: string
  version: number
  name?: string
  description?: string
  template_type?: string
  participant_id?: string
  created_at?: string
  updated_at?: string
}
