<script setup lang="ts">
import ApprovedSubTemplatePicker from '@template-repository/components/builder-editor/preview/ApprovedSubTemplatePicker.vue'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import TypedClauseForm from '@template-repository/components/clauses-editor/TypedClauseForm.vue'
import { typedClauseValuesSummary } from '@template-repository/utils/typed-clause'
import {
  getPlaceholderLabelFromConditions,
  parseSegmentsFromContent,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import BlockPaletteItem from './document-block/BlockPaletteItem.vue'
import { useClauseCatalogStore } from '@/stores/clause-catalog-store'
import { TemplateType } from '../../models/contract-template.ts'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { DcsClause } from '@/models/dcs-jsonld'
import type { NewBlockType } from '@template-repository/models/template-draft-store'

const draftStore = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { addBlockModalContext, pendingPlacementClauseBlockId } = storeToRefs(uiStore)
const { blocks, semanticConditions, subTemplateSnapshots } = storeToRefs(draftStore)

const isContractWorkflow = computed(() => uiStore.workflow === 'contract')
const paletteBlockTypes: { blockType: NewBlockType; label: string }[] = [
  { blockType: 'dcs:Section', label: 'Section' },
  { blockType: 'dcs:TextBlock', label: 'Text' },
  { blockType: 'dcs:Clause', label: 'Clause' },
]

const isContractTemplate = computed(() => draftStore.templateType === TemplateType.contractTemplate)

const referenceCountByDid = computed(() => {
  const inOutline = draftStore.blockIdsInOutline
  const count: Record<string, number> = {}
  for (const b of blocks.value) {
    if (b['@type'] !== 'dcs:ApprovedTemplate' || !inOutline.has(b['@id'])) continue
    const did = b['dcs:templateDid']
    count[did] = (count[did] ?? 0) + 1
  }
  return count
})

const unusedClauses = computed((): DcsClause[] => {
  const inOutline = draftStore.blockIdsInOutline
  const clauses = blocks.value.filter((b): b is DcsClause => b['@type'] === 'dcs:Clause')
  const unused = clauses.filter((c) => !inOutline.has(c['@id']))
  return [...unused].sort((a, b) => (a['dcs:title'] ?? '').localeCompare(b['dcs:title'] ?? ''))
})

const pendingPlacementClause = computed(() =>
  unusedClauses.value.find((clause) => clause['@id'] === pendingPlacementClauseBlockId.value),
)
const clauseSearch = ref('')
const filteredUnusedClauses = computed((): DcsClause[] => {
  const query = clauseSearch.value.trim().toLowerCase()
  const clauses = unusedClauses.value.filter((clause) => clause['@id'] !== pendingPlacementClauseBlockId.value)
  if (!query) return clauses
  return clauses.filter((clause) => {
    const contentText = clauseContentText(clause)
    return [clause['dcs:title'] ?? '', contentText].some((v) => v.toLowerCase().includes(query))
  })
})

function clauseContentText(clause: DcsClause): string {
  const content = clause['dcs:content']
  if (typeof content === 'string') return content
  return content['@list'].map((seg) => (typeof seg === 'string' ? seg : '')).join(' ')
}

// Typed clauses (ADR-10): the palette refetches the Semantic Hub's active
// clause catalog every time the modal opens, so a catalog version
// registered+activated in the hub is placeable immediately — no reload.
const clauseCatalog = useClauseCatalogStore()
const { clauses: typedClauseTypes } = storeToRefs(clauseCatalog)
const selectedTypedClauseType = ref<string | null>(null)
const selectedTypedClause = computed(() => typedClauseTypes.value.find((c) => c.type === selectedTypedClauseType.value))

watch(addBlockModalContext, (ctx) => {
  clauseSearch.value = ''
  selectedTypedClauseType.value = null
  if (ctx !== null) void clauseCatalog.refresh()
})

function handleAddTypedClause(payload: { clauseType: string; title: string; instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance }) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  const instanceType = String(payload.instance['@type'] ?? '')
  if (instanceType.startsWith('http://www.w3.org/ns/odrl/2/')) {
    // A hub-templated ODRL rule: the store derives the policy entry plus
    // its prose clause block (placed via the clauses editor).
    void draftStore.addTypedClause(payload)
    uiStore.closeAddBlockModal()
    return
  }
  const instance = payload.instance
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: 'dcs:Clause',
    title: payload.title || clauseCatalog.labelFor(payload.clauseType),
    content: [typedClauseValuesSummary(instance)],
    typedClause: instance,
  })
  uiStore.closeAddBlockModal()
}

function getSegments(clause: DcsClause): Segment[] {
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  return parseSegmentsFromContent(content['@list'], semanticConditions.value)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, semanticConditions.value)
}

function handleCancel() {
  uiStore.closeAddBlockModal()
}

function handleAddBlock(blockType: NewBlockType) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, { blockType })
  uiStore.closeAddBlockModal()
}

function handleAddApprovedTemplate(template: SubTemplateSnapshot) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: 'dcs:ApprovedTemplate',
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
    blockType: 'dcs:Clause',
    clauseBlockId,
  })
  uiStore.closeAddBlockModal()
}
</script>

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
        <template v-if="!isContractWorkflow && isContractTemplate">
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

          <div v-if="typedClauseTypes.length" class="border-t border-base-300 pt-4">
            <p class="mb-2 text-sm text-base-content/70">Typed clauses (Semantic Hub):</p>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="clause in typedClauseTypes"
                :key="clause.type"
                type="button"
                class="btn btn-xs"
                :class="selectedTypedClauseType === clause.type ? 'btn-primary' : 'btn-outline'"
                @click="selectedTypedClauseType = selectedTypedClauseType === clause.type ? null : clause.type"
              >
                {{ clause.label }}
              </button>
            </div>
            <div v-if="selectedTypedClause" class="mt-3 rounded-lg border border-base-300 bg-base-200/40 p-3">
              <TypedClauseForm
                :clause="selectedTypedClause"
                :shapes="clauseCatalog.shapes"
                submit-label="Add to document"
                @submit="handleAddTypedClause"
              />
            </div>
          </div>

          <div class="border-t border-base-300 pt-4">
            <div v-if="pendingPlacementClause" class="mb-4 rounded-lg border border-primary/30 bg-primary/5 p-3">
              <p class="mb-2 text-xs font-semibold text-primary">Selected clause</p>
              <button
                type="button"
                class="flex min-h-11 w-full cursor-pointer flex-col justify-center rounded-lg border border-primary/40 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
                @click="handleAddClause(pendingPlacementClause['@id'])"
              >
                <span class="text-sm font-medium text-base-content">
                  {{ pendingPlacementClause['dcs:title'] || 'Untitled clause' }}
                </span>
                <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                  <ClauseSegmentsPreview
                    :segments="getSegments(pendingPlacementClause)"
                    :get-placeholder-label="getPlaceholderLabel"
                  />
                </p>
              </button>
            </div>
            <div class="mb-2 flex flex-col gap-2">
              <p class="text-sm text-base-content/70">Defined clauses:</p>
              <input
                v-model="clauseSearch"
                type="search"
                class="input-bordered input input-sm w-full"
                placeholder="Search clauses"
                autocomplete="off"
              />
            </div>
            <div class="flex max-h-64 flex-col gap-2 overflow-y-auto">
              <button
                v-for="clause in filteredUnusedClauses"
                :key="clause['@id']"
                type="button"
                class="flex min-h-11 cursor-pointer flex-col justify-center rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
                @click="handleAddClause(clause['@id'])"
              >
                <span class="text-sm font-medium text-base-content">
                  {{ clause['dcs:title'] || 'Untitled clause' }}
                </span>
                <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                  <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
                </p>
              </button>
              <p v-if="!filteredUnusedClauses.length" class="py-2 text-sm text-base-content/50">
                {{ unusedClauses.length ? 'No matching clauses.' : 'No unplaced clauses from the Clauses tab.' }}
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
