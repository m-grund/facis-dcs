<template>
  <Teleport to="body">
    <div
      v-if="addBlockModalContext !== null"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-block-title"
      @click.self="handleCancel"
    >
      <div
        class="mx-4 flex max-h-[85vh] w-full max-w-2xl flex-col gap-4 overflow-y-auto rounded-2xl bg-base-100 p-6 shadow-xl"
        @click.stop
      >
        <h2 id="add-block-title" class="text-lg font-bold">Add block</h2>
        <template v-if="!isContractWorkflow && isFrameContract">
          <ApprovedSubTemplatePicker
            :templates="subTemplateSnapshots"
            :reference-count-by-did="referenceCountByDid"
            @select="handleAddApprovedTemplate"
          />
        </template>
        <template v-else>
          <div>
            <p class="mb-2 text-sm text-base-content/70">Common:</p>
            <div class="flex flex-col gap-2">
              <BlockPaletteItem
                v-for="item in paletteBlockTypes"
                :key="item.blockType"
                :label="item.label"
                @select="handleAddBlock(item.blockType)"
              />
            </div>
          </div>

          <div class="border-t border-base-300 pt-4">
            <div class="flex flex-col gap-2 mb-2">
              <p class="text-sm text-base-content/70">Defined clauses:</p>
              <input
                v-model="clauseSearch"
                type="search"
                class="input input-bordered input-sm w-full"
                placeholder="Search clauses"
                autocomplete="off"
              />
            </div>
            <div class="flex flex-col gap-2 max-h-64 overflow-y-auto">
              <button v-for="clause in filteredUnusedClauses" :key="clause.blockId" type="button"
                class="text-left min-h-[44px] flex flex-col justify-center select-none rounded-lg border border-base-300 bg-base-100 px-3 py-2 cursor-pointer hover:bg-base-200 transition-colors"
                @click="handleAddClause(clause.blockId)">
                <span class="text-sm font-medium text-base-content">{{ clause.title || 'Untitled clause' }}</span>
                <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                  <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
                </p>
              </button>
              <p v-if="!filteredUnusedClauses.length" class="text-sm text-base-content/50 py-2">
                {{ unusedClauses.length ? 'No matching clauses.' : 'No unplaced clauses from the Clauses tab.' }}
              </p>
            </div>
          </div>

          <div class="border-t border-base-300 pt-4">
            <p class="text-sm text-base-content/70 mb-2">Company party:</p>
            <div class="rounded-lg border border-base-300 bg-base-100 p-3 space-y-3">
              <div class="grid grid-cols-1 sm:grid-cols-[1fr_auto] gap-3 items-end">
                <label class="form-control">
                  <span class="label-text text-xs text-base-content/60 mb-1">Role in contract</span>
                  <select v-model="selectedCompanyRole" class="select select-bordered select-sm w-full">
                    <option value="" disabled>Select role</option>
                    <option v-for="option in companyRoleOptions" :key="option.value" :value="option.value">
                      {{ option.label }}
                    </option>
                  </select>
                </label>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!selectedCompanyRole || !companyFields.length || !companyEntityType"
                  @click="handleAddCompanyPartyBlock"
                >
                  Add company
                </button>
              </div>
              <p class="text-xs text-base-content/50">
                Adds legal name, registration, VAT, representative, contact, address, country, and a fixed role.
              </p>
            </div>
          </div>
        </template>

        <div class="flex justify-end pt-2">
          <button type="button" class="btn btn-outline btn-sm" @click="handleCancel">Cancel</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import {
  DocumentBlockType,
  SEMANTIC_CONDITION_SCHEMA_VERSION,
  isClauseBlock,
  isApprovedTemplateBlock,
  TemplateType,
  type ClauseBlock,
  type DomainFieldDefinition,
  type SemanticConditionParameter,
  type SemanticValueConstraint,
} from '@/modules/template-repository/models/contract-template'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import BlockPaletteItem from './document-block/BlockPaletteItem.vue'
import {
  parseSegments,
  getPlaceholderLabelFromConditions,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import ApprovedSubTemplatePicker from '@template-repository/components/builder-editor/preview/ApprovedSubTemplatePicker.vue'
import { ONTOLOGY_DOMAIN_FIELDS, ONTOLOGY_ENTITY_ROLES } from '@template-repository/utils/ontology-domain-fields'

const draftStore = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { addBlockModalContext } = storeToRefs(uiStore)
const { documentBlocks, semanticConditions, subTemplateSnapshots } = storeToRefs(draftStore)

const isContractWorkflow = computed(() => uiStore.workflow === 'contract')
const paletteBlockTypes = [
  { blockType: DocumentBlockType.Section, label: 'Section' },
  { blockType: DocumentBlockType.Text, label: 'Text' },
  { blockType: DocumentBlockType.Clause, label: 'Clause' },
] as const
const companyRoleOptions = ONTOLOGY_ENTITY_ROLES
const companyFields = ONTOLOGY_DOMAIN_FIELDS
  .filter((field) => field.semanticPath.startsWith('company.'))
  .sort((left, right) => companyFieldSortIndex(left.semanticPath) - companyFieldSortIndex(right.semanticPath))
const companyRoleField = companyFields.find((field) => field.mapsEntityRole) ?? companyFields.find((field) => field.semanticPath === 'company.role')
const companyEntityType = localOntologyName(companyRoleField?.statementType ?? '')
const selectedCompanyRole = ref('')

const isFrameContract = computed(() => draftStore.templateType === TemplateType.frameContract)

// For each template did, number of ApprovedTemplate blocks in the outline that reference it.
const referenceCountByDid = computed(() => {
  const inOutline = draftStore.blockIdsInOutline
  const count: Record<string, number> = {}
  for (const b of documentBlocks.value) {
    if (!isApprovedTemplateBlock(b) || !inOutline.has(b.blockId)) continue
    count[b.templateId] = (count[b.templateId] ?? 0) + 1
  }
  return count
})

/** Clause blocks that are not referenced in the document outline, sorted by title. */
const unusedClauses = computed((): ClauseBlock[] => {
  const inOutline = draftStore.blockIdsInOutline
  const clauses = documentBlocks.value.filter((b): b is ClauseBlock => isClauseBlock(b))
  const unused = clauses.filter((c) => !inOutline.has(c.blockId))
  return [...unused].sort((a, b) => (a.title ?? '').localeCompare(b.title ?? ''))
})
const clauseSearch = ref('')
const filteredUnusedClauses = computed((): ClauseBlock[] => {
  const query = clauseSearch.value.trim().toLowerCase()
  if (!query) return unusedClauses.value
  return unusedClauses.value.filter((clause) =>
    [
      clause.title ?? '',
      clause.text ?? '',
      clause.conditionIds.join(' '),
      clause.semanticPath ?? '',
      clause.schemaRef ?? '',
    ].some((value) => value.toLowerCase().includes(query)),
  )
})

watch(addBlockModalContext, () => {
  clauseSearch.value = ''
  selectedCompanyRole.value = ''
})

function getSegments(clause: ClauseBlock): Segment[] {
  return parseSegments(clause.text ?? '', semanticConditions.value)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, semanticConditions.value)
}

function handleCancel() {
  uiStore.closeAddBlockModal()
}

function handleAddBlock(blockType: DocumentBlockType) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, { blockType, text: '' })
  uiStore.closeAddBlockModal()
}

function handleAddCompanyPartyBlock() {
  const ctx = addBlockModalContext.value
  const role = selectedCompanyRole.value
  if (ctx === null || !role || !companyEntityType) return
  const conditionId = `company-${role}-${crypto.randomUUID()}`
  const roleLabel = companyRoleOptions.find((option) => option.value === role)?.label ?? role
  const parameters = companyFields.map((field) => buildCompanyParameter(field, role))

  draftStore.semanticConditions.push({
    conditionId,
    conditionName: `${roleLabel} company`,
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    entityType: companyEntityType,
    entityRole: role,
    parameters,
  })
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: DocumentBlockType.Clause,
    title: `${roleLabel} company`,
    text: buildCompanyBlockText(conditionId, roleLabel, parameters),
    conditionIds: [conditionId],
    schemaRef: companyFields[0]?.schemaRef,
    semanticPath: 'company',
  })
  uiStore.closeAddBlockModal()
}

function handleAddApprovedTemplate(template: SubTemplateSnapshot) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: DocumentBlockType.ApprovedTemplate,
    text: '',
    templateId: template.did,
    version: template.version,
    document_number: template.document_number,
  })
  uiStore.closeAddBlockModal()
}

function handleAddClause(clauseBlockId: string) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: DocumentBlockType.Clause,
    // Don't set text here, clauseBlockId is enough to link to the document outline.
    text: '',
    clauseBlockId,
  })
  uiStore.closeAddBlockModal()
}

function buildCompanyParameter(field: DomainFieldDefinition, role: string): SemanticConditionParameter {
  const fixedValue = field.semanticPath === 'company.role' ? role : undefined
  const parameter: SemanticConditionParameter = {
    parameterName: field.semanticPath.split('.').join('_'),
    type: field.type,
    schemaRef: field.schemaRef,
    semanticPath: field.semanticPath,
    valueConstraint: cloneValueConstraint(field.valueConstraint),
    isRequired: true,
    operators: [],
    value: undefined,
  }
  if (fixedValue !== undefined) parameter.fixedValue = fixedValue
  return parameter
}

function buildCompanyBlockText(conditionId: string, roleLabel: string, parameters: SemanticConditionParameter[]): string {
  const lines = [`${roleLabel} company`, `Role: ${roleLabel}`]
  for (const parameter of parameters) {
    if (parameter.fixedValue !== undefined) continue
    lines.push(`${formatCompanyFieldLabel(parameter.semanticPath)}: {{${conditionId}.${parameter.parameterName}}}`)
  }
  return lines.join('\n')
}

function formatCompanyFieldLabel(semanticPath: string): string {
  return companyFields.find((field) => field.semanticPath === semanticPath)?.label ?? semanticPath
}

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
  }
}

function localOntologyName(resource: string): string {
  return resource.replace(/^.*[:#/]/, '')
}

function companyFieldSortIndex(semanticPath: string): number {
  const order = [
    'company.role',
    'company.legalName',
    'company.registrationNumber',
    'company.vatId',
    'company.representative.name',
    'company.representative.role',
    'company.contact.email',
    'company.contact.phone',
    'company.location.street',
    'company.location.postalCode',
    'company.location.city',
    'company.location.region',
    'company.location.country',
  ]
  const index = order.indexOf(semanticPath)
  return index >= 0 ? index : order.length
}
</script>
