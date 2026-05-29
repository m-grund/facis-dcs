<template>
  <div class="space-y-6">
    <!-- Section 1: New clause -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <ClauseEditorForm
        mode="create"
        initial-title=""
        initial-text=""
        :semantic-conditions="semanticConditions"
        @submit="addClause"
      />
    </section>

    <!-- Section 2: Existing clauses -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Existing clauses</h3>
      <ExistingClausesList
        :clause-blocks="clauseBlocks"
        :semantic-conditions="semanticConditions"
        :block-ids-in-outline="store.blockIdsInOutline"
        :editing-block-id="editingBlockId"
        :editable="uiStore.isTemplateEditable"
        @delete="deleteClause"
        @edit="startEditClause"
        @save="saveEditedClause"
        @cancel-edit="cancelEdit"
      />

      <div v-if="uiStore.isTemplateEditable && ontologyClausePresets.length" class="mt-5 border-t border-base-300 pt-4">
        <h4 class="mb-3 text-xs font-semibold uppercase text-base-content/50">Prebuilt clauses</h4>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <div
            v-for="preset in ontologyClausePresets"
            :key="preset.id"
            class="group grid min-h-[88px] grid-cols-1 items-center gap-3 rounded-lg border border-base-300 bg-base-100 px-3 py-3 shadow-sm transition-all hover:border-primary/50 hover:bg-base-200 hover:shadow md:grid-cols-[minmax(0,1fr)_minmax(11rem,14rem)_auto]"
          >
            <div class="min-w-0">
              <span class="block text-sm font-medium text-base-content">{{ preset.label }}</span>
              <span class="block text-xs text-base-content/50">{{ preset.fields.length }} ontology fields</span>
            </div>
            <label v-if="preset.roleRequired" class="mx-auto w-full max-w-56">
              <span class="label-text mb-1 block text-xs text-base-content/60">Contract role</span>
              <select
                v-model="selectedPrebuiltRoles[preset.id]"
                class="select select-bordered select-xs w-full text-left"
              >
                <option value="">Select role</option>
                <option v-for="option in roleOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <span v-else class="hidden md:block" />
            <button
              type="button"
              class="btn btn-secondary btn-xs justify-self-start transition-transform group-hover:translate-x-0.5 md:justify-self-end"
              :disabled="preset.roleRequired && !selectedPrebuiltRoles[preset.id]"
              @click="addPrebuiltClause(preset.id)"
            >
              Add
            </button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import {
  SEMANTIC_CONDITION_SCHEMA_VERSION,
  isClauseBlock,
  type ClauseBlock,
} from '@/modules/template-repository/models/contract-template'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import {
  ONTOLOGY_CLAUSE_PRESETS,
  buildOntologyClauseText,
  buildOntologyConditionParameters,
  ontologyRoleOptions,
  roleLabelFor,
} from '@template-repository/utils/ontology-clause-presets'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { documentBlocks, semanticConditions: mainSemanticConditions, subTemplateSnapshots } = storeToRefs(store)

const editingBlockId = ref<string | null>(null)
const selectedPrebuiltRoles = ref<Record<string, string>>({})
const ontologyClausePresets = ONTOLOGY_CLAUSE_PRESETS
const roleOptions = ontologyRoleOptions

/** Extract conditionIds from clause text placeholders {{conditionId.parameterName}}. */
function conditionIdsFromText(text: string): string[] {
  const set = new Set<string>()
  const re = /\{\{([^}]+)\}\}/g
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    const inner = m[1] ?? ''
    const dot = inner.indexOf('.')
    const conditionId = dot >= 0 ? inner.slice(0, dot) : inner
    if (conditionId) set.add(conditionId)
  }
  return [...set]
}

const clauseBlocks = computed((): ClauseBlock[] => {
  const mainClauses = documentBlocks.value.filter((b): b is ClauseBlock => isClauseBlock(b))
  const subTemplateClauses = subTemplateSnapshots.value.flatMap((subTemplate) =>
    (subTemplate.template_data?.documentBlocks ?? []).filter((block): block is ClauseBlock => isClauseBlock(block)),
  )
  return [...mainClauses, ...subTemplateClauses]
})

const semanticConditions = computed(() => {
  const subTemplateConditions = subTemplateSnapshots.value.flatMap(
    (subTemplate) => subTemplate.template_data?.semanticConditions ?? [],
  )
  return [...mainSemanticConditions.value, ...subTemplateConditions]
})

function addClause(payload: { title: string; text: string }) {
  const text = payload.text
  if (!text.trim()) return
  store.addClause({
    title: payload.title.trim(),
    text,
    conditionIds: conditionIdsFromText(text),
  })
}

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; text: string }) {
  const text = payload.text
  const title = payload.title.trim()
  if (!text.trim()) return
  store.updateClause(payload.blockId, {
    title,
    text,
    conditionIds: conditionIdsFromText(text),
  })
  if (editingBlockId.value === payload.blockId) cancelEdit()
}

function deleteClause(blockId: string) {
  store.deleteClause(blockId)
  if (editingBlockId.value === blockId) cancelEdit()
}

function addPrebuiltClause(presetId: string) {
  const preset = ontologyClausePresets.find((item) => item.id === presetId)
  if (!preset) return
  const role = preset.roleRequired ? selectedPrebuiltRoles.value[preset.id] ?? '' : ''
  if (preset.roleRequired && !role) return

  const conditionId = `${preset.id}-${role || 'default'}-${crypto.randomUUID()}`
  const roleLabel = role ? roleLabelFor(role) : ''
  const title = roleLabel ? `${roleLabel} ${preset.label}` : preset.label
  const parameters = buildOntologyConditionParameters(preset)
  const text = buildOntologyClauseText(conditionId, preset, role)

  store.semanticConditions.push({
    conditionId,
    conditionName: title,
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    entityType: preset.entityType,
    ...(role ? { entityRole: role } : {}),
    parameters,
  })
  const blockId = store.addClause({
    title,
    text,
    conditionIds: conditionIdsFromText(text),
    schemaRef: preset.fields[0]?.schemaRef,
    semanticPath: preset.fields[0]?.semanticPath.split('.', 1)[0] ?? preset.id,
  })
  editingBlockId.value = blockId
}
</script>
