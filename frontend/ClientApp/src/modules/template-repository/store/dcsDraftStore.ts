import { defineStore } from 'pinia'
import type {
  TemplateDraftState,
  AddBlockPayload,
  AddBlockOptions,
  SubTemplateReference,
} from '@template-repository/models/template-draft-store'
import type {
  DocumentOutline,
  DocumentOutlineBlock,
  DocumentBlock,
  TemplateTypeValue,
  SemanticCondition,
  MetaData,
} from '@template-repository/models/contract-template'
import {
  DocumentBlockType,
  TemplateType,
  isClauseBlock,
  isTextBlock,
  isSectionBlock,
  isApprovedTemplateBlock,
  FACIS_SCHEMA_REFS,
  FACIS_TEMPLATE_POLICY_REFS,
  FACIS_TEMPLATE_VALIDATION_PROFILE,
} from '@template-repository/models/contract-template'
import type { ContractTemplate, SubTemplateSnapshot } from '@/models/contract-template'
import type { ContractTemplateCreateRequest, ContractTemplateUpdateRequest } from '@/models/requests/template-request'
import { FACIS_DCS_SEMANTIC_PROFILE } from '@/models/semantic/facis-dcs-semantic'
import type {
  SemanticConditionParameter,
  SemanticParameterOperator,
} from '@template-repository/models/contract-template'
import type { DcsOperator } from '@/models/semantic/facis-dcs-semantic'
import { isSameTemplateDataRef } from '@template-repository/utils/template-data-ref'
import {
  isDcsTemplateData,
  DCS_JSONLD_CONTEXT,
  type DcsTemplateData,
  type DcsBlock,
  type DcsLayoutNode,
  type DcsContentSegment,
  type DcsParameterRef,
  type OdrlSet,
  type OdrlDuty,
  type OdrlConstraint,
} from '@/models/dcs-jsonld'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { ContractTemplateResponsible } from '@/models/contract-template-responsible'

const storeId = 'dcsDraft'

const defaultState: Readonly<TemplateDraftState> = {
  did: null,
  name: '',
  description: '',
  templateDataVersion: 1,
  documentOutline: [],
  documentBlocks: [],
  semanticConditions: [],
  customMetaData: [],
  schemaRefs: {
    documentStructure: FACIS_SCHEMA_REFS.documentStructure,
    semanticCondition: FACIS_SCHEMA_REFS.semanticCondition,
    templateData: FACIS_SCHEMA_REFS.templateData,
  },
  policyRefs: FACIS_TEMPLATE_POLICY_REFS,
  validation: FACIS_TEMPLATE_VALIDATION_PROFILE,
  semanticProfile: FACIS_DCS_SEMANTIC_PROFILE,
  templateVariables: [],
  placeholderBindings: [],
  semanticRules: [],
  policyBundle: null,
  sla: null,
  subTemplateSnapshots: [],
  templateType: TemplateType.subContract,
  state: null,
  document_number: null,
  version: null,
  updated_at: null,
  created_by: '',
  responsible: null,
  workflow: 'template',
}

export interface LoadDocumentMeta {
  did: string
  name: string
  description: string
  templateType?: TemplateTypeValue
  state?: ContractTemplateState | null
  version?: number | null
  document_number?: string | null
  updated_at?: string | null
  created_by?: string
  responsible?: ContractTemplateResponsible | null
}

export const useDcsDraftStore = defineStore(storeId, {
  state: (): TemplateDraftState => getInitialState(),
  getters: {
    hasTemplateId(): boolean {
      return !!this.did
    },
    blockIdsInOutline(): Set<string> {
      return collectBlockIdsInOutline(this.documentOutline)
    },
    /** Assembles minimal JSON-LD document from builder state — single source of truth for the stored format. */
    templateDocument(): DcsTemplateData {
      const blocks: DcsBlock[] = this.documentBlocks.map((b) => {
        if (isSectionBlock(b)) {
          return {
            '@type': 'dcs:Section' as const,
            '@id': blockIri(b.blockId),
            ...(b.title ? { 'dcs:title': b.title } : {}),
          }
        }
        if (isTextBlock(b)) {
          return { '@type': 'dcs:TextBlock' as const, '@id': blockIri(b.blockId), 'dcs:content': b.text }
        }
        if (isClauseBlock(b)) {
          return {
            '@type': 'dcs:Clause' as const,
            '@id': blockIri(b.blockId),
            'dcs:content': { '@list': clauseTextToSegments(b.text) },
            ...(b.title ? { 'dcs:title': b.title } : {}),
          }
        }
        if (isApprovedTemplateBlock(b)) {
          return {
            '@type': 'dcs:ApprovedTemplate' as const,
            '@id': blockIri(b.blockId),
            'dcs:templateDid': b.templateId,
            'dcs:version': b.version,
            ...(b.document_number ? { 'dcs:documentNumber': b.document_number } : {}),
          }
        }
        return { '@type': 'dcs:TextBlock' as const, '@id': blockIri(b.blockId), 'dcs:content': '' }
      })

      const layout: DcsLayoutNode[] = this.documentOutline.map((node) => ({
        '@id': blockIri(node.blockId),
        ...(node.isRoot ? { 'dcs:isRoot': true } : {}),
        'dcs:children': { '@list': node.children.map((id) => ({ '@id': blockIri(id) })) },
      }))

      const policyId = this.did ?? undefined

      return {
        '@context': DCS_JSONLD_CONTEXT,
        '@type': 'dcs:ContractTemplate',
        ...(this.did ? { '@id': this.did } : {}),
        'dcs:title': this.name,
        'dcs:templateType': this.templateType === TemplateType.frameContract ? 'dcs:FrameContract' : 'dcs:SubContract',
        'dcs:blocks': blocks,
        'dcs:layout': layout,
        'odrl:policy': semanticConditionsToOdrlPolicy(this.semanticConditions, policyId),
        'dcs:customMetaData': this.customMetaData,
        'dcs:subTemplateSnapshots': normalizeSubTemplateSnapshots(this.subTemplateSnapshots),
      }
    },
    templateCreateRequestData(): ContractTemplateCreateRequest {
      return {
        name: this.name,
        description: this.description,
        template_type: this.templateType,
        template_data: this.templateDocument,
      }
    },
    templateUpdateRequestData(): ContractTemplateUpdateRequest | null {
      if (!this.did || !this.updated_at) return null
      return {
        did: this.did,
        updated_at: this.updated_at,
        name: this.name,
        description: this.description,
        template_data: this.templateDocument,
      }
    },
  },
  actions: {
    /**
     * Loads a template document (JSON-LD or legacy format) plus DB-level metadata into store state.
     * Replaces the old pattern of calling reset({ documentOutline, documentBlocks, ... }).
     */
    loadDocument(rawDoc: unknown, meta: LoadDocumentMeta): void {
      const templateType = meta.templateType ?? TemplateType.subContract

      if (isDcsTemplateData(rawDoc)) {
        const jsonLdType = rawDoc['dcs:templateType']
        const derivedTemplateType: TemplateTypeValue =
          jsonLdType === 'dcs:FrameContract' ? TemplateType.frameContract : TemplateType.subContract

        const documentBlocks: DocumentBlock[] = (rawDoc['dcs:blocks'] ?? []).map(convertDcsBlockToBuilder)

        const rawLayout = rawDoc['dcs:layout'] ?? []
        const documentOutline: DocumentOutline =
          rawLayout.length > 0
            ? rawLayout.map((node) => ({
                blockId: blockIdFromIri(node['@id']),
                isRoot: node['dcs:isRoot'] ?? false,
                children: (node['dcs:children']['@list'] ?? []).map((ref) => blockIdFromIri(ref['@id'])),
              }))
            : getInitialOutline()

        this.reset({
          did: meta.did,
          name: meta.name || rawDoc['dcs:title'],
          description: meta.description,
          templateType: templateType !== TemplateType.subContract ? templateType : derivedTemplateType,
          state: meta.state ?? null,
          version: meta.version ?? null,
          document_number: meta.document_number ?? null,
          updated_at: meta.updated_at ?? null,
          created_by: meta.created_by ?? '',
          responsible: meta.responsible ?? null,
          documentOutline,
          documentBlocks,
          semanticConditions: odrlPolicyToSemanticConditions(rawDoc['odrl:policy']),
          customMetaData: (rawDoc['dcs:customMetaData'] as MetaData[]) ?? [],
          subTemplateSnapshots: (rawDoc['dcs:subTemplateSnapshots'] as SubTemplateSnapshot[]) ?? [],
        })
        return
      }

      if (isLegacyTemplateData(rawDoc)) {
        const rawOutline = rawDoc.documentOutline ?? []
        this.reset({
          did: meta.did,
          name: meta.name,
          description: meta.description,
          templateType,
          state: meta.state ?? null,
          version: meta.version ?? null,
          document_number: meta.document_number ?? null,
          updated_at: meta.updated_at ?? null,
          created_by: meta.created_by ?? '',
          responsible: meta.responsible ?? null,
          documentOutline: rawOutline.length > 0 ? rawOutline : getInitialOutline(),
          documentBlocks: rawDoc.documentBlocks ?? [],
          semanticConditions: rawDoc.semanticConditions ?? [],
          customMetaData: rawDoc.customMetaData ?? [],
          schemaRefs: rawDoc.schemaRefs ?? { ...defaultState.schemaRefs },
          policyRefs: rawDoc.policyRefs ?? defaultState.policyRefs.map((p) => ({ ...p })),
          validation: rawDoc.validation ?? { ...defaultState.validation },
          semanticProfile: rawDoc.semanticProfile ?? { ...defaultState.semanticProfile },
          templateVariables: rawDoc.templateVariables ?? [],
          placeholderBindings: rawDoc.placeholderBindings ?? [],
          semanticRules: rawDoc.semanticRules ?? [],
          policyBundle: rawDoc.policyBundle ?? null,
          sla: rawDoc.sla ?? null,
          subTemplateSnapshots: rawDoc.subTemplateSnapshots ?? [],
          templateDataVersion: rawDoc.templateDataVersion ?? 1,
        })
        return
      }

      // Unknown / empty format — start fresh
      this.reset({
        did: meta.did,
        name: meta.name,
        description: meta.description,
        templateType,
        state: meta.state ?? null,
        version: meta.version ?? null,
        document_number: meta.document_number ?? null,
        updated_at: meta.updated_at ?? null,
        created_by: meta.created_by ?? '',
        responsible: meta.responsible ?? null,
      })
    },
    addBlock(parentBlockId: string, insertIndex: number, payload: AddBlockPayload, options?: AddBlockOptions): string {
      if (this.workflow === 'template') {
        if (
          this.templateType === TemplateType.subContract &&
          payload.blockType === DocumentBlockType.ApprovedTemplate
        ) {
          throw new Error('subContract template cannot add APPROVED_TEMPLATE blocks')
        }
        if (
          this.templateType === TemplateType.frameContract &&
          payload.blockType !== DocumentBlockType.ApprovedTemplate
        ) {
          throw new Error('frameContract template can only add APPROVED_TEMPLATE blocks')
        }
      }
      return addBlock(this.documentOutline, this.documentBlocks, parentBlockId, insertIndex, payload, options)
    },
    deleteBlock(blockId: string): void {
      deleteBlock(this.documentOutline, this.documentBlocks, blockId)
    },
    updateBlock(blockId: string, payload: { title?: string; text?: string; conditionIds?: string[] }): void {
      for (const subTemplate of this.subTemplateSnapshots) {
        const td = subTemplate.template_data
        if (!td) continue
        const blocks = getDocumentBlocksFromTemplateData(td)
        const block = blocks.find((b) => b.blockId === blockId)
        if (!block || !isClauseBlock(block)) continue
        if (payload.title !== undefined) block.title = payload.title
        if (payload.text !== undefined) block.text = payload.text
        block.conditionIds = payload.conditionIds ?? []
      }

      const block = this.documentBlocks.find((b) => b.blockId === blockId)
      if (!block) return
      if (payload.title !== undefined) block.title = payload.title
      if (payload.text !== undefined) block.text = payload.text
      if (isClauseBlock(block)) block.conditionIds = payload.conditionIds ?? []
      for (const b of this.documentBlocks) if (isClauseBlock(b)) b.conditionIds = b.conditionIds ?? []
    },
    moveBlock(blockId: string, parentBlockId: string, insertIndex: number): void {
      moveBlock(this.documentOutline, blockId, parentBlockId, insertIndex)
    },
    addSemanticCondition(payload: Omit<SemanticCondition, 'conditionId'>): void {
      this.semanticConditions.push({
        ...payload,
        conditionId: crypto.randomUUID(),
      })
    },
    updateSemanticCondition(
      conditionId: string,
      payload: Omit<SemanticCondition, 'conditionId'>,
      subTemplateRef?: SubTemplateReference,
    ): void {
      const conditions = subTemplateRef
        ? getSemanticConditionsFromTemplateData(
            findSubTemplateSnapshotByRef(this.subTemplateSnapshots, subTemplateRef)?.template_data,
          )
        : this.semanticConditions
      const idx = conditions.findIndex((item) => item.conditionId === conditionId)
      if (idx < 0) return
      conditions[idx] = { ...payload, conditionId }
    },
    deleteSemanticCondition(conditionId: string, subTemplateRef?: SubTemplateReference): void {
      const blocks = subTemplateRef
        ? getDocumentBlocksFromTemplateData(
            findSubTemplateSnapshotByRef(this.subTemplateSnapshots, subTemplateRef)?.template_data,
          )
        : this.documentBlocks
      const conditions = subTemplateRef
        ? getSemanticConditionsFromTemplateData(
            findSubTemplateSnapshotByRef(this.subTemplateSnapshots, subTemplateRef)?.template_data,
          )
        : this.semanticConditions

      const placeholderRegex = placeholderRegexForCondition(conditionId)
      for (const block of blocks) {
        if (!isClauseBlock(block)) continue
        const hadCondition = block.conditionIds.includes(conditionId)
        block.conditionIds = block.conditionIds.filter((id) => id !== conditionId)
        if (hadCondition) block.text = block.text.replace(placeholderRegex, '')
      }

      const filteredConditions = conditions.filter((c) => c.conditionId !== conditionId)
      if (!subTemplateRef) {
        this.semanticConditions = filteredConditions
        return
      }
      const snapshot = findSubTemplateSnapshotByRef(this.subTemplateSnapshots, subTemplateRef)
      if (!snapshot?.template_data) return
      setSemanticConditionsOnTemplateData(snapshot.template_data, filteredConditions)
    },
    addClause(payload: {
      title?: string
      text: string
      conditionIds: string[]
      schemaRef?: string
      semanticPath?: string
    }): string {
      const blockId = crypto.randomUUID()
      const block = createBlockFromPayload(blockId, {
        blockType: DocumentBlockType.Clause,
        text: payload.text,
        title: payload.title,
        conditionIds: payload.conditionIds,
        schemaRef: payload.schemaRef,
        semanticPath: payload.semanticPath,
      })
      this.documentBlocks.push(block)
      return blockId
    },
    deleteClause(blockId: string): void {
      this.documentBlocks = removeClauseAndOutlineRefs(this.documentBlocks, this.documentOutline, blockId)
      for (const subTemplate of this.subTemplateSnapshots) {
        if (!subTemplate.template_data) continue
        const blocks = getDocumentBlocksFromTemplateData(subTemplate.template_data)
        const outlineBlocks = getOutlineFromTemplateData(subTemplate.template_data)
        const filtered = removeClauseAndOutlineRefs(blocks, outlineBlocks, blockId)
        setDocumentBlocksOnTemplateData(subTemplate.template_data, filtered)
      }
    },
    updateClause(blockId: string, payload: { title?: string; text?: string; conditionIds?: string[] }): void {
      this.updateBlock(blockId, payload)
    },
    addMetaData(payload: MetaData): boolean {
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m) => m.name.trim().toLowerCase() === lower)
      if (hasDuplicate) return false
      this.customMetaData.push({ name, value })
      return true
    },
    deleteMetaData(index: number): void {
      if (index < 0 || index >= this.customMetaData.length) return
      this.customMetaData.splice(index, 1)
    },
    updateMetaData(index: number, payload: MetaData): boolean {
      if (index < 0 || index >= this.customMetaData.length) return false
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m, idx) => {
        if (idx === index) return false
        return m.name.trim().toLowerCase() === lower
      })
      if (hasDuplicate) return false
      this.customMetaData[index] = { name, value }
      return true
    },
    updateTemplateType(templateType: TemplateTypeValue): void {
      if (this.did !== null && this.did !== undefined) {
        throw new Error('Cannot change template type after template is created')
      }
      this.templateType = templateType
    },
    updateName(name: string): void {
      this.name = name
    },
    updateDescription(description: string): void {
      this.description = description
    },
    addSubTemplateSnapshot(template: ContractTemplate): void {
      const snapshot: SubTemplateSnapshot = {
        did: template.did,
        version: template.version,
        document_number: template.document_number,
        name: template.name,
        description: template.description,
        template_data: template.template_data,
      }
      this.subTemplateSnapshots = [
        ...this.subTemplateSnapshots.filter((item) => !isSameTemplate(item, snapshot)),
        snapshot,
      ]
    },
    removeSubTemplateSnapshot(template: { did: string; version: number; document_number?: string }): void {
      this.subTemplateSnapshots = this.subTemplateSnapshots.filter((item) => !isSameTemplate(item, template))
    },
    reset(overrides?: Partial<TemplateDraftState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

// ---- JSON-LD URN helpers ----

const UUID_URN_PREFIX = 'urn:uuid:'

function blockIri(id: string): string {
  return `${UUID_URN_PREFIX}${id}`
}

function conditionIri(id: string): string {
  return `${UUID_URN_PREFIX}${id}`
}

function blockIdFromIri(iri: string): string {
  if (iri.startsWith(UUID_URN_PREFIX)) return iri.slice(UUID_URN_PREFIX.length)
  if (iri.startsWith('urn:block:')) return iri.slice('urn:block:'.length) // backwards compat
  if (iri.startsWith('dcs:block:')) return iri.slice('dcs:block:'.length) // backwards compat
  return iri
}

function conditionIdFromIri(iri: string): string {
  if (iri.startsWith(UUID_URN_PREFIX)) return iri.slice(UUID_URN_PREFIX.length)
  if (iri.startsWith('urn:condition:')) return iri.slice('urn:condition:'.length) // backwards compat
  if (iri.startsWith('dcs:condition:')) return iri.slice('dcs:condition:'.length) // backwards compat
  return iri
}

// ---- JSON-LD ↔ Builder model converters ----

function convertDcsBlockToBuilder(block: DcsBlock): DocumentBlock {
  switch (block['@type']) {
    case 'dcs:Section':
      return {
        blockId: blockIdFromIri(block['@id']),
        type: DocumentBlockType.Section,
        text: '',
        title: block['dcs:title'],
      }
    case 'dcs:TextBlock':
      return { blockId: blockIdFromIri(block['@id']), type: DocumentBlockType.Text, text: block['dcs:content'] }
    case 'dcs:Clause':
      return {
        blockId: blockIdFromIri(block['@id']),
        type: DocumentBlockType.Clause,
        text: segmentsToClauseText(block['dcs:content']),
        title: block['dcs:title'],
        conditionIds: extractConditionIdsFromSegments(block['dcs:content']),
      }
    case 'dcs:ApprovedTemplate':
      return {
        blockId: blockIdFromIri(block['@id']),
        type: DocumentBlockType.ApprovedTemplate,
        text: '',
        templateId: block['dcs:templateDid'],
        version: block['dcs:version'],
        document_number: block['dcs:documentNumber'] ?? '',
      }
  }
}

function clauseTextToSegments(text: string): DcsContentSegment[] {
  const segments: DcsContentSegment[] = []
  const regex = /\{\{([^.}]+)\.([^}]+)\}\}/g
  let lastIndex = 0
  let match
  while ((match = regex.exec(text)) !== null) {
    if (match.index > lastIndex) segments.push(text.slice(lastIndex, match.index))
    segments.push({
      '@type': 'dcs:ParameterRef',
      'dcs:constraint': { '@id': conditionIri(match[1]) },
      'odrl:leftOperand': { '@id': `dcs:${match[2]}` },
    } satisfies DcsParameterRef)
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < text.length) segments.push(text.slice(lastIndex))
  return segments
}

type RawClauseContent = { '@list': DcsContentSegment[] } | string

function segmentsToClauseText(content: RawClauseContent): string {
  if (typeof content === 'string') return content
  return content['@list']
    .map((seg) => {
      if (typeof seg === 'string') return seg
      const conditionId = conditionIdFromIri(seg['dcs:constraint']['@id'])
      const leftOperand = seg['odrl:leftOperand']['@id']
      const semanticPath = leftOperand.startsWith('dcs:') ? leftOperand.slice(4) : leftOperand
      return `{{${conditionId}.${semanticPath}}}`
    })
    .join('')
}

function extractConditionIdsFromSegments(content: RawClauseContent): string[] {
  if (typeof content === 'string') {
    const ids: string[] = []
    const regex = /\{\{([^.}]+)\.[^}]*\}\}/g
    let match
    while ((match = regex.exec(content)) !== null) {
      if (!ids.includes(match[1])) ids.push(match[1])
    }
    return ids
  }
  const ids: string[] = []
  for (const seg of content['@list']) {
    if (typeof seg !== 'string') {
      const id = conditionIdFromIri(seg['dcs:constraint']['@id'])
      if (!ids.includes(id)) ids.push(id)
    }
  }
  return ids
}

// ---- Legacy format helpers ----

interface LegacyTemplateData {
  documentOutline?: DocumentOutline
  documentBlocks?: DocumentBlock[]
  semanticConditions?: SemanticCondition[]
  customMetaData?: MetaData[]
  schemaRefs?: TemplateDraftState['schemaRefs']
  policyRefs?: TemplateDraftState['policyRefs']
  validation?: TemplateDraftState['validation']
  semanticProfile?: TemplateDraftState['semanticProfile']
  templateVariables?: TemplateDraftState['templateVariables']
  placeholderBindings?: TemplateDraftState['placeholderBindings']
  semanticRules?: TemplateDraftState['semanticRules']
  policyBundle?: TemplateDraftState['policyBundle']
  sla?: TemplateDraftState['sla']
  subTemplateSnapshots?: SubTemplateSnapshot[]
  templateDataVersion?: TemplateDraftState['templateDataVersion']
}

function isLegacyTemplateData(raw: unknown): raw is LegacyTemplateData {
  return typeof raw === 'object' && raw !== null && ('documentOutline' in raw || 'documentBlocks' in raw)
}

// ---- Sub-template data accessors (handle both JSON-LD and legacy) ----

function getDocumentBlocksFromTemplateData(td: SubTemplateSnapshot['template_data']): DocumentBlock[] {
  if (!td) return []
  if (isDcsTemplateData(td)) return (td['dcs:blocks'] ?? []).map(convertDcsBlockToBuilder)
  return (td as LegacyTemplateData).documentBlocks ?? []
}

function setDocumentBlocksOnTemplateData(td: SubTemplateSnapshot['template_data'], blocks: DocumentBlock[]): void {
  if (!td) return
  if (isDcsTemplateData(td)) return // JSON-LD snapshots are read-only; skip write-back
  ;(td as LegacyTemplateData).documentBlocks = blocks
}

function getOutlineFromTemplateData(td: SubTemplateSnapshot['template_data']): DocumentOutlineBlock[] {
  if (!td) return []
  if (isDcsTemplateData(td)) {
    return (td['dcs:layout'] ?? []).map((node) => ({
      blockId: blockIdFromIri(node['@id']),
      isRoot: node['dcs:isRoot'] ?? false,
      children: (node['dcs:children']['@list'] ?? []).map((ref) => blockIdFromIri(ref['@id'])),
    }))
  }
  return (td as LegacyTemplateData).documentOutline ?? []
}

function getSemanticConditionsFromTemplateData(td: SubTemplateSnapshot['template_data']): SemanticCondition[] {
  if (!td) return []
  if (isDcsTemplateData(td)) return odrlPolicyToSemanticConditions(td['odrl:policy'])
  return (td as LegacyTemplateData).semanticConditions ?? []
}

function setSemanticConditionsOnTemplateData(
  td: SubTemplateSnapshot['template_data'],
  conditions: SemanticCondition[],
): void {
  if (!td) return
  if (isDcsTemplateData(td)) {
    const policyId = td['odrl:policy']?.['@id'] ?? td['@id']
    td['odrl:policy'] = semanticConditionsToOdrlPolicy(conditions, policyId)
    return
  }
  ;(td as LegacyTemplateData).semanticConditions = conditions
}

// ---- Outline helpers ----

function getInitialOutline(): DocumentOutlineBlock[] {
  return [createOutlineItem({ isRoot: true, children: [] })]
}

function createOutlineItem(
  overrides?: Partial<Pick<DocumentOutlineBlock, 'blockId' | 'isRoot' | 'children'>>,
): DocumentOutlineBlock {
  return {
    blockId: overrides?.blockId ?? crypto.randomUUID(),
    isRoot: overrides?.isRoot ?? false,
    children: overrides?.children ?? [],
  }
}

function collectBlockIdsInOutline(outline: DocumentOutline): Set<string> {
  const blockIds = new Set<string>()
  const blockMap = new Map(outline.map((b) => [b.blockId, b]))
  function visit(blockId: string) {
    if (blockIds.has(blockId)) return
    blockIds.add(blockId)
    const block = blockMap.get(blockId)
    if (block) block.children.forEach(visit)
  }
  const root = outline.find((b) => b.isRoot)
  if (root) visit(root.blockId)
  return blockIds
}

// ---- Block mutation helpers ----

function addBlock(
  outline: DocumentOutlineBlock[],
  blocks: DocumentBlock[],
  parentBlockId: string,
  insertIndex: number,
  payload: AddBlockPayload,
  options?: AddBlockOptions,
): string {
  const addToOutline = options?.addToOutline !== false

  if (payload.clauseBlockId) {
    const clauseBlockId = payload.clauseBlockId
    const block = blocks.find((b) => b.blockId === clauseBlockId)
    if (!block || !isClauseBlock(block)) {
      throw new Error(`addBlock: clause block not found: ${clauseBlockId}`)
    }
    if (isClauseBlock(block)) block.conditionIds = block.conditionIds ?? []
    const inOutline = collectBlockIdsInOutline(outline)
    if (inOutline.has(clauseBlockId)) return clauseBlockId
    const parent = outline.find((b) => b.blockId === parentBlockId)
    if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
    parent.children.splice(insertIndex, 0, clauseBlockId)
    return clauseBlockId
  }

  const blockId = crypto.randomUUID()
  const block = createBlockFromPayload(blockId, payload)
  if (isClauseBlock(block)) block.conditionIds = block.conditionIds ?? []

  if (isClauseBlock(block) && !addToOutline) {
    blocks.push(block)
    return blockId
  }

  const parent = outline.find((b) => b.blockId === parentBlockId)
  if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
  parent.children.splice(insertIndex, 0, blockId)
  if (isSectionBlock(block) || isApprovedTemplateBlock(block)) {
    outline.push(createOutlineItem({ blockId, isRoot: false, children: [] }))
  }
  blocks.push(block)
  return blockId
}

function moveBlock(outline: DocumentOutlineBlock[], blockId: string, parentBlockId: string, insertIndex: number): void {
  const oldParent = outline.find((b) => b.children.includes(blockId))
  const newParent = outline.find((b) => b.blockId === parentBlockId)
  if (!oldParent || !newParent) return

  if (oldParent.blockId === newParent.blockId) {
    const siblings = oldParent.children.filter((id) => id !== blockId)
    const idx = Math.min(insertIndex, siblings.length)
    siblings.splice(idx, 0, blockId)
    oldParent.children = siblings
    return
  }
  oldParent.children = oldParent.children.filter((id) => id !== blockId)
  const newSiblings = [...newParent.children]
  const idx = Math.min(insertIndex, newSiblings.length)
  newSiblings.splice(idx, 0, blockId)
  newParent.children = newSiblings
}

function deleteBlock(outline: DocumentOutlineBlock[], blocks: DocumentBlock[], blockId: string): void {
  const block = blocks.find((b) => b.blockId === blockId)
  const parent = outline.find((b) => b.children.includes(blockId))
  if (!parent) return
  if (block && isClauseBlock(block)) {
    parent.children = parent.children.filter((id) => id !== blockId)
    return
  }
  const outlineByBlockId = new Map(outline.map((b) => [b.blockId, b]))
  const toRemove = collectDescendantBlockIds(blockId, outlineByBlockId)
  parent.children = parent.children.filter((id) => id !== blockId)
  const outlineToKeep = outline.filter((b) => b.isRoot === true || !toRemove.has(b.blockId))
  outline.length = 0
  outline.push(...outlineToKeep)
  const blocksToKeep = blocks.filter((b) => !toRemove.has(b.blockId))
  blocks.length = 0
  blocks.push(...blocksToKeep)
}

function createBlockFromPayload(blockId: string, payload: AddBlockPayload): DocumentBlock {
  const text = payload.text ?? ''
  const blockMeta = {
    blockCatalogueId: payload.blockCatalogueId,
    schemaRef: payload.schemaRef,
    semanticPath: payload.semanticPath,
  }
  switch (payload.blockType) {
    case DocumentBlockType.Section:
      return { blockId, type: DocumentBlockType.Section, text, ...blockMeta }
    case DocumentBlockType.Text:
      return { blockId, type: DocumentBlockType.Text, text, ...blockMeta }
    case DocumentBlockType.Clause:
      return {
        blockId,
        type: DocumentBlockType.Clause,
        text,
        title: payload.title,
        conditionIds: payload.conditionIds ?? [],
        ...blockMeta,
      }
    case DocumentBlockType.ApprovedTemplate:
      return {
        blockId,
        type: DocumentBlockType.ApprovedTemplate,
        text,
        ...blockMeta,
        templateId: payload.templateId ?? '',
        version: payload.version ?? 1,
        document_number: payload.document_number ?? '',
      }
    default:
      throw new Error(`Unknown blockType: ${payload.blockType}`)
  }
}

function removeClauseAndOutlineRefs(
  blocks: DocumentBlock[],
  outlineBlocks: DocumentOutlineBlock[],
  blockId: string,
): DocumentBlock[] {
  outlineBlocks.forEach((ob) => {
    if (!ob.children.includes(blockId)) return
    ob.children = ob.children.filter((id) => id !== blockId)
  })
  return blocks.filter((b) => b.blockId !== blockId)
}

function collectDescendantBlockIds(blockId: string, outlineByBlockId: Map<string, DocumentOutlineBlock>): Set<string> {
  const set = new Set<string>([blockId])
  const node = outlineByBlockId.get(blockId)
  const childIds = node?.children ?? []
  childIds.forEach((id) => collectDescendantBlockIds(id, outlineByBlockId).forEach((x) => set.add(x)))
  return set
}

function placeholderRegexForCondition(conditionId: string): RegExp {
  return new RegExp(`\\{\\{${conditionId}\\.([^}]*)\\}\\}`, 'g')
}

function getInitialState(): TemplateDraftState {
  return {
    ...defaultState,
    documentOutline: getInitialOutline(),
    documentBlocks: [...defaultState.documentBlocks],
    semanticConditions: [...defaultState.semanticConditions],
    customMetaData: [...defaultState.customMetaData],
    schemaRefs: { ...defaultState.schemaRefs },
    policyRefs: defaultState.policyRefs.map((policy) => ({ ...policy })),
    validation: {
      ...defaultState.validation,
      requiredPolicies: [...defaultState.validation.requiredPolicies],
    },
    semanticProfile: { ...defaultState.semanticProfile },
    templateVariables: [...defaultState.templateVariables],
    placeholderBindings: [...defaultState.placeholderBindings],
    semanticRules: [...defaultState.semanticRules],
    policyBundle: defaultState.policyBundle,
    sla: defaultState.sla,
    subTemplateSnapshots: [...defaultState.subTemplateSnapshots],
  }
}

function isSameTemplate(
  t1: { did: string; version: number; document_number?: string },
  t2: { did: string; version: number; document_number?: string },
): boolean {
  return isSameTemplateDataRef(
    { templateId: t1.did, version: t1.version, document_number: t1.document_number },
    { templateId: t2.did, version: t2.version, document_number: t2.document_number },
  )
}

function findSubTemplateSnapshotByRef(
  subTemplates: SubTemplateSnapshot[],
  subTemplateRef: SubTemplateReference,
): SubTemplateSnapshot | undefined {
  return subTemplates.find((subTemplate) => isSameTemplate(subTemplate, subTemplateRef))
}

function normalizeSubTemplateSnapshots(snapshots: SubTemplateSnapshot[]): SubTemplateSnapshot[] {
  return snapshots.map((snapshot) => {
    if (!snapshot.template_data) return snapshot
    if (isDcsTemplateData(snapshot.template_data)) return snapshot
    const td = snapshot.template_data as LegacyTemplateData
    return {
      ...snapshot,
      template_data: {
        documentOutline: td.documentOutline,
        semanticConditions: td.semanticConditions,
        documentBlocks: td.documentBlocks,
        customMetaData: td.customMetaData,
        schemaRefs: td.schemaRefs,
        policyRefs: td.policyRefs,
        validation: td.validation,
        semanticProfile: td.semanticProfile,
        templateVariables: td.templateVariables,
        placeholderBindings: td.placeholderBindings,
        semanticRules: td.semanticRules,
        policyBundle: td.policyBundle,
        sla: td.sla,
      },
    }
  })
}

// ---- SemanticCondition ↔ ODRL policy converters ----

function semanticConditionsToOdrlPolicy(conditions: SemanticCondition[], policyId: string): OdrlSet | undefined {
  if (conditions.length === 0) return undefined

  const obligations: OdrlDuty[] = conditions.map((condition) => {
    const constraints = condition.parameters
      .map((param) => buildOdrlConstraint(param))
      .filter((constraint): constraint is OdrlConstraint => !!constraint)

    return {
      '@type': 'odrl:Duty',
      '@id': `urn:condition:${condition.conditionId}`,
      'odrl:action': { '@id': 'dcs:ProvideParameter' },
      ...(constraints.length > 0 ? { 'odrl:constraint': constraints } : {}),
      'dcs:conditionName': condition.conditionName,
      'dcs:schemaVersion': condition.schemaVersion,
      ...(condition.entityType ? { 'dcs:entityType': condition.entityType } : {}),
      ...(condition.entityRole ? { 'dcs:entityRole': condition.entityRole } : {}),
    }
  })

  return {
    '@type': 'odrl:Set',
    ...(policyId ? { '@id': policyId } : {}),
    'odrl:obligation': obligations,
  }
}

function buildOdrlConstraint(param: SemanticConditionParameter): OdrlConstraint | undefined {
  const primaryOperator = param.operators[0]
  if (!primaryOperator || !isStandardOdrlOperator(primaryOperator.operate)) return undefined
  const rightOperand = odrlRightOperand(primaryOperator)
  return {
    '@type': 'odrl:Constraint',
    'odrl:leftOperand': { '@id': `dcs:${param.semanticPath}` },
    'odrl:operator': { '@id': primaryOperator.operate },
    ...(rightOperand !== undefined ? { 'odrl:rightOperand': rightOperand } : {}),
  }
}

function odrlRightOperand(operator: SemanticParameterOperator): unknown {
  if (!operator.targets.length) return undefined
  if (isSetOperator(operator.operate)) return operator.targets
  return operator.targets[0]
}

function isStandardOdrlOperator(operator: string): operator is DcsOperator {
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
  ].includes(operator)
}

function isSetOperator(operator: string): boolean {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf'
}

function odrlPolicyToSemanticConditions(policy: OdrlSet | undefined): SemanticCondition[] {
  if (!policy) return []
  return (policy['odrl:obligation'] ?? [])
    .filter((duty) => typeof duty['@id'] === 'string')
    .map((duty) => ({
      conditionId: conditionIdFromIri(duty['@id']!),
      conditionName: duty['dcs:conditionName'] ?? '',
      schemaVersion: (duty['dcs:schemaVersion'] ?? 'v1') as 'v1',
      entityType: duty['dcs:entityType'],
      entityRole: duty['dcs:entityRole'],
      parameters: (duty['odrl:constraint'] ?? []).map(odrlConstraintToParameter),
    }))
}

function odrlConstraintToParameter(constraint: OdrlConstraint): SemanticConditionParameter {
  const operate = constraint['odrl:operator']['@id'] as DcsOperator
  const leftOperandId = constraint['odrl:leftOperand']['@id']
  const semanticPath = leftOperandId.startsWith('dcs:') ? leftOperandId.slice(4) : leftOperandId
  const parameterName = semanticPath.split('.').pop() ?? semanticPath
  const rightOperand = constraint['odrl:rightOperand']
  const targets =
    rightOperand === undefined
      ? []
      : isSetOperator(operate) && Array.isArray(rightOperand)
        ? rightOperand
        : [rightOperand]
  return {
    parameterName,
    type: 'string',
    schemaRef: '',
    semanticPath,
    isRequired: false,
    operators: isStandardOdrlOperator(operate) ? [{ operate, targets }] : [],
    value: rightOperand ?? null,
  }
}
