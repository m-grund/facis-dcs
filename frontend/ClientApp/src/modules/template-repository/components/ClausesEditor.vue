<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import type { DcsClause, DcsContentSegment } from '@/models/dcs-jsonld'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { blocks, layout, semanticConditions: mainSemanticConditions, subTemplateSnapshots } = storeToRefs(store)

const editingBlockId = ref<string | null>(null)

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))

const clauseBlocks = computed((): DcsClause[] => {
  const mainClauses = blocks.value.filter((b): b is DcsClause => b['@type'] === 'dcs:Clause')
  const subTemplateClauses = subTemplateSnapshots.value.flatMap((subTemplate) => {
    const subBlocks = subTemplate.template_data
    if (!subBlocks || typeof subBlocks !== 'object') return []
    const doc = subBlocks as import('@/models/dcs-jsonld').DcsDocumentData
    if (!doc['dcs:documentStructure']) return []
    return doc['dcs:documentStructure']['dcs:blocks']['@list'].filter(
      (b): b is DcsClause => b['@type'] === 'dcs:Clause',
    )
  })
  return [...mainClauses, ...subTemplateClauses]
})

const semanticConditions = computed(() => {
  const subTemplateConditions = subTemplateSnapshots.value.flatMap((subTemplate) =>
    getSemanticConditionsFromTemplateData(subTemplate.template_data),
  )
  return [...mainSemanticConditions.value, ...subTemplateConditions]
})

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; content: DcsContentSegment[] }) {
  const title = payload.title.trim()
  if (!payload.content.length) return
  store.updateClause(payload.blockId, { title, content: payload.content })
  if (editingBlockId.value === payload.blockId) cancelEdit()
}

function deleteClause(blockId: string) {
  store.deleteClause(blockId)
  if (editingBlockId.value === blockId) cancelEdit()
}

function placeClause(blockId: string) {
  const root = rootBlock.value
  if (!root) return
  uiStore.startClausePlacement(blockId)
  uiStore.openAddBlockModal(root['@id'], root['dcs:children']['@list'].length)
}
</script>

<template>
  <ExistingClausesList
    :clause-blocks="clauseBlocks"
    :semantic-conditions="semanticConditions"
    :block-ids-in-outline="store.blockIdsInOutline"
    :editing-block-id="editingBlockId"
    :editable="uiStore.isTemplateEditable"
    @delete="deleteClause"
    @edit="startEditClause"
    @place="placeClause"
    @save="saveEditedClause"
    @cancel-edit="cancelEdit"
  />
</template>
