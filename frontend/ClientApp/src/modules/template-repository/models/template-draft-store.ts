import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { ContractTemplateResponsible } from '@/models/contract-template-responsible'
import type { DcsBlock, DcsDataRequirement, DcsLayoutNode, OdrlRule } from '@/models/dcs-jsonld'
import type { MetaData, TemplateTypeValue } from '@/modules/template-repository/models/contract-template'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'

export const TEMPLATE_DATA_VERSIONS = [1] as const
export type TemplateDataVersion = (typeof TEMPLATE_DATA_VERSIONS)[number]

interface TemplateDraftState {
  did: string | null
  /** The document's @id — its dereferenceable resource IRI; authored fragments anchor to it. */
  documentIri: string | null
  name: string
  description: string
  templateDataVersion: TemplateDataVersion
  /** JSON-LD blocks (canonical + virtual merged blocks for frame-contract editing). */
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[]
  /** JSON-LD layout tree. */
  layout: DcsLayoutNode[]
  /** JSON-LD data requirements (replaces semanticConditions as stored state). */
  contractData: DcsDataRequirement[]
  /** JSON-LD ODRL policies (operator constraints). */
  policies: OdrlRule[]
  customMetaData: MetaData[]
  subTemplateSnapshots: SubTemplateSnapshot[]
  templateType: TemplateTypeValue
  state: ContractTemplateState | undefined
  document_number: string | null
  version: number | null
  updated_at: string | null
  created_by: string
  responsible: ContractTemplateResponsible | null
  workflow: 'contract' | 'template'
}

/** Block types in JSON-LD @type notation. */
export type NewBlockType = 'dcs:Section' | 'dcs:TextBlock' | 'dcs:Clause' | 'dcs:ApprovedTemplate'

/** Payload for adding a new block. */
export interface AddBlockPayload {
  blockType: NewBlockType
  text?: string
  title?: string
  // #### For Clause ####
  clauseBlockId?: string
  content?: import('@/models/dcs-jsonld').DcsContentSegment[]
  /** Typed clause instance (ADR-10) nested into the new dcs:Clause block. */
  typedClause?: import('@/models/dcs-jsonld').DcsTypedClauseInstance
  blockCatalogueId?: string
  schemaRef?: string
  semanticPath?: string
  // #### For ApprovedTemplate ####
  templateId?: string
  version?: number
  document_number?: string
  merged_approved_block_id?: string
}

export interface AddBlockOptions {
  addToOutline?: boolean
}

export type SubTemplateReference = Pick<SubTemplateSnapshot, 'did' | 'version' | 'document_number'>

export type { TemplateDraftState }
