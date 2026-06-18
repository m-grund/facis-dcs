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
  type DcsDataRequirement,
  type DcsRequirementField,
  type DcsSubTemplateSnapshot,
  type JsonLdTypedValue,
  type OdrlRule,
} from '@/models/dcs-jsonld'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { ContractTemplateResponsible } from '@/models/contract-template-responsible'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'

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
      const documentID = this.did ?? undefined
      const fieldIds = requirementFieldIds(this.semanticConditions, documentID)
      const blocks: DcsBlock[] = this.documentBlocks.map((b) => {
        if (isSectionBlock(b)) {
          return {
            '@type': 'dcs:Section' as const,
            '@id': blockIri(b.blockId, documentID),
            ...(b.title ? { 'dcs:title': b.title } : {}),
          }
        }
        if (isTextBlock(b)) {
          return { '@type': 'dcs:TextBlock' as const, '@id': blockIri(b.blockId, documentID), 'dcs:text': b.text }
        }
        if (isClauseBlock(b)) {
          return {
            '@type': 'dcs:Clause' as const,
            '@id': blockIri(b.blockId, documentID),
            'dcs:content': { '@list': clauseTextToSegments(b.text, fieldIds) },
            ...(b.title ? { 'dcs:title': b.title } : {}),
          }
        }
        if (isApprovedTemplateBlock(b)) {
          return {
            '@type': 'dcs:ApprovedTemplate' as const,
            '@id': blockIri(b.blockId, documentID),
            'dcs:templateDid': b.templateId,
            'dcs:version': b.version,
            ...(b.document_number ? { 'dcs:documentNumber': b.document_number } : {}),
          }
        }
        return { '@type': 'dcs:TextBlock' as const, '@id': blockIri(b.blockId, documentID), 'dcs:text': '' }
      })

      const layout: DcsLayoutNode[] = this.documentOutline.map((node) => ({
        '@id': blockIri(node.blockId, documentID),
        ...(node.isRoot ? { 'dcs:isRoot': true } : {}),
        'dcs:children': { '@list': node.children.map((id) => ({ '@id': blockIri(id, documentID) })) },
      }))

      return {
        '@context': DCS_JSONLD_CONTEXT,
        '@type': 'dcs:ContractTemplate',
        ...(this.did ? { '@id': this.did } : {}),
        'dcs:metadata': {
          '@type': 'dcs:TemplateMetadata',
          ...(this.did ? { '@id': `${this.did}#metadata` } : {}),
          'dcs:title': this.name,
          ...(this.description ? { 'dcs:description': this.description } : {}),
          'dcs:templateType':
            this.templateType === TemplateType.frameContract ? 'dcs:FrameContract' : 'dcs:SubContract',
          ...(this.customMetaData.length ? { 'dcs:customMetaData': this.customMetaData } : {}),
          ...(this.subTemplateSnapshots.length
            ? { 'dcs:subTemplates': serializeSubTemplateSnapshots(this.subTemplateSnapshots) }
            : {}),
        },
        'dcs:documentStructure': {
          '@type': 'dcs:DocumentStructure',
          ...(this.did ? { '@id': `${this.did}#document-structure` } : {}),
          'dcs:blocks': blocks,
          'dcs:layout': layout,
        },
        'dcs:contractData': semanticConditionsToContractData(this.semanticConditions, documentID),
        'dcs:policies': semanticConditionsToPolicies(this.semanticConditions, fieldIds, documentID),
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
    /** Loads the canonical JSON-LD envelope plus DB-level metadata into store state. */
    loadDocument(rawDoc: unknown, meta: LoadDocumentMeta): void {
      const templateType = meta.templateType ?? TemplateType.subContract

      if (isDcsTemplateData(rawDoc)) {
        const metadata = rawDoc['dcs:metadata']
        const jsonLdType = metadata['dcs:templateType']
        const derivedTemplateType: TemplateTypeValue =
          jsonLdType === 'dcs:FrameContract' ? TemplateType.frameContract : TemplateType.subContract
        const builderData = templateDataToBuilderData(rawDoc)

        this.reset({
          did: meta.did,
          name: meta.name ? meta.name : (metadata['dcs:title'] ?? ''),
          description: meta.description ? meta.description : (metadata['dcs:description'] ?? ''),
          templateType: templateType !== TemplateType.subContract ? templateType : derivedTemplateType,
          state: meta.state ?? null,
          version: meta.version ?? null,
          document_number: meta.document_number ?? null,
          updated_at: meta.updated_at ?? null,
          created_by: meta.created_by ?? '',
          responsible: meta.responsible ?? null,
          ...builderData,
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

function objectIri(kind: string, id: string, documentId?: string): string {
  const fragment = `${kind}-${encodeURIComponent(id)}`
  return documentId ? `${documentId}#${fragment}` : `${UUID_URN_PREFIX}${fragment}`
}

function blockIri(id: string, documentId?: string): string {
  return objectIri('block', id, documentId)
}

function conditionIri(id: string, documentId?: string): string {
  return objectIri('requirement', id, documentId)
}

function fieldIri(conditionId: string, parameterName: string, documentId?: string): string {
  return objectIri('field', `${conditionId}-${parameterName}`, documentId)
}

function policyIri(conditionId: string, parameterName: string, index: number, documentId?: string): string {
  return objectIri('policy', `${conditionId}-${parameterName}-${index}`, documentId)
}

function blockIdFromIri(iri: string): string {
  const local = iri.includes('#') ? iri.slice(iri.lastIndexOf('#') + 1) : iri.slice(UUID_URN_PREFIX.length)
  return decodeURIComponent(local.replace(/^block-/, ''))
}

// ---- JSON-LD ↔ Builder model converters ----

function convertDcsBlockToBuilder(
  block: DcsBlock,
  fieldsById: ReadonlyMap<string, { conditionId: string }>,
): DocumentBlock {
  switch (block['@type']) {
    case 'dcs:Section':
      return {
        blockId: blockIdFromIri(block['@id']),
        type: DocumentBlockType.Section,
        text: '',
        title: block['dcs:title'],
      }
    case 'dcs:TextBlock':
      return { blockId: blockIdFromIri(block['@id']), type: DocumentBlockType.Text, text: block['dcs:text'] }
    case 'dcs:Clause':
      return {
        blockId: blockIdFromIri(block['@id']),
        type: DocumentBlockType.Clause,
        text: segmentsToClauseText(block['dcs:content']),
        title: block['dcs:title'],
        conditionIds: extractConditionIdsFromSegments(block['dcs:content'], fieldsById),
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

function clauseTextToSegments(text: string, fieldIds: ReadonlyMap<string, string>): DcsContentSegment[] {
  const segments: DcsContentSegment[] = []
  const regex = /\{\{([^.}]+)\.([^}]+)\}\}/g
  let lastIndex = 0
  let match
  while ((match = regex.exec(text)) !== null) {
    if (match.index > lastIndex) segments.push(text.slice(lastIndex, match.index))
    const key = parameterKey(match[1] ?? '', match[2] ?? '')
    const bindsTo = fieldIds.get(key)
    if (!bindsTo) {
      segments.push(match[0])
      lastIndex = match.index + match[0].length
      continue
    }
    segments.push({
      '@type': 'dcs:Placeholder',
      'dcs:token': match[0],
      'dcs:bindsTo': { '@id': bindsTo },
    })
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < text.length) segments.push(text.slice(lastIndex))
  return segments
}

type RawClauseContent = { '@list': DcsContentSegment[] } | string

function segmentsToClauseText(content: RawClauseContent): string {
  if (typeof content === 'string') return content
  return content['@list'].map((segment) => (typeof segment === 'string' ? segment : segment['dcs:token'])).join('')
}

function extractConditionIdsFromSegments(
  content: RawClauseContent,
  fieldsById: ReadonlyMap<string, { conditionId: string }>,
): string[] {
  if (typeof content === 'string') {
    const ids: string[] = []
    const regex = /\{\{([^.}]+)\.[^}]*\}\}/g
    let match
    while ((match = regex.exec(content)) !== null) {
      const conditionId = match[1] ?? ''
      if (conditionId && !ids.includes(conditionId)) ids.push(conditionId)
    }
    return ids
  }
  const ids: string[] = []
  for (const seg of content['@list']) {
    if (typeof seg !== 'string') {
      const field = fieldsById.get(seg['dcs:bindsTo']['@id'])
      if (field && !ids.includes(field.conditionId)) ids.push(field.conditionId)
    }
  }
  return ids
}

export interface TemplateBuilderData {
  documentOutline: DocumentOutline
  documentBlocks: DocumentBlock[]
  semanticConditions: SemanticCondition[]
  customMetaData: MetaData[]
  subTemplateSnapshots: SubTemplateSnapshot[]
}

export function templateDataToBuilderData(raw: unknown): TemplateBuilderData {
  if (!isDcsTemplateData(raw)) {
    return {
      documentOutline: getInitialOutline(),
      documentBlocks: [],
      semanticConditions: [],
      customMetaData: [],
      subTemplateSnapshots: [],
    }
  }

  const semanticConditions = contractDataToSemanticConditions(raw['dcs:contractData'], raw['dcs:policies'])
  const fieldsById = requirementFieldsById(raw['dcs:contractData'])
  const structure = raw['dcs:documentStructure']
  const documentBlocks = structure['dcs:blocks'].map((block) => convertDcsBlockToBuilder(block, fieldsById))
  const documentOutline = structure['dcs:layout'].map((node) => ({
    blockId: blockIdFromIri(node['@id']),
    isRoot: node['dcs:isRoot'] ?? false,
    children: node['dcs:children']['@list'].map((ref) => blockIdFromIri(ref['@id'])),
  }))
  const metadata = raw['dcs:metadata']

  return {
    documentOutline: documentOutline.length ? documentOutline : getInitialOutline(),
    documentBlocks,
    semanticConditions,
    customMetaData: (metadata['dcs:customMetaData'] as MetaData[]) ?? [],
    subTemplateSnapshots: deserializeSubTemplateSnapshots(metadata['dcs:subTemplates'] ?? []),
  }
}

export function getDocumentBlocksFromTemplateData(td: SubTemplateSnapshot['template_data']): DocumentBlock[] {
  return templateDataToBuilderData(td).documentBlocks
}

function setDocumentBlocksOnTemplateData(_td: SubTemplateSnapshot['template_data'], _blocks: DocumentBlock[]): void {
  // Sub-template snapshots are immutable canonical envelopes.
}

export function getOutlineFromTemplateData(td: SubTemplateSnapshot['template_data']): DocumentOutlineBlock[] {
  return templateDataToBuilderData(td).documentOutline
}

export function getSemanticConditionsFromTemplateData(td: SubTemplateSnapshot['template_data']): SemanticCondition[] {
  return templateDataToBuilderData(td).semanticConditions
}

function setSemanticConditionsOnTemplateData(
  _td: SubTemplateSnapshot['template_data'],
  _conditions: SemanticCondition[],
): void {
  // Sub-template snapshots are immutable canonical envelopes.
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

function serializeSubTemplateSnapshots(snapshots: SubTemplateSnapshot[]): DcsSubTemplateSnapshot[] {
  return snapshots.flatMap((snapshot) => {
    if (!isDcsTemplateData(snapshot.template_data)) return []
    return [
      {
        '@id': snapshot.did,
        'dcs:version': snapshot.version,
        ...(snapshot.document_number ? { 'dcs:documentNumber': snapshot.document_number } : {}),
        ...(snapshot.name ? { 'dcs:name': snapshot.name } : {}),
        ...(snapshot.description ? { 'dcs:description': snapshot.description } : {}),
        'dcs:template': snapshot.template_data,
      },
    ]
  })
}

function deserializeSubTemplateSnapshots(snapshots: DcsSubTemplateSnapshot[]): SubTemplateSnapshot[] {
  return snapshots.map((snapshot) => ({
    did: snapshot['@id'],
    version: snapshot['dcs:version'],
    document_number: snapshot['dcs:documentNumber'],
    name: snapshot['dcs:name'],
    description: snapshot['dcs:description'],
    template_data: snapshot['dcs:template'],
  }))
}

function parameterKey(conditionId: string, parameterName: string): string {
  return `${conditionId}\u0000${parameterName}`
}

function requirementFieldIds(
  conditions: readonly SemanticCondition[],
  documentId?: string,
): ReadonlyMap<string, string> {
  const result = new Map<string, string>()
  for (const condition of conditions) {
    for (const parameter of condition.parameters) {
      result.set(
        parameterKey(condition.conditionId, parameter.parameterName),
        fieldIri(condition.conditionId, parameter.parameterName, documentId),
      )
    }
  }
  return result
}

function semanticConditionsToContractData(
  conditions: readonly SemanticCondition[],
  documentId?: string,
): DcsDataRequirement[] {
  return conditions.map((condition) => ({
    '@id': conditionIri(condition.conditionId, documentId),
    '@type': 'dcs:DataRequirement',
    'dcs:conditionId': condition.conditionId,
    'dcs:name': condition.conditionName,
    'dcs:schemaVersion': condition.schemaVersion,
    ...(condition.entityType ? { 'dcs:entityType': condition.entityType } : {}),
    ...(condition.entityRole ? { 'dcs:entityRole': condition.entityRole } : {}),
    'dcs:fields': condition.parameters.map((parameter) => {
      const domainField = ONTOLOGY_DOMAIN_FIELDS.find((field) => field.semanticPath === parameter.semanticPath)
      return {
        '@id': fieldIri(condition.conditionId, parameter.parameterName, documentId),
        '@type': 'dcs:RequirementField',
        'dcs:parameterName': parameter.parameterName,
        'dcs:domainField': { '@id': domainField?.ontologyId ?? parameter.semanticPath },
        'dcs:semanticPath': parameter.semanticPath,
        'dcs:required': parameter.isRequired,
      }
    }),
  }))
}

function semanticConditionsToPolicies(
  conditions: readonly SemanticCondition[],
  fieldIds: ReadonlyMap<string, string>,
  documentId?: string,
): OdrlRule[] {
  return conditions.flatMap((condition) =>
    condition.parameters.flatMap((parameter) =>
      parameter.operators.flatMap((operator, index) => {
        if (!isStandardOdrlOperator(operator.operate)) return []
        const leftOperand = fieldIds.get(parameterKey(condition.conditionId, parameter.parameterName))
        if (!leftOperand) return []
        return [
          {
            '@id': policyIri(condition.conditionId, parameter.parameterName, index, documentId),
            '@type': 'odrl:Duty',
            'odrl:constraint': {
              '@type': 'odrl:Constraint',
              'odrl:leftOperand': { '@id': leftOperand },
              'odrl:operator': { '@id': operator.operate },
              ...(odrlRightOperand(operator, parameter.type) !== undefined
                ? { 'odrl:rightOperand': odrlRightOperand(operator, parameter.type) }
                : {}),
            },
          } satisfies OdrlRule,
        ]
      }),
    ),
  )
}

function odrlRightOperand(
  operator: SemanticParameterOperator,
  parameterType: SemanticConditionParameter['type'],
): JsonLdTypedValue | JsonLdTypedValue[] | undefined {
  if (!operator.targets.length) return undefined
  const operands = operator.targets.map((target) => typedJsonLdValue(target, parameterType))
  if (isSetOperator(operator.operate)) return operands
  return operands[0]
}

function typedJsonLdValue(value: unknown, parameterType: SemanticConditionParameter['type']): JsonLdTypedValue {
  return {
    '@value': jsonLdLexicalValue(value, parameterType),
    '@type': xsdTypeForParameter(parameterType),
  }
}

function jsonLdLexicalValue(value: unknown, parameterType: SemanticConditionParameter['type']): string {
  if (parameterType === 'boolean') return value === true || value === 'true' ? 'true' : 'false'
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'bigint') return value.toString()
  return JSON.stringify(value) ?? ''
}

function xsdTypeForParameter(parameterType: SemanticConditionParameter['type']): JsonLdTypedValue['@type'] {
  switch (parameterType) {
    case 'decimal':
      return 'xsd:decimal'
    case 'integer':
      return 'xsd:integer'
    case 'boolean':
      return 'xsd:boolean'
    case 'date':
      return 'xsd:date'
    case 'string':
    case 'enum':
      return 'xsd:string'
  }
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

function requirementFieldsById(
  requirements: readonly DcsDataRequirement[],
): ReadonlyMap<string, { conditionId: string; field: DcsRequirementField }> {
  const result = new Map<string, { conditionId: string; field: DcsRequirementField }>()
  for (const requirement of requirements) {
    for (const field of requirement['dcs:fields']) {
      result.set(field['@id'], { conditionId: requirement['dcs:conditionId'], field })
    }
  }
  return result
}

function contractDataToSemanticConditions(
  requirements: readonly DcsDataRequirement[],
  policies: readonly OdrlRule[],
): SemanticCondition[] {
  const operatorsByField = new Map<string, SemanticParameterOperator[]>()
  for (const policy of policies) {
    const constraint = policy['odrl:constraint']
    if (!constraint) continue
    const operate = constraint['odrl:operator']['@id'] as DcsOperator
    if (!isStandardOdrlOperator(operate)) continue
    const rightOperand = constraint['odrl:rightOperand']
    const targets =
      rightOperand === undefined
        ? []
        : Array.isArray(rightOperand)
          ? rightOperand.map(jsonLdValue)
          : [jsonLdValue(rightOperand)]
    const fieldId = constraint['odrl:leftOperand']['@id']
    operatorsByField.set(fieldId, [...(operatorsByField.get(fieldId) ?? []), { operate, targets }])
  }

  return requirements.map((requirement) => ({
    conditionId: requirement['dcs:conditionId'],
    conditionName: requirement['dcs:name'],
    schemaVersion: requirement['dcs:schemaVersion'],
    entityType: requirement['dcs:entityType'],
    entityRole: requirement['dcs:entityRole'],
    parameters: requirement['dcs:fields'].flatMap((field) => {
      const ontologyField = ONTOLOGY_DOMAIN_FIELDS.find(
        (candidate) =>
          candidate.ontologyId === field['dcs:domainField']['@id'] ||
          candidate.semanticPath === field['dcs:semanticPath'],
      )
      if (!ontologyField) return []
      return [
        {
          parameterName: field['dcs:parameterName'],
          type: ontologyField.type,
          schemaRef: ontologyField.schemaRef,
          semanticPath: ontologyField.semanticPath,
          valueConstraint: cloneValueConstraint(ontologyField.valueConstraint),
          uiMetadata: { label: ontologyField.label },
          isRequired: field['dcs:required'],
          operators: operatorsByField.get(field['@id']) ?? [],
          value: undefined,
        },
      ]
    }),
  }))
}

function jsonLdValue(value: JsonLdTypedValue): unknown {
  switch (value['@type']) {
    case 'xsd:decimal':
    case 'xsd:integer':
      return Number(value['@value'])
    case 'xsd:boolean':
      return value['@value'] === 'true'
    case 'xsd:string':
    case 'xsd:date':
      return value['@value']
  }
}

function cloneValueConstraint(
  constraint: SemanticConditionParameter['valueConstraint'],
): SemanticConditionParameter['valueConstraint'] {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions?.map((option) => ({ ...option })),
  }
}
