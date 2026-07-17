import { TemplateType } from '@template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import { DCS_ODRL_PROFILE_IRI, DEFAULT_FIELD_CONSTRAINT_ACTION } from '@template-repository/utils/sla-ontology-catalog'
import { isMergedBlockId, isSameTemplateDataRef } from '@template-repository/utils/template-data-ref'
import { typedClauseValuesSummary } from '@template-repository/utils/typed-clause'
import jsonld from 'jsonld'
import { defineStore } from 'pinia'
import {
  type DcsApprovedTemplate,
  type DcsBlock,
  type DcsClause,
  type DcsContentSegment,
  type DcsContractData,
  type DcsDataRequirement,
  type DcsDocumentData,
  type DcsDocumentStructure,
  type DcsLayoutNode,
  type DcsRequirementField,
  type DcsSection,
  type DcsSubTemplateSnapshot,
  type DcsTemplateData,
  type DcsTextBlock,
  isDcsClause,
  isDcsDocumentData,
  isDcsTemplateData,
  type JsonLdReference,
  type JsonLdTypedValue,
  type OdrlRule,
  type OdrlSet,
} from '@/models/dcs-jsonld'
import {
  applyInlineSemanticValues,
  applyInlineSemanticValuesToSnapshots,
} from '@/modules/contract-workflow-engine/utils/semantic-condition-values'
import { getHubContext } from '@/services/semantic-hub-service'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { ContractTemplate, SubTemplateSnapshot } from '@/models/contract-template'
import type { ContractTemplateResponsible } from '@/models/contract-template-responsible'
import type {
  ContractTemplateCreateRequest,
  ContractTemplateUpdateManageRequest,
  ContractTemplateUpdateRequest,
} from '@/models/requests/template-request'
import type { DcsOperator } from '@/models/semantic/facis-dcs-semantic'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type {
  MetaData,
  SemanticCondition,
  SemanticConditionParameter,
  SemanticParameterOperator,
  TemplateTypeValue,
} from '@template-repository/models/contract-template'
import type {
  AddBlockOptions,
  AddBlockPayload,
  SubTemplateReference,
  TemplateDraftState,
} from '@template-repository/models/template-draft-store'

// ---- MergedApprovedTemplateBlock (UI-only virtual block for composed contract templates) ----

export interface MergedApprovedTemplateBlock {
  '@type': 'dcs:MergedApprovedTemplate'
  '@id': string
  'dcs:templateDid': string
  'dcs:version': number
  'dcs:documentNumber': string
}

export function isDcsMergedApprovedTemplate(
  block: DcsBlock | MergedApprovedTemplateBlock,
): block is MergedApprovedTemplateBlock {
  return block['@type'] === 'dcs:MergedApprovedTemplate'
}

const storeId = 'dcsDraft'

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
      return collectBlockIdsInLayout(this.layout)
    },
    /** Enriched semantic conditions derived from stored JSON-LD contractData + policies. */
    semanticConditions(): SemanticCondition[] {
      return contractDataToSemanticConditions(this.contractData, this.policies)
    },
    /** Assembles the canonical JSON-LD document from store state — no conversion needed. */
    templateDocument(): DcsTemplateData {
      return assembleCanonicalDocument({
        documentType: 'dcs:ContractTemplate',
        documentId: this.documentIri ?? this.did ?? undefined,
        name: this.name,
        description: this.description,
        templateType: this.templateType,
        blocks: this.blocks,
        layout: this.layout,
        contractData: this.contractData,
        policies: this.policies,
        customMetaData: this.customMetaData,
        subTemplateSnapshots: this.subTemplateSnapshots,
      }) as DcsTemplateData
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
    templateUpdateManageRequestData(): ContractTemplateUpdateManageRequest | null {
      if (!this.did || !this.updated_at) return null
      return {
        did: this.did,
        state: this.state,
        updated_at: this.updated_at,
        document_number: this.document_number ?? undefined,
        template_type: this.templateType,
        name: this.name,
        description: this.description,
        template_data: this.templateDocument,
      }
    },
  },
  actions: {
    /** Loads the canonical JSON-LD envelope plus DB-level metadata into store state. */
    loadDocument(rawDoc: unknown, meta: LoadDocumentMeta): void {
      const templateType = meta.templateType ?? TemplateType.component

      if (isDcsTemplateData(rawDoc)) {
        const metadata = rawDoc['dcs:metadata']
        const jsonLdType = metadata['dcs:templateType']
        const derivedTemplateType: TemplateTypeValue =
          jsonLdType === 'dcs:ContractTemplate' ? TemplateType.contractTemplate : TemplateType.component
        const structure = rawDoc['dcs:documentStructure']

        this.reset({
          did: meta.did,
          documentIri: rawDoc['@id'] ?? null,
          name: meta.name ? meta.name : (metadata['dcs:title'] ?? ''),
          description: meta.description ? meta.description : (metadata['dcs:description'] ?? ''),
          templateType: templateType !== TemplateType.component ? templateType : derivedTemplateType,
          state: meta.state ?? undefined,
          version: meta.version ?? null,
          document_number: meta.document_number ?? null,
          updated_at: meta.updated_at ?? null,
          created_by: meta.created_by ?? '',
          responsible: meta.responsible ?? null,
          blocks: extractBlockList(structure['dcs:blocks']),
          layout: structure['dcs:layout'].length ? structure['dcs:layout'] : getInitialLayout(),
          contractData: rawDoc['dcs:contractData'],
          policies: flattenPolicySet(rawDoc['dcs:policies']),
          customMetaData: (metadata['dcs:customMetaData'] as MetaData[]) ?? [],
          subTemplateSnapshots: deserializeSubTemplateSnapshots(metadata['dcs:subTemplates'] ?? []),
        })
        return
      }

      // Unknown / empty format — start fresh
      this.reset({
        did: meta.did,
        name: meta.name,
        description: meta.description,
        templateType,
        state: meta.state ?? undefined,
        version: meta.version ?? null,
        document_number: meta.document_number ?? null,
        updated_at: meta.updated_at ?? null,
        created_by: meta.created_by ?? '',
        responsible: meta.responsible ?? null,
      })
    },
    addBlock(parentBlockId: string, insertIndex: number, payload: AddBlockPayload, options?: AddBlockOptions): string {
      if (this.workflow === 'template') {
        if (this.templateType === TemplateType.component && payload.blockType === 'dcs:ApprovedTemplate') {
          throw new Error('components cannot add ApprovedTemplate blocks')
        }
        if (this.templateType === TemplateType.contractTemplate && payload.blockType !== 'dcs:ApprovedTemplate') {
          throw new Error('contract templates can only add ApprovedTemplate blocks')
        }
      }
      return addBlock(this.layout, this.blocks, parentBlockId, insertIndex, payload, options, this.did ?? undefined)
    },
    deleteBlock(blockId: string): void {
      deleteBlock(this.layout, this.blocks, blockId)
    },
    updateBlock(
      blockId: string,
      payload: {
        title?: string
        text?: string
        content?: DcsContentSegment[]
        typedClause?: import('@/models/dcs-jsonld').DcsTypedClauseInstance
      },
    ): void {
      const block = this.blocks.find((b) => b['@id'] === blockId)
      if (!block) return
      if (isDcsClause(block as DcsBlock)) {
        const clause = block as DcsClause
        if (payload.title !== undefined) clause['dcs:title'] = payload.title || undefined
        if (payload.content !== undefined) clause['dcs:content'] = { '@list': payload.content }
        if (payload.typedClause !== undefined) clause['dcs:typedClause'] = payload.typedClause
      } else if ((block as DcsBlock)['@type'] === 'dcs:TextBlock') {
        const tb = block as DcsTextBlock
        if (payload.text !== undefined) tb['dcs:text'] = payload.text
      } else if ((block as DcsBlock)['@type'] === 'dcs:Section') {
        const sec = block as DcsSection
        if (payload.title !== undefined) sec['dcs:title'] = payload.title || undefined
      }
    },
    moveBlock(blockId: string, parentBlockId: string, insertIndex: number): void {
      moveBlock(this.layout, blockId, parentBlockId, insertIndex)
    },
    updateFieldPolicies(
      fieldId: string,
      conditionId: string,
      parameterName: string,
      parameterType: SemanticConditionParameter['type'],
      operators: SemanticParameterOperator[],
    ): void {
      const documentId = this.documentIri ?? this.did ?? undefined
      const requirement = requirementForField(this.contractData, fieldId)
      const role = requirement?.['dcs:entityRole']
      this.policies = this.policies.filter((p) => p['odrl:constraint']?.['odrl:leftOperand']['@id'] !== fieldId)
      operators.forEach((operator, index) => {
        if (!isStandardOdrlOperator(operator.operate)) return
        const rightOperand = odrlRightOperand(operator, parameterType)
        this.policies.push({
          '@id': policyIri(conditionId, parameterName, index, documentId),
          '@type': 'odrl:Duty',
          'odrl:action': { '@id': DEFAULT_FIELD_CONSTRAINT_ACTION },
          'odrl:assigner': partyReference(role, documentId),
          'odrl:assignee': partyReference(counterpartRole(role), documentId),
          'odrl:target': targetReference(documentId),
          'dcs:prose': proseBlockForField(this.blocks, fieldId),
          'odrl:constraint': {
            '@type': 'odrl:Constraint',
            'odrl:leftOperand': { '@id': fieldId },
            'odrl:operator': { '@id': operator.operate },
            ...(rightOperand !== undefined ? { 'odrl:rightOperand': rightOperand } : {}),
          },
        } satisfies OdrlRule)
      })
    },
    addSemanticCondition(payload: Omit<SemanticCondition, 'conditionId'>): void {
      const conditionId = crypto.randomUUID()
      const documentId = this.documentIri ?? this.did ?? undefined
      this.contractData.push({
        '@id': conditionIri(conditionId, documentId),
        '@type': 'dcs:DataRequirement',
        'dcs:conditionId': conditionId,
        'dcs:name': payload.conditionName,
        'dcs:schemaVersion': payload.schemaVersion,
        ...(payload.entityType ? { 'dcs:entityType': payload.entityType } : {}),
        ...(payload.entityRole ? { 'dcs:entityRole': payload.entityRole } : {}),
        'dcs:fields': payload.parameters.map((p) => semanticParamToField(conditionId, p, documentId)),
      })
      this.policies.push(...semanticConditionToPolicies({ ...payload, conditionId }, this.contractData, this.blocks, documentId))
    },
    updateSemanticCondition(
      conditionId: string,
      payload: Omit<SemanticCondition, 'conditionId'>,
      subTemplateRef?: SubTemplateReference,
    ): void {
      if (subTemplateRef) return // sub-template snapshots are immutable
      const documentId = this.documentIri ?? this.did ?? undefined
      const idx = this.contractData.findIndex((r) => r['dcs:conditionId'] === conditionId)
      const existing = this.contractData[idx]
      if (idx < 0 || !existing) return
      const oldFieldIds = new Set(existing['dcs:fields'].map((f) => f['@id']))
      this.contractData[idx] = {
        '@id': conditionIri(conditionId, documentId),
        '@type': 'dcs:DataRequirement',
        'dcs:conditionId': conditionId,
        'dcs:name': payload.conditionName,
        'dcs:schemaVersion': payload.schemaVersion,
        ...(payload.entityType ? { 'dcs:entityType': payload.entityType } : {}),
        ...(payload.entityRole ? { 'dcs:entityRole': payload.entityRole } : {}),
        'dcs:fields': payload.parameters.map((p) => semanticParamToField(conditionId, p, documentId)),
      }
      this.policies = this.policies.filter((p) => {
        const leftOp = p['odrl:constraint']?.['odrl:leftOperand']['@id']
        return !leftOp || !oldFieldIds.has(leftOp)
      })
      this.policies.push(...semanticConditionToPolicies({ ...payload, conditionId }, this.contractData, this.blocks, documentId))
    },
    deleteSemanticCondition(conditionId: string, subTemplateRef?: SubTemplateReference): void {
      if (subTemplateRef) return // sub-template snapshots are immutable
      const req = this.contractData.find((r) => r['dcs:conditionId'] === conditionId)
      if (!req) return
      const fieldIds = new Set(req['dcs:fields'].map((f) => f['@id']))

      // Remove placeholders from clause blocks
      for (const block of this.blocks) {
        if ((block as DcsBlock)['@type'] !== 'dcs:Clause') continue
        const clause = block as DcsClause
        const content = clause['dcs:content']
        if (typeof content === 'string') continue
        clause['dcs:content'] = {
          '@list': content['@list'].filter(
            (seg) => typeof seg === 'string' || !fieldIds.has(seg['dcs:bindsTo']['@id']),
          ),
        }
      }

      this.contractData = this.contractData.filter((r) => r['dcs:conditionId'] !== conditionId)
      this.policies = this.policies.filter((p) => {
        const leftOp = p['odrl:constraint']?.['odrl:leftOperand']['@id']
        return !leftOp || !fieldIds.has(leftOp)
      })
    },
    addClause(payload: {
      title?: string
      content: DcsContentSegment[]
    }): string {
      const blockId = crypto.randomUUID()
      const id = blockIri(blockId, this.did ?? undefined)
      const block: import('@/models/dcs-jsonld').DcsClause = {
        '@type': 'dcs:Clause',
        '@id': id,
        'dcs:content': { '@list': payload.content },
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
      }
      this.blocks.push(block)
      return id
    },
    /**
     * Phase 3 (DCS-FR-TR-03/TR-04, ADR-10): adds a typed clause instance
     * generated from the Semantic Hub's clause catalog (TypedClausePalette
     * .vue). Nested inside a plain dcs:Clause block (dcs:typedClause) so it
     * renders/places/persists through the existing free-text clause
     * machinery, while still becoming its own JSON-LD node server-side
     * validation (validateAgainstHubShapes) targets by its @type.
     */
    async addTypedClause(payload: { clauseType: string; title?: string; instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance }): Promise<string> {
      const blockId = crypto.randomUUID()
      const id = blockIri(blockId, this.did ?? undefined)
      const trimmedTitle = payload.title?.trim()
      const typeTail = payload.clauseType.split(/[#/:]/).pop()
      const title = trimmedTitle?.length ? trimmedTitle : typeTail?.length ? typeTail : payload.clauseType
      const instance = payload.instance

      const compacted = await compactAgainstHubContext(instance)
      if (isOdrlRuleInstance(compacted['@type'])) {
        // A hub-templated ODRL rule: the clause block is its human-readable
        // prose, the compacted rule joins dcs:policies backed by it.
        const block: import('@/models/dcs-jsonld').DcsClause = {
          '@type': 'dcs:Clause',
          '@id': id,
          'dcs:title': title,
          'dcs:content': { '@list': [typedClauseValuesSummary(instance)] },
        }
        this.blocks.push(block)
        const root = this.layout.find((node) => node['dcs:isRoot'])
        if (root) {
          const children = root['dcs:children']['@list'].map((ref) => ref['@id'])
          children.push(id)
          root['dcs:children'] = { '@list': children.map((childId) => ({ '@id': childId })) }
        }
        const documentId = this.documentIri ?? this.did ?? undefined
        this.policies.push({
          'odrl:assigner': partyReference(undefined, documentId),
          'odrl:assignee': partyReference(undefined, documentId),
          'odrl:target': targetReference(documentId),
          'odrl:action': { '@id': DEFAULT_FIELD_CONSTRAINT_ACTION },
          ...compacted,
          '@id': `urn:uuid:${crypto.randomUUID()}`,
          '@type': compacted['@type'],
          'dcs:prose': { '@id': id },
        } as OdrlRule)
        return id
      }

      const block: import('@/models/dcs-jsonld').DcsClause = {
        '@type': 'dcs:Clause',
        '@id': id,
        'dcs:title': title,
        'dcs:content': { '@list': [typedClauseValuesSummary(instance)] },
        'dcs:typedClause': instance,
      }
      this.blocks.push(block)
      return id
    },
    /**
     * Re-fills a typed clause with new shape-conformant values: the nested
     * machine-readable instance and the clause's human-readable summary text
     * change together, so the two representations cannot drift apart
     * (DCS-FR-CWE-04). Available in both workflows — the template freezes
     * the clause's shape, the contract creator fills its values.
     */
    updateTypedClause(blockId: string, payload: { title?: string; instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance }): void {
      const block = this.blocks.find((b) => b['@id'] === blockId)
      if (!block || !isDcsClause(block as DcsBlock)) return
      const clause = block as DcsClause
      if (!clause['dcs:typedClause']) return
      clause['dcs:typedClause'] = payload.instance
      clause['dcs:content'] = { '@list': [typedClauseValuesSummary(payload.instance)] }
      const trimmedTitle = payload.title?.trim()
      if (trimmedTitle?.length) clause['dcs:title'] = trimmedTitle
    },
    deleteClause(blockId: string): void {
      removeClauseFromLayout(this.layout, blockId)
      this.blocks = this.blocks.filter((b) => b['@id'] !== blockId)
      // A machine-readable rule must never outlive the prose it is backed
      // by — drop policies whose dcs:prose referenced the deleted clause.
      this.policies = this.policies.filter((p) => p['dcs:prose']?.['@id'] !== blockId)
    },
    updateClause(blockId: string, payload: { title?: string; content?: DcsContentSegment[] }): void {
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
    updateDocumentNumber(documentNumber: string): void {
      this.document_number = documentNumber || null
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

// ---- JSON-LD IRI helpers ----

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

function policySetIri(documentId?: string): string {
  return documentId ? `${documentId}#policy-set` : `${UUID_URN_PREFIX}policy-set`
}

// ---- ODRL rule parties/target (DCS ODRL profile: assigner/assignee/target required) ----
//
// Template = open parties (ODRL-Offer character): the two sides of a rule
// aren't bound to real party DIDs yet, so a role-derived open reference is
// used. Contract instance = bound parties (ODRL-Agreement character): once
// bound to a real contract, the same role-derived reference still resolves
// consistently against that contract's own DID, which is what the profile
// requires (presence of odrl:assigner/odrl:assignee/odrl:target); resolving
// to the real counterpart legal-entity DID is left to the semantic mapper
// that already publishes bound envelopes for peer exchange.

function counterpartRole(role: string | undefined): string {
  if (role === 'provider') return 'customer'
  if (role === 'customer') return 'provider'
  return 'assignee'
}

function partyReference(role: string | undefined, documentId?: string): JsonLdReference {
  return { '@id': objectIri('party', role ?? 'assigner', documentId) }
}

function targetReference(documentId?: string): JsonLdReference {
  return { '@id': documentId ?? `${UUID_URN_PREFIX}pending-target` }
}

function requirementForField(
  contractData: readonly DcsDataRequirement[],
  fieldId: string,
): DcsDataRequirement | undefined {
  return contractData.find((r) => r['dcs:fields'].some((f) => f['@id'] === fieldId))
}

/** Assembles the single enclosing odrl:Offer from the flat internal rule array; the first signature seals it into an odrl:Agreement server-side. */
function assemblePolicySet(policies: readonly OdrlRule[], documentId?: string): OdrlSet {
  const set: OdrlSet = {
    '@id': policySetIri(documentId),
    '@type': 'odrl:Offer',
    'odrl:profile': { '@id': DCS_ODRL_PROFILE_IRI },
  }
  const duties = policies.filter((p) => p['@type'] === 'odrl:Duty')
  const permissions = policies.filter((p) => p['@type'] === 'odrl:Permission')
  const prohibitions = policies.filter((p) => p['@type'] === 'odrl:Prohibition')
  if (duties.length) set['odrl:obligation'] = duties
  if (permissions.length) set['odrl:permission'] = permissions
  if (prohibitions.length) set['odrl:prohibition'] = prohibitions
  return set
}

/** Flattens the enclosing ODRL policy (or the empty "no policies yet" array) into the flat internal rule array. */
export function flattenPolicySet(policies: OdrlSet | OdrlRule[] | undefined): OdrlRule[] {
  if (!policies) return []
  if (Array.isArray(policies)) return policies
  return [
    ...(policies['odrl:obligation'] ?? []),
    ...(policies['odrl:permission'] ?? []),
    ...(policies['odrl:prohibition'] ?? []),
  ]
}

export function blockIdFromIri(iri: string): string {
  const local = iri.includes('#') ? iri.slice(iri.lastIndexOf('#') + 1) : iri.slice(UUID_URN_PREFIX.length)
  return decodeURIComponent(local.replace(/^block-/, ''))
}

// ---- Document assembly (trivial — blocks/layout already in JSON-LD) ----

interface CanonicalDocumentInput {
  documentType: DcsDocumentData['@type']
  documentId?: string
  name?: string
  description?: string
  templateType?: TemplateTypeValue
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[]
  layout: DcsLayoutNode[]
  contractData: DcsDataRequirement[]
  policies: OdrlRule[]
  customMetaData?: MetaData[]
  subTemplateSnapshots?: SubTemplateSnapshot[]
  semanticConditionValues?: SemanticConditionValue[]
  parentContractDid?: string
  derivedFromTemplate?: DcsContractData['derivedFromTemplate']
}

function assembleCanonicalDocument(input: CanonicalDocumentInput): DcsDocumentData {
  const isContract = input.documentType === 'dcs:Contract'
  const submittedValues = input.semanticConditionValues ?? []
  // A contract carries its submitted values inline on the requirement field
  // each one targets (dcs:parameterValue) — own fields and composed
  // sub-template fields alike; a template declares fields with no values.
  const contractData = isContract
    ? applyInlineSemanticValues(input.contractData, submittedValues)
    : input.contractData
  const subTemplateSnapshots = isContract
    ? applyInlineSemanticValuesToSnapshots(input.subTemplateSnapshots ?? [], submittedValues)
    : (input.subTemplateSnapshots ?? [])
  const canonicalBlocks = canonicalizeBlocks(input.blocks)
  const canonicalLayout = canonicalizeLayout(input.layout)
  const commonMetadata = {
    ...(input.documentId ? { '@id': `${input.documentId}#metadata` } : {}),
    ...(input.name ? { 'dcs:title': input.name } : {}),
    ...(input.description ? { 'dcs:description': input.description } : {}),
    ...(input.customMetaData?.length ? { 'dcs:customMetaData': input.customMetaData } : {}),
    ...(subTemplateSnapshots.length
      ? { 'dcs:subTemplates': serializeSubTemplateSnapshots(subTemplateSnapshots) }
      : {}),
  }
  const metadata =
    input.documentType === 'dcs:ContractTemplate'
      ? {
          '@type': 'dcs:TemplateMetadata' as const,
          ...commonMetadata,
          'dcs:templateType':
            input.templateType === TemplateType.contractTemplate ? 'dcs:ContractTemplate' : 'dcs:Component',
        }
      : { '@type': 'dcs:ContractMetadata' as const, ...commonMetadata }

  return {
    '@type': input.documentType,
    ...(input.documentId ? { '@id': input.documentId } : {}),
    'dcs:metadata': metadata,
    'dcs:documentStructure': {
      '@type': 'dcs:DocumentStructure',
      ...(input.documentId ? { '@id': `${input.documentId}#document-structure` } : {}),
      'dcs:blocks': { '@list': canonicalBlocks },
      'dcs:layout': canonicalLayout,
    },
    'dcs:contractData': contractData,
    'dcs:policies': assemblePolicySet(input.policies, input.documentId),
    ...(isContract
      ? {
          ...(input.parentContractDid ? { 'dcs:parentContract': { '@id': input.parentContractDid } } : {}),
          ...(input.derivedFromTemplate ? { derivedFromTemplate: input.derivedFromTemplate } : {}),
        }
      : {}),
  }
}

function canonicalizeBlocks(blocks: (DcsBlock | MergedApprovedTemplateBlock)[]): DcsBlock[] {
  return blocks.flatMap((block): DcsBlock[] => {
    if (isDcsMergedApprovedTemplate(block)) {
      const approvedTemplate: DcsApprovedTemplate = {
        '@type': 'dcs:ApprovedTemplate',
        '@id': block['@id'],
        'dcs:templateDid': block['dcs:templateDid'],
        'dcs:version': block['dcs:version'],
        ...(block['dcs:documentNumber'] ? { 'dcs:documentNumber': block['dcs:documentNumber'] } : {}),
      }
      return [approvedTemplate]
    }
    if (isMergedBlockId(block['@id'])) {
      return []
    }
    return [block]
  })
}

function canonicalizeLayout(layout: DcsLayoutNode[]): DcsLayoutNode[] {
  return layout.flatMap((node): DcsLayoutNode[] => {
    if (isMergedBlockId(node['@id'])) {
      return []
    }
    return [
      {
        ...node,
        '@type': 'dcs:LayoutNode',
        'dcs:children': {
          '@list': node['dcs:children']['@list'].filter((ref) => !isMergedBlockId(ref['@id'])),
        },
      },
    ]
  })
}

// ---- buildContractDocument (public API for contract workflow) ----

export interface ContractDocumentInput {
  documentId: string
  name?: string
  description?: string
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[]
  layout: DcsLayoutNode[]
  contractData: DcsDataRequirement[]
  policies: OdrlRule[]
  subTemplateSnapshots: SubTemplateSnapshot[]
  semanticConditionValues: SemanticConditionValue[]
  parentContractDid?: string
  derivedFromTemplate?: DcsContractData['derivedFromTemplate']
}

export function buildContractDocument(input: ContractDocumentInput): DcsContractData {
  return assembleCanonicalDocument({
    ...input,
    documentType: 'dcs:Contract',
  }) as DcsContractData
}

// ---- Sub-template accessors (replaces templateDataToBuilderData) ----

export function getBlocksFromTemplateData(td: SubTemplateSnapshot['template_data']): DcsBlock[] {
  return isDcsDocumentData(td) ? td['dcs:documentStructure']['dcs:blocks']['@list'] : []
}

export function getLayoutFromTemplateData(td: SubTemplateSnapshot['template_data']): DcsLayoutNode[] {
  return isDcsDocumentData(td) ? td['dcs:documentStructure']['dcs:layout'] : []
}

export function getSemanticConditionsFromTemplateData(
  td: DcsDocumentData | SubTemplateSnapshot['template_data'],
): SemanticCondition[] {
  if (!isDcsDocumentData(td)) return []
  return contractDataToSemanticConditions(td['dcs:contractData'], flattenPolicySet(td['dcs:policies']))
}

// ---- Layout helpers ----

function extractBlockList(raw: DcsDocumentStructure['dcs:blocks'] | DcsBlock[]): DcsBlock[] {
  return Array.isArray(raw) ? raw : raw['@list']
}

function getInitialLayout(): DcsLayoutNode[] {
  return [
    {
      '@id': `${UUID_URN_PREFIX}block-${crypto.randomUUID()}`,
      '@type': 'dcs:LayoutNode',
      'dcs:isRoot': true,
      'dcs:children': { '@list': [] },
    },
  ]
}

function layoutNodeChildren(node: DcsLayoutNode): string[] {
  return node['dcs:children']['@list'].map((ref) => ref['@id'])
}

function collectBlockIdsInLayout(layout: DcsLayoutNode[]): Set<string> {
  const ids = new Set<string>()
  const nodeById = new Map(layout.map((n) => [n['@id'], n]))
  function visit(id: string) {
    if (ids.has(id)) return
    ids.add(id)
    const node = nodeById.get(id)
    if (node) layoutNodeChildren(node).forEach(visit)
  }
  const root = layout.find((n) => n['dcs:isRoot'])
  if (root) visit(root['@id'])
  return ids
}

// ---- Block mutation helpers ----

function addBlock(
  layout: DcsLayoutNode[],
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  parentBlockId: string,
  insertIndex: number,
  payload: AddBlockPayload,
  options?: AddBlockOptions,
  documentId?: string,
): string {
  const addToOutline = options?.addToOutline !== false

  if (payload.clauseBlockId) {
    const clauseId = payload.clauseBlockId
    const exists = blocks.find((b) => b['@id'] === clauseId)
    if (exists?.['@type'] !== 'dcs:Clause') {
      throw new Error(`addBlock: clause block not found: ${clauseId}`)
    }
    const inLayout = collectBlockIdsInLayout(layout)
    if (inLayout.has(clauseId)) return clauseId
    const parent = layout.find((n) => n['@id'] === parentBlockId)
    if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
    const children = layoutNodeChildren(parent)
    children.splice(insertIndex, 0, clauseId)
    parent['dcs:children'] = { '@list': children.map((id) => ({ '@id': id })) }
    return clauseId
  }

  const uuid = crypto.randomUUID()
  const id = blockIri(uuid, documentId)
  const block = createBlock(id, payload)

  if (payload.blockType === 'dcs:Clause' && !addToOutline) {
    blocks.push(block)
    return id
  }

  const parent = layout.find((n) => n['@id'] === parentBlockId)
  if (!parent) throw new Error(`addBlock: parent not found: ${parentBlockId}`)
  const children = layoutNodeChildren(parent)
  children.splice(insertIndex, 0, id)
  parent['dcs:children'] = { '@list': children.map((ref) => ({ '@id': ref })) }

  if (payload.blockType === 'dcs:Section' || payload.blockType === 'dcs:ApprovedTemplate') {
    layout.push({ '@id': id, '@type': 'dcs:LayoutNode', 'dcs:children': { '@list': [] } })
  }
  blocks.push(block)
  return id
}

function createBlock(id: string, payload: AddBlockPayload): DcsBlock | MergedApprovedTemplateBlock {
  switch (payload.blockType) {
    case 'dcs:Section':
      return {
        '@type': 'dcs:Section',
        '@id': id,
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
      }
    case 'dcs:TextBlock':
      return { '@type': 'dcs:TextBlock', '@id': id, 'dcs:text': payload.text ?? '' }
    case 'dcs:Clause':
      return {
        '@type': 'dcs:Clause',
        '@id': id,
        'dcs:content': { '@list': payload.content ?? [] },
        ...(payload.title ? { 'dcs:title': payload.title } : {}),
        ...(payload.typedClause ? { 'dcs:typedClause': payload.typedClause } : {}),
      }
    case 'dcs:ApprovedTemplate':
      return {
        '@type': 'dcs:ApprovedTemplate',
        '@id': id,
        'dcs:templateDid': payload.templateId ?? '',
        'dcs:version': payload.version ?? 1,
        ...(payload.document_number != null && payload.document_number !== ''
          ? { 'dcs:documentNumber': payload.document_number }
          : {}),
      }
    default:
      throw new Error('Unknown blockType')
  }
}

function moveBlock(layout: DcsLayoutNode[], blockId: string, parentBlockId: string, insertIndex: number): void {
  const oldParent = layout.find((n) => layoutNodeChildren(n).includes(blockId))
  const newParent = layout.find((n) => n['@id'] === parentBlockId)
  if (!oldParent || !newParent) return

  if (oldParent['@id'] === newParent['@id']) {
    const siblings = layoutNodeChildren(oldParent).filter((id) => id !== blockId)
    const idx = Math.min(insertIndex, siblings.length)
    siblings.splice(idx, 0, blockId)
    oldParent['dcs:children'] = { '@list': siblings.map((id) => ({ '@id': id })) }
    return
  }
  const oldChildren = layoutNodeChildren(oldParent).filter((id) => id !== blockId)
  oldParent['dcs:children'] = { '@list': oldChildren.map((id) => ({ '@id': id })) }
  const newChildren = [...layoutNodeChildren(newParent)]
  const idx = Math.min(insertIndex, newChildren.length)
  newChildren.splice(idx, 0, blockId)
  newParent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
}

function deleteBlock(
  layout: DcsLayoutNode[],
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  blockId: string,
): void {
  const block = blocks.find((b) => b['@id'] === blockId)
  const parent = layout.find((n) => layoutNodeChildren(n).includes(blockId))
  if (!parent) return

  if (block?.['@type'] === 'dcs:Clause') {
    const newChildren = layoutNodeChildren(parent).filter((id) => id !== blockId)
    parent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
    return
  }

  const nodeById = new Map(layout.map((n) => [n['@id'], n]))
  const toRemove = collectDescendantIds(blockId, nodeById)
  const newChildren = layoutNodeChildren(parent).filter((id) => id !== blockId)
  parent['dcs:children'] = { '@list': newChildren.map((id) => ({ '@id': id })) }
  const layoutToKeep = layout.filter((n) => (n['dcs:isRoot'] ?? false) || !toRemove.has(n['@id']))
  layout.length = 0
  layout.push(...layoutToKeep)
  const blocksToKeep = blocks.filter((b) => !toRemove.has(b['@id']))
  blocks.length = 0
  blocks.push(...blocksToKeep)
}

function removeClauseFromLayout(layout: DcsLayoutNode[], blockId: string): void {
  for (const node of layout) {
    const children = layoutNodeChildren(node)
    if (children.includes(blockId)) {
      node['dcs:children'] = { '@list': children.filter((id) => id !== blockId).map((id) => ({ '@id': id })) }
    }
  }
}

function collectDescendantIds(blockId: string, nodeById: Map<string, DcsLayoutNode>): Set<string> {
  const set = new Set<string>([blockId])
  const node = nodeById.get(blockId)
  const childIds = node ? layoutNodeChildren(node) : []
  childIds.forEach((id) => collectDescendantIds(id, nodeById).forEach((x) => set.add(x)))
  return set
}

// ---- Initial state ----

const defaultState: Readonly<Omit<TemplateDraftState, 'blocks' | 'layout'>> = {
  did: null,
  documentIri: null,
  name: '',
  description: '',
  templateDataVersion: 1,
  contractData: [],
  policies: [],
  customMetaData: [],
  subTemplateSnapshots: [],
  templateType: TemplateType.component,
  state: undefined,
  document_number: null,
  version: null,
  updated_at: null,
  created_by: '',
  responsible: null,
  workflow: 'template',
}

function getInitialState(): TemplateDraftState {
  return {
    ...(defaultState as TemplateDraftState),
    blocks: [],
    layout: getInitialLayout(),
    contractData: [],
    policies: [],
    customMetaData: [],
    subTemplateSnapshots: [],
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

// ---- Semantic condition helpers (contractData ↔ SemanticCondition[]) ----

function semanticParamToField(
  conditionId: string,
  parameter: SemanticConditionParameter,
  documentId?: string,
): DcsRequirementField {
  const domainField = ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === parameter.fieldIri)
  const value = parameter.value
  const hasValue = value !== undefined && value !== null && value !== ''
  return {
    '@id': fieldIri(conditionId, parameter.parameterName, documentId),
    '@type': 'dcs:RequirementField',
    'dcs:parameterName': parameter.parameterName,
    'dcs:domainField': { '@id': domainField?.ontologyId ?? parameter.fieldIri },
    'dcs:required': parameter.isRequired,
    ...(hasValue ? { 'dcs:parameterValue': value as string | number | boolean } : {}),
  }
}


/** The clause block whose text carries a placeholder bound to fieldId — the prose an ODRL rule over that field is backed by. */

const ODRL_RULE_TYPES = new Set(['odrl:Duty', 'odrl:Permission', 'odrl:Prohibition'])

/** Compacts a shacl-form instance (absolute IRIs) against the hub context. */
async function compactAgainstHubContext(instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance): Promise<Record<string, unknown>> {
  const compacted = (await jsonld.compact(instance as object, (await getHubContext()) as never)) as Record<string, unknown>
  delete compacted['@context']
  return compacted
}

/** Whether a compacted instance is an ODRL rule the policy buckets accept. */
export function isOdrlRuleInstance(compactedType: unknown): boolean {
  return typeof compactedType === 'string' && ODRL_RULE_TYPES.has(compactedType)
}

function proseBlockForField(blocks: readonly (DcsBlock | MergedApprovedTemplateBlock)[], fieldId: string): JsonLdReference {
  for (const block of blocks) {
    if (isDcsMergedApprovedTemplate(block) || !isDcsClause(block)) continue
    const content = block['dcs:content']
    const segments = typeof content === 'string' ? [] : content['@list']
    for (const segment of segments) {
      if (typeof segment !== 'string' && segment['dcs:bindsTo']['@id'] === fieldId) {
        return { '@id': block['@id'] }
      }
    }
  }
  throw new Error(
    `No clause text binds field ${fieldId}: every machine-readable rule must be backed by human-readable prose (place the field's placeholder in a clause first).`,
  )
}

function semanticConditionToPolicies(
  condition: SemanticCondition,
  contractData: DcsDataRequirement[],
  blocks: readonly (DcsBlock | MergedApprovedTemplateBlock)[],
  documentId?: string,
): OdrlRule[] {
  const req = contractData.find((r) => r['dcs:conditionId'] === condition.conditionId)
  if (!req) return []
  const role = condition.entityRole
  return condition.parameters.flatMap((parameter, _pi) =>
    parameter.operators.flatMap((operator, index) => {
      if (!isStandardOdrlOperator(operator.operate)) return []
      const field = req['dcs:fields'].find((f) => f['dcs:parameterName'] === parameter.parameterName)
      if (!field) return []
      const rightOperand = odrlRightOperand(operator, parameter.type)
      return [
        {
          '@id': policyIri(condition.conditionId, parameter.parameterName, index, documentId),
          '@type': 'odrl:Duty',
          'odrl:action': { '@id': DEFAULT_FIELD_CONSTRAINT_ACTION },
          'odrl:assigner': partyReference(role, documentId),
          'odrl:assignee': partyReference(counterpartRole(role), documentId),
          'odrl:target': targetReference(documentId),
          'dcs:prose': proseBlockForField(blocks, field['@id']),
          'odrl:constraint': {
            '@type': 'odrl:Constraint',
            'odrl:leftOperand': { '@id': field['@id'] },
            'odrl:operator': { '@id': operator.operate },
            ...(rightOperand !== undefined ? { 'odrl:rightOperand': rightOperand } : {}),
          },
        } satisfies OdrlRule,
      ]
    }),
  )
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
        (candidate) => candidate.ontologyId === field['dcs:domainField']?.['@id'],
      )
      if (!ontologyField) {
        // A requirement field is valid without a domain-field binding (the
        // served RequirementField shape requires only dcs:parameterName) —
        // its fill input derives from the field's own declaration.
        return [
          {
            parameterName: field['dcs:parameterName'],
            fieldId: field['@id'],
            type: field['dcs:valueType'] === 'number' ? ('decimal' as const) : ('string' as const),
            fieldIri: field['@id'],
            valueConstraint: undefined,
            uiMetadata: { label: field['dcs:parameterName'] },
            isRequired: field['dcs:required'],
            operators: operatorsByField.get(field['@id']) ?? [],
            value: undefined,
          },
        ]
      }
      return [
        {
          parameterName: field['dcs:parameterName'],
          fieldId: field['@id'],
          type: ontologyField.type,
          fieldIri: ontologyField.ontologyId,
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
